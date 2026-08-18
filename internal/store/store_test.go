package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/YuYu1015/haste-server/internal/compress"
	"github.com/YuYu1015/haste-server/internal/corpus"
	"github.com/YuYu1015/haste-server/internal/id"
)

func newTestStore(t *testing.T, tweak func(*Options)) *Store {
	t.Helper()

	codec, err := compress.New(compress.DefaultLevel)
	if err != nil {
		t.Fatalf("compress.New: %v", err)
	}
	t.Cleanup(codec.Close)

	opts := Options{
		Path:     filepath.Join(t.TempDir(), "haste.db"),
		CacheMB:  8,
		ReadPool: 4,
		MaxChars: 4000,
		Codec:    codec,
		IDs:      id.NewGenerator([]byte("test-secret"), id.DefaultMinLen, []string{"api", "raw"}),
	}
	if tweak != nil {
		tweak(&opts)
	}

	st, err := Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestCreateAndGetRoundTrip(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	content := "package main\n\nfunc main() {\n\tprintln(\"héllo, 世界\")\n}\n"
	p, err := st.Create(ctx, content, "go", NoExpiry)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Code == "" {
		t.Fatal("Create returned an empty code")
	}

	got, body, err := st.Get(ctx, p.Code)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if body != content {
		t.Errorf("body round-trip mismatch:\n got %q\nwant %q", body, content)
	}
	if got.Language != "go" {
		t.Errorf("Language = %q, want %q", got.Language, "go")
	}
	// Runes, not bytes: the limit is expressed in characters.
	if want := len([]rune(content)); got.Chars != want {
		t.Errorf("Chars = %d, want %d", got.Chars, want)
	}
}

// Codes must read as hashes from the very first paste, not as a counter that
// happens to start small.
func TestCodesAreMinimumLengthAndUnordered(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	var codes []string
	for i := 0; i < 20; i++ {
		p, err := st.Create(ctx, "x", "", NoExpiry)
		if err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
		if len(p.Code) != id.DefaultMinLen {
			t.Fatalf("paste #%d got code %q (len %d), want %d", i, p.Code, len(p.Code), id.DefaultMinLen)
		}
		codes = append(codes, p.Code)
	}

	for i := 1; i < len(codes); i++ {
		if codes[i-1][:3] == codes[i][:3] {
			t.Errorf("consecutive codes %q and %q share a prefix", codes[i-1], codes[i])
		}
	}
}

func TestLimits(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	if _, err := st.Create(ctx, "", "", NoExpiry); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty paste: got %v, want ErrEmpty", err)
	}
	// 4001 multi-byte runes: proves the cap counts characters, not bytes.
	if _, err := st.Create(ctx, strings.Repeat("界", 4001), "", NoExpiry); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversized paste: got %v, want ErrTooLarge", err)
	}
	if _, err := st.Create(ctx, strings.Repeat("界", 4000), "", NoExpiry); err != nil {
		t.Errorf("paste at exactly the limit should be accepted: %v", err)
	}
}

func TestGetUnknownCode(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	for _, code := range []string{"zzzz", "", "../../etc/passwd", "not a code"} {
		if _, _, err := st.Get(ctx, code); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q): got %v, want ErrNotFound", code, err)
		}
	}
}

// "Locked" has to mean locked in the database, not merely unexposed by the API.
func TestPasteContentIsImmutable(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	p, err := st.Create(ctx, "original", "", NoExpiry)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Values match each column's declared type, so it is the trigger that
	// rejects the write rather than STRICT type checking getting there first.
	columns := []struct {
		name  string
		value any
	}{
		{"seq", int64(999)},
		{"code", "tampered"},
		{"body", []byte("tampered")},
		{"codec", int64(1)},
		{"bytes", int64(1)},
		{"chars", int64(1)},
		{"raw_bytes", int64(1)},
		{"language", "tampered"},
		{"created_at", int64(0)},
		// A lifetime is a promise made to whoever holds the link, so it is
		// frozen the same way the content is — in both directions.
		{"expires_at", int64(0)},
	}

	for _, c := range columns {
		_, err := st.w.ExecContext(ctx,
			fmt.Sprintf(`UPDATE pastes SET %s = ? WHERE code = ?`, c.name), c.value, p.Code)
		if err == nil {
			t.Errorf("UPDATE of %s succeeded; the immutability trigger is not covering it", c.name)
			continue
		}
		if !strings.Contains(err.Error(), "immutable") {
			t.Errorf("unexpected error updating %s: %v", c.name, err)
		}
	}

	if _, body, err := st.Get(ctx, p.Code); err != nil || body != "original" {
		t.Errorf("after blocked updates: body = %q, err = %v", body, err)
	}
}

// accessed_at is bookkeeping rather than content, and LRU eviction needs it to
// move, so it is the one column the trigger deliberately leaves alone.
func TestAccessTimeRemainsWritable(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	p, err := st.Create(ctx, "content", "", NoExpiry)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := st.w.ExecContext(ctx,
		`UPDATE pastes SET accessed_at = ? WHERE seq = ?`, time.Now().Unix()+60, p.Seq); err != nil {
		t.Errorf("accessed_at must stay writable: %v", err)
	}
}

// The reader pool must reject writes even if a future query is written wrong.
func TestReadPoolIsQueryOnly(t *testing.T) {
	st := newTestStore(t, nil)

	if _, err := st.r.ExecContext(context.Background(), `DELETE FROM pastes`); err == nil {
		t.Fatal("write through the read pool succeeded; query_only is not applied")
	}
}

func TestSQLiteCacheIsApplied(t *testing.T) {
	st := newTestStore(t, nil)

	for name, db := range map[string]*sql.DB{"writer": st.w, "reader": st.r} {
		var pages int
		if err := db.QueryRow(`PRAGMA cache_size`).Scan(&pages); err != nil {
			t.Fatalf("%s: PRAGMA cache_size: %v", name, err)
		}
		// Negative means KiB; the test store asks for 8 MiB.
		if want := -8 * 1024; pages != want {
			t.Errorf("%s cache_size = %d, want %d", name, pages, want)
		}

		var mode string
		if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
			t.Fatalf("%s: PRAGMA journal_mode: %v", name, err)
		}
		if !strings.EqualFold(mode, "wal") {
			t.Errorf("%s journal_mode = %q, want wal", name, mode)
		}
	}
}

// The cap is the one hard guarantee, so it has to hold continuously rather than
// only at whatever moment the sweeper happens to run.
func TestSpaceCapHoldsOnEveryWrite(t *testing.T) {
	const cap = 64 << 10
	st := newTestStore(t, func(o *Options) { o.MaxBytes = cap })
	ctx := context.Background()

	for i := 0; i < 400; i++ {
		if _, err := st.Create(ctx, fmt.Sprintf("paste number %d %s", i, strings.Repeat("x", 900)), "", NoExpiry); err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
		stored := storedBytesNow(t, st)
		if stored > cap {
			t.Fatalf("after %d pastes the store holds %d bytes, over the %d cap", i+1, stored, cap)
		}
	}

	// The counter must agree with the rows it claims to describe.
	var actual int64
	if err := st.r.QueryRowContext(ctx, `SELECT COALESCE(SUM(bytes), 0) FROM pastes`).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if got := storedBytesNow(t, st); got != actual {
		t.Errorf("stored_bytes counter = %d, table sum = %d", got, actual)
	}
}

// Eviction has to remove the least recently *read* paste, not simply the oldest
// one, or a popular paste disappears while a forgotten one survives.
func TestEvictionRemovesLeastRecentlyUsed(t *testing.T) {
	st := newTestStore(t, func(o *Options) { o.MaxBytes = 1 << 20 })
	ctx := context.Background()

	filler := strings.Repeat("unique-filler-content ", 40)

	oldest, err := st.Create(ctx, "oldest paste "+filler, "", NoExpiry)
	if err != nil {
		t.Fatal(err)
	}
	middle, err := st.Create(ctx, "middle paste "+filler, "", NoExpiry)
	if err != nil {
		t.Fatal(err)
	}

	// Read the oldest so it becomes the most recently used, then persist that.
	if _, _, err := st.Get(ctx, oldest.Code); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FlushAccess(ctx); err != nil {
		t.Fatal(err)
	}
	// Access times have one-second resolution, so make the ordering explicit.
	if _, err := st.w.ExecContext(ctx,
		`UPDATE pastes SET accessed_at = ? WHERE seq = ?`, time.Now().Add(time.Hour).Unix(), oldest.Seq); err != nil {
		t.Fatal(err)
	}

	tx, err := st.w.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, _, err := evict(ctx, tx, 1); err != nil {
		t.Fatalf("evict: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if _, _, err := st.Get(ctx, middle.Code); !errors.Is(err, ErrNotFound) {
		t.Errorf("the untouched paste should have been evicted first, got %v", err)
	}
	if _, _, err := st.Get(ctx, oldest.Code); err != nil {
		t.Errorf("the recently read paste should have survived: %v", err)
	}
}

func TestRejectsPasteLargerThanTheWholeCap(t *testing.T) {
	st := newTestStore(t, func(o *Options) { o.MaxBytes = 64 })
	ctx := context.Background()

	// Random-ish text so compression cannot shrink it under the cap.
	var b strings.Builder
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&b, "%d%c", i*7919, rune('a'+i%26))
	}
	content := string([]rune(b.String())[:4000])

	if _, err := st.Create(ctx, content, "", NoExpiry); !errors.Is(err, ErrNoRoom) {
		t.Errorf("got %v, want ErrNoRoom", err)
	}
}

func TestTTLsAreDisabledByDefault(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	p, err := st.Create(ctx, "permanent", "", NoExpiry)
	if err != nil {
		t.Fatal(err)
	}
	// Backdate the row far beyond any plausible TTL.
	backdate(t, st, p.Seq, time.Now().Add(-10*365*24*time.Hour))

	result, err := st.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if result.Removed() != 0 {
		t.Errorf("sweep removed %d rows with both TTLs off", result.Removed())
	}
	if _, _, err := st.Get(ctx, p.Code); err != nil {
		t.Errorf("Get after sweep: %v", err)
	}
}

func TestAccessTTL(t *testing.T) {
	st := newTestStore(t, func(o *Options) { o.TTLAccess = time.Hour })
	ctx := context.Background()

	stale, err := st.Create(ctx, "stale", "", NoExpiry)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := st.Create(ctx, "fresh", "", NoExpiry)
	if err != nil {
		t.Fatal(err)
	}
	backdate(t, st, stale.Seq, time.Now().Add(-2*time.Hour))

	// Hidden from reads before the sweeper has even run.
	if _, _, err := st.Get(ctx, stale.Code); !errors.Is(err, ErrNotFound) {
		t.Errorf("paste past the access TTL is still readable: %v", err)
	}

	result, err := st.Sweep(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessExpired != 1 {
		t.Errorf("AccessExpired = %d, want 1", result.AccessExpired)
	}
	if _, _, err := st.Get(ctx, fresh.Code); err != nil {
		t.Errorf("the fresh paste was removed: %v", err)
	}
}

func TestCreateTTL(t *testing.T) {
	st := newTestStore(t, func(o *Options) { o.TTLCreate = time.Hour })
	ctx := context.Background()

	old, err := st.Create(ctx, "old", "", NoExpiry)
	if err != nil {
		t.Fatal(err)
	}
	// Only created_at moves: a paste read a second ago must still expire on age.
	if _, err := st.w.ExecContext(ctx,
		`UPDATE pastes SET accessed_at = ? WHERE seq = ?`, time.Now().Unix(), old.Seq); err != nil {
		t.Fatal(err)
	}
	if _, err := st.w.ExecContext(ctx,
		`DELETE FROM pastes WHERE seq = ?`, old.Seq); err != nil {
		t.Fatal(err)
	}
	if _, err := st.w.ExecContext(ctx,
		`INSERT INTO pastes (seq, code, body, codec, bytes, chars, raw_bytes, language, created_at, accessed_at)
		 VALUES (?, ?, ?, 0, 3, 3, 3, '', ?, ?)`,
		old.Seq, old.Code, []byte("abc"), time.Now().Add(-2*time.Hour).Unix(), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}

	if _, _, err := st.Get(ctx, old.Code); !errors.Is(err, ErrNotFound) {
		t.Errorf("paste past the creation TTL is still readable: %v", err)
	}
	result, err := st.Sweep(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.CreateExpired != 1 {
		t.Errorf("CreateExpired = %d, want 1", result.CreateExpired)
	}
}

// Reads must not write through, or every read would queue behind the single
// writer and the read/write split would buy nothing.
func TestReadsDoNotWriteUntilFlushed(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	p, err := st.Create(ctx, "content", "", NoExpiry)
	if err != nil {
		t.Fatal(err)
	}
	before := accessedAt(t, st, p.Seq)

	backdate(t, st, p.Seq, time.Now().Add(-time.Hour))
	if _, _, err := st.Get(ctx, p.Code); err != nil {
		t.Fatal(err)
	}
	if got := accessedAt(t, st, p.Seq); got >= before {
		t.Error("the read updated accessed_at synchronously; it should have been queued")
	}

	n, err := st.FlushAccess(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("FlushAccess reported %d rows, want 1", n)
	}
	if got := accessedAt(t, st, p.Seq); got < before {
		t.Errorf("accessed_at = %d after flush, want >= %d", got, before)
	}
}

func TestConcurrentCreatesGetDistinctCodes(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	const n = 200
	codes := make(chan string, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			p, err := st.Create(ctx, "concurrent", "", NoExpiry)
			if err != nil {
				errs <- err
				return
			}
			codes <- p.Code
		}()
	}

	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		select {
		case err := <-errs:
			t.Fatalf("concurrent Create: %v", err)
		case code := <-codes:
			if _, dup := seen[code]; dup {
				t.Fatalf("duplicate code %q under concurrency", code)
			}
			seen[code] = struct{}{}
		}
	}
}

// The dictionary exists to beat plain zstd on short inputs; if it ever stops
// doing that, the extra codec path is not earning its complexity.
func TestDictionaryBeatsPlainZstdOnShortPastes(t *testing.T) {
	codec, err := compress.New(compress.DefaultLevel)
	if err != nil {
		t.Fatal(err)
	}
	defer codec.Close()

	src := []byte(`{"level":"error","time":"2024-06-01T10:00:00Z","msg":"request failed","status":500}`)
	blob, which := codec.Compress(src)
	if which != compress.CodecZstdDict {
		t.Errorf("short log line used codec %d, expected the dictionary to win", which)
	}
	out, err := codec.Decompress(blob, which, len(src))
	if err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if string(out) != string(src) {
		t.Error("dictionary round-trip mismatch")
	}
	t.Logf("raw=%d stored=%d (%.1fx)", len(src), len(blob), float64(len(src))/float64(len(blob)))
}

func storedBytesNow(t *testing.T, st *Store) int64 {
	t.Helper()
	var n int64
	if err := st.r.QueryRow(`SELECT value FROM counters WHERE name = 'stored_bytes'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func accessedAt(t *testing.T, st *Store, seq uint64) int64 {
	t.Helper()
	var n int64
	if err := st.r.QueryRow(`SELECT accessed_at FROM pastes WHERE seq = ?`, seq).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// backdate moves both timestamps into the past, standing in for a paste that
// has simply been sitting there.
func backdate(t *testing.T, st *Store, seq uint64, when time.Time) {
	t.Helper()
	if _, err := st.w.Exec(`UPDATE pastes SET accessed_at = ? WHERE seq = ?`, when.Unix(), seq); err != nil {
		t.Fatal(err)
	}
}

// The limit matches the classic haste-server's 400000, which is large enough
// that the paths sized for a few kilobytes have to be exercised at that scale.
func TestAcceptsAClassicSizedPaste(t *testing.T) {
	if testing.Short() {
		t.Skip("compresses 400k characters; slow under -short")
	}

	const limit = 400_000
	st := newTestStore(t, func(o *Options) {
		o.MaxChars = limit
		// The cap has to admit one maximal paste, or every write evicts itself.
		o.MaxBytes = 64 << 20
	})
	ctx := context.Background()

	var b strings.Builder
	for i := 0; b.Len() < limit*2; i++ {
		b.WriteString(corpus.Log(i))
	}
	content := string([]rune(b.String())[:limit])

	p, err := st.Create(ctx, content, "log", NoExpiry)
	if err != nil {
		t.Fatalf("Create at the limit: %v", err)
	}
	if p.Chars != limit {
		t.Errorf("Chars = %d, want %d", p.Chars, limit)
	}

	got, body, err := st.Get(ctx, p.Code)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if body != content {
		t.Error("a maximum-size paste did not survive the round trip")
	}
	if got.StoredSize >= got.RawBytes {
		t.Errorf("stored %d B for %d B of input; compression did nothing", got.StoredSize, got.RawBytes)
	}
	t.Logf("%d chars: %d B raw -> %d B stored (%.1fx)",
		limit, got.RawBytes, got.StoredSize, float64(got.RawBytes)/float64(got.StoredSize))

	// One character over is still refused, at the new limit as at the old.
	if _, err := st.Create(ctx, content+"x", "", NoExpiry); !errors.Is(err, ErrTooLarge) {
		t.Errorf("one character over the limit: got %v, want ErrTooLarge", err)
	}
}

func TestLifetimeIsRecordedAsAnInstant(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	before := time.Now()
	p, err := st.Create(ctx, "temporary", "", 6*time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// An absolute deadline, not a duration counted from whenever the row is
	// next read: a restart must not extend a paste's life.
	want := before.Add(6 * time.Hour)
	if diff := p.ExpiresAt.Sub(want); diff < -2*time.Second || diff > 2*time.Second {
		t.Errorf("ExpiresAt = %s, want within 2s of %s", p.ExpiresAt, want)
	}

	// And it survives the round trip rather than living only in the response.
	got, _, err := st.Get(ctx, p.Code)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.ExpiresAt.Equal(p.ExpiresAt) {
		t.Errorf("ExpiresAt after reload = %s, want %s", got.ExpiresAt, p.ExpiresAt)
	}
}

func TestNoLifetimeLeavesTheColumnNull(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	p, err := st.Create(ctx, "permanent", "", NoExpiry)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !p.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %s, want zero for a paste that asked for no lifetime", p.ExpiresAt)
	}

	// NULL rather than 0: "no lifetime" has to be distinguishable from "expired
	// at the epoch", or every query would need a sentinel comparison.
	var expires sql.NullInt64
	if err := st.w.QueryRowContext(ctx,
		`SELECT expires_at FROM pastes WHERE seq = ?`, p.Seq).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	if expires.Valid {
		t.Errorf("expires_at = %d, want NULL", expires.Int64)
	}
}

// The ladder is the contract, not a range: a value between two rungs is
// refused rather than quietly behaving like the one below it.
func TestCreateAcceptsOnlyTheLadder(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	for _, ttl := range []time.Duration{
		time.Second,
		time.Hour - time.Second,
		time.Hour + time.Second,       // just off the first rung
		2 * time.Hour,                 // between rungs, and a plausible ask
		9 * time.Hour,                 // between rungs
		31 * 24 * time.Hour,           // past the last one
		30*24*time.Hour - time.Second, // just short of it
		-time.Hour,
	} {
		if _, err := st.Create(ctx, "content", "", ttl); !errors.Is(err, ErrBadTTL) {
			t.Errorf("Create with ttl %s: err = %v, want ErrBadTTL", ttl, err)
		}
	}

	for _, ttl := range append([]time.Duration{NoExpiry}, TTLOptions...) {
		if _, err := st.Create(ctx, "content", "", ttl); err != nil {
			t.Errorf("Create with ttl %s: %v", ttl, err)
		}
	}
}

func TestTTLOptionsAreOrderedAndNamed(t *testing.T) {
	for i, d := range TTLOptions {
		if d <= 0 {
			t.Errorf("TTLOptions[%d] = %s, want a positive duration", i, d)
		}
		if i > 0 && d <= TTLOptions[i-1] {
			t.Errorf("TTLOptions[%d] = %s is not above %s", i, d, TTLOptions[i-1])
		}
	}
	// The rejection message quotes this, so it has to read like the picker's
	// labels rather than like a Duration ("24h0m0s").
	if want := "1h, 6h, 12h, 1d, 3d, 7d, 14d, 30d"; ttlList != want {
		t.Errorf("ttlList = %q, want %q", ttlList, want)
	}
	if !AllowedTTL(NoExpiry) {
		t.Error("NoExpiry is not allowed")
	}
}

// A lifetime has to hold the moment it runs out, not whenever the hourly sweep
// next happens to run — otherwise the time shown to the user is fiction.
func TestExpiredPasteStopsBeingServedBeforeTheSweep(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	live, err := st.Create(ctx, "still here", "", TTLOptions[0])
	if err != nil {
		t.Fatal(err)
	}
	dead := expiringRow(t, st, "gone", time.Now().Add(-time.Minute))

	if _, _, err := st.Get(ctx, dead); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get on an expired paste: err = %v, want ErrNotFound", err)
	}
	if _, err := st.Meta(ctx, dead); !errors.Is(err, ErrNotFound) {
		t.Errorf("Meta on an expired paste: err = %v, want ErrNotFound", err)
	}
	if _, _, err := st.Get(ctx, live.Code); err != nil {
		t.Errorf("Get on a paste still within its lifetime: %v", err)
	}

	result, err := st.Sweep(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Expired != 1 {
		t.Errorf("Expired = %d, want 1", result.Expired)
	}
	if result.SpaceEvicted+result.AccessExpired+result.CreateExpired != 0 {
		t.Errorf("another rule also fired: %+v", result)
	}

	var rows int
	if err := st.w.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pastes WHERE code = ?`, dead).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("the expired row is still in the table after a sweep")
	}
}

// The byte counter drives the space cap on every write, so a sweep that frees
// bytes without decrementing it would shrink the usable database on each pass.
func TestSweepingAnExpiredPasteReturnsItsBytes(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	before := storedBytesNow(t, st)
	expiringRow(t, st, strings.Repeat("expired ", 200), time.Now().Add(-time.Minute))
	if grew := storedBytesNow(t, st) - before; grew <= 0 {
		t.Fatalf("stored_bytes did not grow after the insert: %d", grew)
	}

	if _, err := st.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if after := storedBytesNow(t, st); after != before {
		t.Errorf("stored_bytes = %d after sweeping the expired paste, want %d", after, before)
	}
}

// expiringRow stores content and then rewrites the row with the given deadline,
// which is the only way to reach the past: Create validates the lifetime, and
// the immutability trigger refuses to let expires_at be updated afterwards.
func expiringRow(t *testing.T, st *Store, content string, at time.Time) string {
	t.Helper()
	ctx := context.Background()

	p, err := st.Create(ctx, content, "", NoExpiry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.w.ExecContext(ctx,
		`UPDATE pastes SET expires_at = ? WHERE seq = ?`, at.Unix(), p.Seq); err == nil {
		t.Fatal("expires_at was updatable; the immutability trigger is not covering it")
	}
	if _, err := st.w.ExecContext(ctx, `DELETE FROM pastes WHERE seq = ?`, p.Seq); err != nil {
		t.Fatal(err)
	}
	if _, err := st.w.ExecContext(ctx,
		`INSERT INTO pastes (seq, code, body, codec, bytes, chars, raw_bytes, language, created_at, accessed_at, expires_at)
		 VALUES (?, ?, ?, 0, ?, ?, ?, '', ?, ?, ?)`,
		p.Seq, p.Code, []byte(content), len(content), len(content), len(content),
		p.CreatedAt.Unix(), p.CreatedAt.Unix(), at.Unix()); err != nil {
		t.Fatal(err)
	}
	return p.Code
}

// A database written before temporary pastes existed has to keep working, with
// its rows intact: CREATE TABLE IF NOT EXISTS does nothing to a table that is
// already there, so nothing but the migration adds the column.
func TestOpensADatabaseWrittenBeforeExpiries(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "haste.db")

	// The shipped schema as it stood, trigger included, so the test exercises
	// the real starting point rather than a convenient subset of it.
	legacy, err := sql.Open("sqlite", dsn(path, 8, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `
		CREATE TABLE counters (name TEXT PRIMARY KEY, value INTEGER NOT NULL) STRICT;
		INSERT INTO counters (name, value) VALUES ('paste_seq', 1), ('stored_bytes', 3);
		CREATE TABLE pastes (
		    seq INTEGER PRIMARY KEY, code TEXT NOT NULL, body BLOB NOT NULL,
		    codec INTEGER NOT NULL, bytes INTEGER NOT NULL, chars INTEGER NOT NULL,
		    raw_bytes INTEGER NOT NULL, language TEXT NOT NULL DEFAULT '',
		    created_at INTEGER NOT NULL, accessed_at INTEGER NOT NULL
		) STRICT;
		CREATE UNIQUE INDEX pastes_code_idx ON pastes (code);
		CREATE TRIGGER pastes_immutable
		BEFORE UPDATE OF seq, code, body, codec, bytes, chars, raw_bytes, language, created_at
		ON pastes BEGIN SELECT RAISE(ABORT, 'pastes are immutable'); END;
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx,
		`INSERT INTO pastes VALUES (1, 'legacyAA', ?, 0, 3, 3, 3, 'go', ?, ?)`,
		[]byte("abc"), time.Now().Unix(), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	codec, err := compress.New(compress.DefaultLevel)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(codec.Close)

	st, err := Open(ctx, Options{
		Path: path, CacheMB: 8, ReadPool: 2, MaxChars: 4000, Codec: codec,
		IDs: id.NewGenerator([]byte("test-secret"), id.DefaultMinLen, nil),
	})
	if err != nil {
		t.Fatalf("Open over a pre-expiry database: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	// The existing paste survives, and reads as having no lifetime rather than
	// one that already ran out.
	old, err := st.Meta(ctx, "legacyAA")
	if err != nil {
		t.Fatalf("Meta on the migrated row: %v", err)
	}
	if !old.ExpiresAt.IsZero() {
		t.Errorf("migrated row has ExpiresAt = %s, want none", old.ExpiresAt)
	}

	// The new column works for writes...
	fresh, err := st.Create(ctx, "temporary", "", TTLOptions[0])
	if err != nil {
		t.Fatalf("Create with a lifetime after migrating: %v", err)
	}
	if fresh.ExpiresAt.IsZero() {
		t.Error("a paste created with a lifetime came back with none")
	}

	// ...and the trigger was replaced, not left as the older build wrote it.
	if _, err := st.w.ExecContext(ctx,
		`UPDATE pastes SET expires_at = 0 WHERE seq = ?`, fresh.Seq); err == nil {
		t.Error("expires_at is updatable; the old trigger survived the migration")
	}
}
