# haste-server

Paste code or logs, get back the shortest share code that is still free. One Go
binary: JSON API, raw endpoints, and the React frontend all embedded.

- **Shortest possible codes, never colliding.** The first 62 pastes get a
  one-character code, the next 3 844 get two, and so on.
- **Write-once.** There is no edit or delete path, and the database enforces it.
- **zstd level 19, with a dictionary.** A 300-byte log excerpt stores in ~19 bytes.
- **SQLite with split read/write pools**, WAL, and a 48 MiB page cache.
- **React 19 + Tailwind 4 + shadcn/ui**, Shiki highlighting, light and dark.

## Quick start

Requires Go 1.26+, Node 22+, and a C toolchain (zstd is a C library).

```bash
cp .env.example .env
make build
./bin/haste
```

Or with Docker:

```bash
docker compose up --build
```

Then open <http://localhost:8080>.

For frontend work, run the API and the Vite dev server side by side — Vite
proxies `/api`, `/raw`, and `/documents` to port 8080:

```bash
make dev      # API on :8080
make dev-web  # UI on :5173
```

## API

Every endpoint enforces the same character limit as the UI.

```bash
# Create from a file
curl --data-binary @main.go http://localhost:8080/api/pastes

# Create with JSON, naming the language
curl -X POST http://localhost:8080/api/pastes \
  -H 'Content-Type: application/json' \
  -d '{"content":"print(1)","language":"python"}'

# Read it back
curl http://localhost:8080/api/pastes/2   # JSON, with content
curl http://localhost:8080/raw/2          # text/plain
```

A create returns the code, both URLs, and what the compressor achieved:

```json
{
  "key": "2",
  "url": "http://localhost:8080/2",
  "rawUrl": "http://localhost:8080/raw/2",
  "language": "go",
  "chars": 66,
  "bytes": 66,
  "stored": 45,
  "ratio": 1.47,
  "createdAt": "2026-08-17T15:02:10Z",
  "expiresAt": "2026-09-16T15:02:10Z"
}
```

| Method | Path                | Purpose                                     |
| ------ | ------------------- | ------------------------------------------- |
| `POST` | `/api/pastes`       | Create. JSON envelope or raw body.          |
| `GET`  | `/api/pastes/{key}` | Read as JSON, including content.            |
| `GET`  | `/raw/{key}`        | Read as `text/plain`.                       |
| `GET`  | `/api/config`       | Limits the server enforces.                 |
| `GET`  | `/api/stats`        | Live paste count and corpus ratio.          |
| `GET`  | `/healthz`          | Liveness.                                   |
| `POST` | `/documents`        | Original haste-server protocol.             |
| `GET`  | `/documents/{key}`  | Original haste-server protocol.             |

`/documents` speaks the original haste wire format, so existing CLI wrappers
keep working unchanged.

Errors come back as `{"error": "code", "message": "..."}` with a matching
status: `400` empty or malformed, `413` over the limit, `429` rate limited,
`404` unknown or expired.

## Configuration

All settings live in `.env` (see [.env.example](.env.example) for the annotated
list). Real environment variables override the file.

| Variable                 | Default          | Notes                                        |
| ------------------------ | ---------------- | -------------------------------------------- |
| `HASTE_ADDR`             | `:8080`          | Listen address.                               |
| `HASTE_MAX_CHARS`        | `4000`           | Unicode code points, not bytes.               |
| `HASTE_RETENTION`        | `30d`            | Accepts `d` and `w`; `0` keeps forever.       |
| `HASTE_CLEANUP_INTERVAL` | `1h`             | Sweep cadence for expired pastes.             |
| `HASTE_ZSTD_LEVEL`       | `19`             | 1–22.                                         |
| `HASTE_SQLITE_CACHE_MB`  | `48`             | Page cache **per connection**.                |
| `HASTE_READ_POOL`        | `min(NumCPU, 8)` | Read connections; the writer is always 1.     |
| `HASTE_RATE_RPS`         | `1`              | Creations per IP per second; `0` disables.    |
| `HASTE_BASE_URL`         | derived          | Set when behind a proxy.                      |
| `HASTE_TRUST_PROXY`      | `false`          | Enable only behind a proxy you control.       |

## How it works

### Share codes

Codes come from a counter, not from randomness, so a collision is not unlikely —
it is impossible, and no retry loop against the database is ever needed. Codes
are laid out in tiers, and length only grows when the shorter space is genuinely
exhausted:

| Length | Codes      | Cumulative |
| ------ | ---------- | ---------- |
| 1      | 62         | 62         |
| 2      | 3 844      | 3 906      |
| 3      | 238 328    | 242 234    |
| 4      | 14 776 336 | 15 018 570 |

Handing out the raw counter would make every paste one increment away from being
guessed, so within each tier the counter runs through a keyed Feistel network
with cycle walking. That is a bijection onto exactly that tier's code space:
still unique, still minimal length, but consecutive pastes land far apart. The
key comes from `HASTE_ID_SECRET`, generated and persisted on first run if unset.

The generator also refuses any code that would shadow a route (`api`, `raw`,
`assets`, …), so share links and server paths can never conflict in either
direction.

### Immutability

A paste is write-once. There is no update or delete endpoint, and a
`BEFORE UPDATE` trigger aborts any write that reaches the table anyway — so no
future code path, migration, or `sqlite3` session can quietly rewrite a code
that has already been shared. Deletion happens only through expiry.

### Compression

Pastes are a few kilobytes at most, which is exactly where plain zstd struggles:
most of the input is spent teaching the compressor about the data before it can
encode anything cheaply. A prebuilt dictionary of common source and log
fragments ([dict/v1.txt](internal/compress/dict/v1.txt)) supplies that model up
front.

Each paste is compressed both ways and the smaller frame wins, with the codec
recorded per row so the dictionary can be revised later without invalidating
anything already stored. Measured on this build:

| Input                     | Raw   | Stored | Ratio |
| ------------------------- | ----- | ------ | ----- |
| Three-line structured log | 301 B | 19 B   | 15.8× |
| One JSON log line         | 111 B | 20 B   | 5.6×  |
| Small Go program          | 66 B  | 45 B   | 1.5×  |

### Storage

SQLite in WAL mode allows one writer and many concurrent readers, so the server
models exactly that: a single-connection writer that takes its lock immediately,
and a pool of readers pinned to `query_only`. Reads never touch the write lock,
which removes the "database is locked" failure mode that comes from letting one
shared pool interleave both.

Each connection gets `HASTE_SQLITE_CACHE_MB` of page cache (48 MiB by default),
plus a shared 256 MiB mmap window. Expired rows are swept hourly; a sweep that
deletes anything also checkpoints the WAL so the space actually comes back.

## Testing

```bash
make test
```

The Go suite covers the invariants that matter: code uniqueness across tier
boundaries and that each tier is a true permutation, the immutability trigger,
the reader pool rejecting writes, pragmas actually landing on both pools,
expiry and purge, concurrent creates receiving distinct codes, and the full HTTP
surface including limits and the raw endpoint's hardening headers.

## Layout

```
cmd/haste/          entry point, graceful shutdown, expiry sweeper
internal/config/    .env loading and validation
internal/id/        counter to short code (tiers + Feistel permutation)
internal/compress/  zstd codec and the embedded dictionary
internal/store/     SQLite schema, read/write pools, queries
internal/httpapi/   routes, middleware, rate limiting, SPA serving
internal/webui/     embedded frontend build
web/                React + Tailwind + shadcn/ui source
```
