package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/YuYu1015/haste-server/internal/compress"
	"github.com/YuYu1015/haste-server/internal/id"
)

func newTestStore(t *testing.T, retention time.Duration) *Store {
	t.Helper()

	codec, err := compress.New(19)
	if err != nil {
		t.Fatalf("compress.New: %v", err)
	}
	t.Cleanup(codec.Close)

	st, err := Open(context.Background(), Options{
		Path:      filepath.Join(t.TempDir(), "haste.db"),
		CacheMB:   8,
		ReadPool:  4,
		MaxChars:  4000,
		Retention: retention,
		Codec:     codec,
		IDs:       id.NewGenerator([]byte("test-secret"), []string{"api", "raw"}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestCreateAndGetRoundTrip(t *testing.T) {
	st := newTestStore(t, 30*24*time.Hour)
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
	if got.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set when retention is configured")
	}
}

func TestFirstCodesAreSingleCharacter(t *testing.T) {
	st := newTestStore(t, time.Hour)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		p, err := st.Create(ctx, "x", "")
		if err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
		if len(p.Code) != 1 {
			t.Fatalf("paste #%d got code %q; the first 62 should be one character", i, p.Code)
		}
	}
}

func TestLimits(t *testing.T) {
	st := newTestStore(t, time.Hour)
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
	st := newTestStore(t, time.Hour)
	ctx := context.Background()

	for _, code := range []string{"zzzz", "", "../../etc/passwd", "not a code"} {
		if _, _, err := st.Get(ctx, code); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q): got %v, want ErrNotFound", code, err)
		}
	}
}

// "Locked" has to mean locked in the database, not merely unexposed by the API.
func TestPastesAreImmutable(t *testing.T) {
	st := newTestStore(t, time.Hour)
	ctx := context.Background()

	p, err := st.Create(ctx, "original", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = st.w.ExecContext(ctx, `UPDATE pastes SET body = ? WHERE code = ?`, []byte("tampered"), p.Code)
	if err == nil {
		t.Fatal("UPDATE on pastes succeeded; the immutability trigger is not firing")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Errorf("unexpected error from UPDATE: %v", err)
	}

	if _, body, err := st.Get(ctx, p.Code); err != nil || body != "original" {
		t.Errorf("after blocked UPDATE: body = %q, err = %v", body, err)
	}
}

// The reader pool must reject writes even if a future query is written wrong.
func TestReadPoolIsQueryOnly(t *testing.T) {
	st := newTestStore(t, time.Hour)

	_, err := st.r.ExecContext(context.Background(), `DELETE FROM pastes`)
	if err == nil {
		t.Fatal("write through the read pool succeeded; query_only is not applied")
	}
}

func TestSQLiteCacheIsApplied(t *testing.T) {
	st := newTestStore(t, time.Hour)

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

func TestExpiredPastesAreHiddenThenPurged(t *testing.T) {
	st := newTestStore(t, time.Hour)
	ctx := context.Background()

	live, err := st.Create(ctx, "still here", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Insert a row that expired an hour ago. Writing it directly beats sleeping
	// through a real retention window, and UPDATE is blocked by design.
	body, codec := st.opts.Codec.Compress([]byte("gone"))
	past := time.Now().Add(-time.Hour).Unix()
	if _, err := st.w.ExecContext(ctx,
		`INSERT INTO pastes (seq, code, body, codec, chars, raw_bytes, language, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, '', ?, ?)`,
		999999, "expired", body, codec, 4, 4, past, past); err != nil {
		t.Fatalf("seed expired row: %v", err)
	}

	if _, _, err := st.Get(ctx, "expired"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired paste is still readable: %v", err)
	}

	n, err := st.PurgeExpired(ctx)
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if n != 1 {
		t.Errorf("PurgeExpired deleted %d rows, want 1", n)
	}
	if _, _, err := st.Get(ctx, live.Code); err != nil {
		t.Errorf("purge removed a live paste: %v", err)
	}
}

func TestRetentionZeroKeepsForever(t *testing.T) {
	st := newTestStore(t, 0)
	ctx := context.Background()

	p, err := st.Create(ctx, "permanent", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !p.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero when retention is disabled", p.ExpiresAt)
	}
	if n, err := st.PurgeExpired(ctx); err != nil || n != 0 {
		t.Errorf("PurgeExpired = %d, %v; want 0, nil", n, err)
	}
	if _, _, err := st.Get(ctx, p.Code); err != nil {
		t.Errorf("Get after purge: %v", err)
	}
}

func TestConcurrentCreatesGetDistinctCodes(t *testing.T) {
	st := newTestStore(t, time.Hour)
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
	codec, err := compress.New(19)
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
