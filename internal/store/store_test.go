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
	p, err := st.Create(ctx, content, "go")
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
		p, err := st.Create(ctx, "x", "")
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

	if _, err := st.Create(ctx, "", ""); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty paste: got %v, want ErrEmpty", err)
	}
	// 4001 multi-byte runes: proves the cap counts characters, not bytes.
	if _, err := st.Create(ctx, strings.Repeat("界", 4001), ""); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversized paste: got %v, want ErrTooLarge", err)
	}
	if _, err := st.Create(ctx, strings.Repeat("界", 4000), ""); err != nil {
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

	p, err := st.Create(ctx, "original", "")
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

	p, err := st.Create(ctx, "content", "")
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
		if _, err := st.Create(ctx, fmt.Sprintf("paste number %d %s", i, strings.Repeat("x", 900)), ""); err != nil {
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

	oldest, err := st.Create(ctx, "oldest paste "+filler, "")
	if err != nil {
		t.Fatal(err)
	}
	middle, err := st.Create(ctx, "middle paste "+filler, "")
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

	if _, err := st.Create(ctx, content, ""); !errors.Is(err, ErrNoRoom) {
		t.Errorf("got %v, want ErrNoRoom", err)
	}
}

func TestTTLsAreDisabledByDefault(t *testing.T) {
	st := newTestStore(t, nil)
	ctx := context.Background()

	p, err := st.Create(ctx, "permanent", "")
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

	stale, err := st.Create(ctx, "stale", "")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := st.Create(ctx, "fresh", "")
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

	old, err := st.Create(ctx, "old", "")
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

	p, err := st.Create(ctx, "content", "")
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
			p, err := st.Create(ctx, "concurrent", "")
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

	p, err := st.Create(ctx, content, "log")
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
	if _, err := st.Create(ctx, content+"x", ""); !errors.Is(err, ErrTooLarge) {
		t.Errorf("one character over the limit: got %v, want ErrTooLarge", err)
	}
}
