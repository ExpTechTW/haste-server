package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/YuYu1015/haste-server/internal/compress"
	"github.com/YuYu1015/haste-server/internal/corpus"
	"github.com/YuYu1015/haste-server/internal/id"
)

// TestStorageFootprint measures what a full-size paste actually costs on disk,
// so the space cap can be reasoned about in pastes rather than in bytes.
//
// It reports rather than asserts, apart from a loose ceiling that would catch a
// change making rows dramatically more expensive.
func TestStorageFootprint(t *testing.T) {
	if testing.Short() {
		t.Skip("measures real disk usage; slow under -short")
	}

	const pastes = 500

	for _, kind := range corpus.Kinds {
		t.Run(kind.Name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "haste.db")

			codec, err := compress.New(compress.DefaultLevel)
			if err != nil {
				t.Fatal(err)
			}
			defer codec.Close()

			st, err := Open(context.Background(), Options{
				Path:     path,
				CacheMB:  8,
				ReadPool: 2,
				MaxChars: corpus.Chars,
				Codec:    codec,
				IDs:      id.NewGenerator([]byte("footprint"), id.DefaultMinLen, nil),
			})
			if err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()
			var rawTotal, storedTotal int
			for i := 0; i < pastes; i++ {
				p, err := st.Create(ctx, kind.Generate(i), "", "", NoExpiry)
				if err != nil {
					t.Fatalf("create %d: %v", i, err)
				}
				rawTotal += p.RawBytes
				storedTotal += p.StoredSize
			}

			// Checkpoint so the WAL's contents land in the main database file.
			if _, err := st.w.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
				t.Fatal(err)
			}
			st.Close()

			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}

			onDisk := float64(info.Size()) / pastes
			t.Logf("raw %d B -> zstd %d B (%.1fx) | on disk %.0f B/paste | 1 GiB holds ~%s pastes",
				rawTotal/pastes,
				storedTotal/pastes,
				float64(rawTotal)/float64(storedTotal),
				onDisk,
				humanCount(int64(float64(1<<30)/onDisk)),
			)

			// A paste is at most 4 bytes per character in UTF-8; approaching
			// that on disk would mean compression or row overhead has regressed.
			if limit := float64(corpus.Chars) * 3.5; onDisk > limit {
				t.Errorf("%.0f B/paste on disk exceeds the %.0f B bound", onDisk, limit)
			}
		})
	}
}

func humanCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}
