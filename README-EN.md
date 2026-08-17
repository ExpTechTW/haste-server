# haste-server

*[中文說明](README.md) · English*

Paste code or logs, get back a short share code. One Go binary: JSON API, raw
endpoints, and the React frontend all embedded.

- **Short hash-style codes that never collide** — `k7Qm2Xp9`, not `1`, `2`, `3`.
- **Write-once.** There is no edit or delete path, and the database enforces it.
- **A byte cap, not a promise.** Storage is bounded on every write; nothing
  advertises a lifetime the server cannot honour.
- **Line links** — `#L17-L25`, addressed and shared the way GitHub does it.
- **Heavy compression with a built-in dictionary.** A 300-byte log excerpt
  stores in ~19 bytes.
- **Download as a file**, named after the code with the right extension.
- **SQLite with split read/write pools**, WAL, and a 48 MiB page cache.
- **React 19 + Tailwind 4 + shadcn/ui**, Shiki highlighting for 80+ languages,
  live as you type, light and dark.

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

## Using it

| Action | How |
| ------ | --- |
| Save | `⌘/Ctrl + S`, or the Save button |
| Load a file | Drop it onto the editor |
| Select a line | Click its number → `#L17` |
| Select a range | Shift-click another number → `#L17-L25` |
| Copy link | `C` — includes the line selection if there is one |
| Copy content | The copy button; line numbers are never included |
| Raw / Download / New | `R` / `S` / `N` |

The language is detected as you type and highlighted live. The picker shows
`Auto · Dart`; choosing one yourself pins it.

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
curl http://localhost:8080/api/pastes/LkKzpZ2q   # JSON, with content
curl http://localhost:8080/raw/LkKzpZ2q          # text/plain

# Save it as a file — the server names it for you
curl -OJ http://localhost:8080/download/LkKzpZ2q # -> LkKzpZ2q.dart
```

A create returns the code, every URL, the download filename, and what the
compressor achieved:

```json
{
  "key": "LkKzpZ2q",
  "url": "http://localhost:8080/LkKzpZ2q",
  "rawUrl": "http://localhost:8080/raw/LkKzpZ2q",
  "downloadUrl": "http://localhost:8080/download/LkKzpZ2q",
  "filename": "LkKzpZ2q.dart",
  "language": "dart",
  "chars": 231,
  "bytes": 231,
  "stored": 162,
  "ratio": 1.43,
  "createdAt": "2026-08-18T16:19:25Z"
}
```

| Method | Path                | Purpose                                     |
| ------ | ------------------- | ------------------------------------------- |
| `POST` | `/api/pastes`       | Create. JSON envelope or raw body.          |
| `GET`  | `/api/pastes/{key}` | Read as JSON, including content.            |
| `GET`  | `/raw/{key}`        | Read as `text/plain`.                       |
| `GET`  | `/download/{key}`   | Download as `{key}.{ext}` for its language. |
| `GET`  | `/api/config`       | Limits the server enforces.                 |
| `GET`  | `/api/stats`        | Live count, corpus ratio, and cap usage.    |
| `GET`  | `/healthz`          | Liveness.                                   |
| `POST` | `/documents`        | Original haste-server protocol.             |
| `GET`  | `/documents/{key}`  | Original haste-server protocol.             |

`/documents` speaks the original haste wire format, so existing CLI wrappers
keep working unchanged.

Errors come back as `{"error": "code", "message": "..."}` with a matching
status: `400` empty or malformed, `413` over the limit, `429` rate limited,
`503` write queue full, `404` unknown or already evicted.

## Configuration

All settings live in `.env` (see [.env.example](.env.example) for the annotated
list). Real environment variables override the file.

| Variable                 | Default          | Notes                                       |
| ------------------------ | ---------------- | ------------------------------------------- |
| `HASTE_ADDR`             | `:8080`          | Listen address.                              |
| `HASTE_MAX_CHARS`        | `4000`           | Unicode code points, not bytes.              |
| `HASTE_CODE_MIN_LEN`     | `8`              | Shortest share code, 1–10 base62 characters. |
| `HASTE_MAX_BYTES`        | `1GiB`           | Hard cap; evicts LRU on write. `0` = none.   |
| `HASTE_TTL_ACCESS`       | off              | Drop pastes unread for this long.            |
| `HASTE_TTL_CREATE`       | off              | Drop pastes older than this.                 |
| `HASTE_CLEANUP_INTERVAL` | `1h`             | Sweep cadence for the TTLs.                  |
| `HASTE_ZSTD_LEVEL`       | `19`             | 1–22.                                        |
| `HASTE_WRITE_CONCURRENCY`| cores            | Simultaneous writes; excess queues.          |
| `HASTE_WRITE_QUEUE`      | `512`            | Waiting writes before shedding with 503.     |
| `HASTE_SQLITE_CACHE_MB`  | `48`             | Page cache **per connection**.               |
| `HASTE_READ_POOL`        | `min(NumCPU, 8)` | Read connections; the writer is always 1.    |
| `HASTE_RATE_RPS`         | `1`              | Creations per IP per second; `0` disables.   |
| `HASTE_BASE_URL`         | derived          | Set when behind a proxy.                     |
| `HASTE_TRUST_PROXY`      | `false`          | Enable only behind a proxy you control.      |

## How it works

### Share codes

Codes come from a counter, not from randomness, so a collision is not unlikely —
it is impossible, and no retry loop against the database is ever needed.

Handing out the counter itself would make every paste one increment away from
being typed into the address bar, so the counter runs through a keyed Feistel
network with cycle walking. That is a bijection onto exactly the code space of a
given length: still unique, but the output is indistinguishable from a random
base62 hash. Consecutive pastes look like this:

```
wxaTLCgp   DDj5XO4k   ACHwVAYu   idpfsjAB   G0VtfB3v
```

Codes start at `HASTE_CODE_MIN_LEN` characters (8 by default, 2.2e14 codes) and
grow only when that space is genuinely exhausted. Length is the only lever that
matters against a brute-force sweep, so lower it only if you specifically want
tiny links:

| Length | Codes  |
| ------ | ------ |
| 6      | 5.7e10 |
| 7      | 3.5e12 |
| 8      | 2.2e14 |
| 9      | 1.4e16 |

The key comes from `HASTE_ID_SECRET`, generated and persisted on first run if
unset. Changing it cannot break existing pastes: codes are stored, not derived.

The generator also refuses any code that would shadow a route (`api`, `raw`,
`download`, `manifest`, …), so share links and server paths can never conflict
in either direction.

### Immutability

A paste is write-once. There is no update or delete endpoint, and a
`BEFORE UPDATE` trigger aborts any write that reaches the table anyway — so no
future code path, migration, or `sqlite3` session can quietly rewrite a code
that has already been shared. Rows are only ever removed, never rewritten.

### Line links

Line numbers are drawn with a CSS counter rather than as text, so they can never
be dragged into a copied selection. That leaves nothing to click, so each line
also carries an empty anchor positioned over its gutter: clickable and
linkable, with no text of its own to end up on the clipboard.

Selection lives in the URL fragment and nowhere else, so the link in the address
bar is always exactly what a reader would receive.

### Log severity

In a log paste, `TRACE` / `DEBUG` / `INFO` / `WARN` / `ERROR` / `FATAL` are
coloured by how bad they are.

Shiki's log grammar does tag them as `log.error`, `log.warning` and so on, but
the GitHub themes carry no rules for those scopes, so every level falls through
to whatever generic scope it piggybacks on — and the result is backwards. `WARN`
inherits `markup.deleted` and comes out red, while `ERROR` inherits
`string.regexp` and comes out blue: calmer than the warning above it, and in the
light theme nearly the same colour as ordinary text. Rules for those scopes are
added from GitHub's own Primer palette, putting the levels back in the order a
reader expects.

### Retention

Retention is a budget rather than a promise, which is why nothing in the UI or
the API advertises a lifetime.

`HASTE_MAX_BYTES` is the only hard guarantee. It is checked inside the same
transaction as every insert, evicting least-recently-**read** pastes to make
room, so the database cannot outgrow its allowance no matter how fast pastes
arrive — an hourly sweep alone would let a burst overshoot for an hour. Two
optional TTLs trim further, one on last access and one on creation; both are off
by default, and either can be left unset to disable that rule entirely. The
sweep applies them in priority order: cap, then access age, then creation age.

Tracking last access means writing on reads, which would funnel every read
through the single writer. Instead reads queue their timestamps in memory and a
flush writes them back once a minute in one transaction. Losing a flush costs
LRU accuracy, never data. The immutability trigger names the content columns
explicitly so `accessed_at` remains writable while everything a reader can
observe stays frozen.

Sizing it is a question of what people paste. Measured on full 4000-character
pastes, worst case per row:

| Content                   | Raw    | Stored | On disk | 1 GiB holds |
| ------------------------- | ------ | ------ | ------- | ----------- |
| Go source                 | 4000 B | 250 B  | 360 B   | 3.0M        |
| Structured / JSON logs    | 4000 B | ~325 B | 442 B   | 2.4M        |
| English prose             | 4000 B | 1048 B | 1434 B  | 749k        |
| CJK prose                 | 12 KB  | 3907 B | 4162 B  | 258k        |
| Incompressible CJK        | 12 KB  | 8734 B | 8937 B  | 120k        |

### Compression

Pastes are a few kilobytes at most, which is exactly where general-purpose
compressors struggle: most of the input is spent teaching the compressor about
the data before it can encode anything cheaply. A prebuilt dictionary of common
source and log fragments ([dict/v1.txt](internal/compress/dict/v1.txt)) supplies
that model up front. Each paste is compressed with and without it and the
smaller frame wins, with the codec recorded per row so the dictionary can be
revised later without invalidating anything already stored.

The choice of codec and level is measured, not assumed. Across 160 full-size
pastes of logs, code, prose and incompressible data:

| Codec                   | Dict | B/paste | Encode | Decode |
| ----------------------- | ---- | ------- | ------ | ------ |
| **zstd -19 + dict**     | yes  | **760** | 345 µs | 4 µs   |
| brotli q11              |      | 766     | 4.1 ms | 14 µs  |
| zstd -19                |      | 799     | 576 µs | 4 µs   |
| zstd -4 + dict          | yes  | 811     | 15 µs  | 4 µs   |
| deflate -9 + dict       | yes  | 841     | 99 µs  | 14 µs  |
| gzip -9                 |      | 916     | 82 µs  | 17 µs  |
| xz (LZMA2)              |      | 954     | 577 µs | 211 µs |
| bzip2 -9                |      | 965     | 267 µs | 61 µs  |

bzip2 and xz coming last is not a surprise once the input size is taken into
account: block sorting and a large LZMA window both need far more than four
kilobytes before they repay their own overhead. Levels 20-22 produce byte-for-
byte the same output as 19, so there is nothing above the default to reach for.

Keeping the smaller of the two frames is worth about 1% of total storage, and
the two are independent, so they are encoded on separate cores. The dictionary
gives its encoder a head start and finishes in roughly half the time of the
plain one, so overlapping them returns the slower of the two rather than their
sum: identical bytes, and a single write went from 967 µs to 661 µs.

The remaining lever is the dictionary rather than the algorithm. A dictionary
*trained* on pastes, rather than hand-written, measured 705 B/paste on held-out
data — another 7%, with nothing to gain past about 16 KB of dictionary. That
figure comes from synthetic samples drawn from the same generators as the
evaluation set, so treat it as an upper bound until it is repeated on real
traffic.

### Storage

SQLite in WAL mode allows one writer and many concurrent readers, so the server
models exactly that: a single-connection writer that takes its lock immediately,
and a pool of readers pinned to `query_only`. Reads never touch the write lock,
which removes the "database is locked" failure mode that comes from letting one
shared pool interleave both.

Each connection gets `HASTE_SQLITE_CACHE_MB` of page cache (48 MiB by default),
plus a shared 256 MiB mmap window. A sweep that deletes anything also
checkpoints the WAL, so the space actually comes back.

Writes need admission control, but not for the reason it first appears. SQLite
is already a queue — `SetMaxOpenConns(1)` serialises transactions FIFO — and an
insert costs about 100 µs. Compression is the expensive half, and it runs before
the transaction with no bound of its own. Measured at 512 concurrent writers,
letting that run unbounded gave a p99 of 296 ms and a worst case of 523 ms;
bounding it to one write per core, with a queue that sheds to `503` once full,
gave a p99 of 111 ms and a worst case of 117 ms at the same throughput.

### The editor

The editor is a transparent textarea sitting on a highlighted copy of the same
text. Two things keep that from drifting apart: both layers are driven by one
set of metrics rather than two definitions that can diverge, and only the
wrapper scrolls, so there is no scroll position to synchronise and no scrollbar
on one side narrowing its text and moving where lines wrap.

Highlighting is computed synchronously during render. An async repaint would
leave the visible text a frame behind the caret, which reads as a glitch.

## Testing

```bash
make test
```

The Go suite covers the invariants that matter: code uniqueness across tier
boundaries, that each tier is a true permutation, that codes honour the minimum
length and leak no ordering, the immutability trigger, the reader pool rejecting
writes, pragmas actually landing on both pools, the byte cap holding on every
single write, eviction removing the least recently *read* paste rather than
merely the oldest, both TTLs including the fact that they are off by default,
reads not writing through until flushed, the write queue shedding when full,
concurrent creates receiving distinct codes, download filenames per language,
and the full HTTP surface including limits and the raw endpoint's hardening
headers.

Two suites report rather than assert, because they measure the machine they run
on: `TestStorageFootprint` prints what a full-size paste costs on disk for each
kind of content, and `TestLevelTradeoff` prints size and time for every zstd
level. The conclusions they support are pinned by ordinary assertions next to
them.

The frontend suite guards language detection, which is a pile of heuristics and
therefore regresses silently — a rule loosened for one language quietly steals
another's pastes. Every case in
[languages.test.ts](web/src/lib/languages.test.ts) comes from a real
misdetection, so the corpus only ever grows.
[lines.test.ts](web/src/lib/lines.test.ts) covers fragment parsing and range
selection, including the backwards ranges a shift-click upwards produces.

## Layout

```
cmd/haste/          entry point, graceful shutdown, retention sweeper
internal/config/    .env loading and validation
internal/id/        counter to short code (tiers + Feistel permutation)
internal/compress/  zstd codec and the embedded dictionary
internal/store/     SQLite schema, read/write pools, queries
internal/httpapi/   routes, middleware, rate limiting, SPA serving
internal/webui/     embedded frontend build
web/                React + Tailwind + shadcn/ui source
```
