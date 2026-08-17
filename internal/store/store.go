// Package store persists pastes in SQLite with separate read and write pools.
//
// SQLite in WAL mode allows one writer and many concurrent readers. Modelling
// that as two *sql.DB handles — a single-connection writer holding an immediate
// transaction lock, and a pool of query-only readers — keeps reads off the
// writer's lock entirely and removes the "database is locked" class of error
// that comes from letting a shared pool interleave both.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/YuYu1015/haste-server/internal/compress"
	"github.com/YuYu1015/haste-server/internal/id"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// mmapBytes is shared across connections (unlike the page cache), so it is set
// generously and independently of the configured cache size.
const mmapBytes = 256 << 20

var (
	// ErrNotFound means the code was never issued, or its paste has expired.
	ErrNotFound = errors.New("store: paste not found")
	// ErrEmpty means the submitted content had no content.
	ErrEmpty = errors.New("store: paste is empty")
	// ErrTooLarge means the submission exceeded the configured character limit.
	ErrTooLarge = errors.New("store: paste exceeds character limit")
)

// Paste is a stored paste's metadata. The body is returned separately, so
// listing metadata never pays for decompression.
type Paste struct {
	Seq        uint64
	Code       string
	Language   string
	Chars      int
	RawBytes   int
	StoredSize int
	CreatedAt  time.Time
	ExpiresAt  time.Time // zero means the paste never expires
}

// Stats summarises the corpus, for the health/stats endpoint.
type Stats struct {
	Count      int64 `json:"count"`
	RawBytes   int64 `json:"rawBytes"`
	StoredSize int64 `json:"storedBytes"`
}

// Options configures a Store.
type Options struct {
	Path      string
	CacheMB   int // page cache per connection
	ReadPool  int // read-only connections
	MaxChars  int
	Retention time.Duration // 0 keeps pastes forever
	Codec     *compress.Codec
	IDs       *id.Generator
}

// Store owns both database handles and the compression codec.
type Store struct {
	w    *sql.DB // exactly one connection: SQLite permits a single writer
	r    *sql.DB // query_only pool
	opts Options
}

// Open prepares the database file, applies the schema, and returns a ready store.
func Open(ctx context.Context, opts Options) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(opts.Path), 0o750); err != nil {
		return nil, fmt.Errorf("store: create data dir: %w", err)
	}

	w, err := sql.Open("sqlite", dsn(opts.Path, opts.CacheMB, false))
	if err != nil {
		return nil, fmt.Errorf("store: open writer: %w", err)
	}
	// One connection, kept alive: more would only contend for the write lock,
	// and recycling it would drop a 48 MiB warm page cache on the floor.
	w.SetMaxOpenConns(1)
	w.SetMaxIdleConns(1)
	w.SetConnMaxLifetime(0)

	if err := w.PingContext(ctx); err != nil {
		w.Close()
		return nil, fmt.Errorf("store: connect writer: %w", err)
	}
	if _, err := w.ExecContext(ctx, schemaSQL); err != nil {
		w.Close()
		return nil, fmt.Errorf("store: apply schema: %w", err)
	}

	// Readers are opened read-write at the file level — WAL readers need to
	// update the shared-memory index — but pinned to query_only so SQLite
	// itself rejects any write that reaches them.
	r, err := sql.Open("sqlite", dsn(opts.Path, opts.CacheMB, true))
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("store: open readers: %w", err)
	}
	r.SetMaxOpenConns(opts.ReadPool)
	r.SetMaxIdleConns(opts.ReadPool)
	r.SetConnMaxLifetime(0)

	if err := r.PingContext(ctx); err != nil {
		w.Close()
		r.Close()
		return nil, fmt.Errorf("store: connect readers: %w", err)
	}
	return &Store{w: w, r: r, opts: opts}, nil
}

// Close releases both pools.
func (s *Store) Close() error {
	return errors.Join(s.r.Close(), s.w.Close())
}

// Create compresses and stores content, allocating the shortest unused code.
func (s *Store) Create(ctx context.Context, content, language string) (*Paste, error) {
	chars := utf8.RuneCountInString(content)
	switch {
	case chars == 0:
		return nil, ErrEmpty
	case chars > s.opts.MaxChars:
		return nil, fmt.Errorf("%w: %d > %d", ErrTooLarge, chars, s.opts.MaxChars)
	}

	raw := []byte(content)
	body, codec := s.opts.Codec.Compress(raw)

	now := time.Now().UTC().Truncate(time.Second)
	p := &Paste{
		Language:   language,
		Chars:      chars,
		RawBytes:   len(raw),
		StoredSize: len(body),
		CreatedAt:  now,
	}
	var expires int64
	if s.opts.Retention > 0 {
		p.ExpiresAt = now.Add(s.opts.Retention)
		expires = p.ExpiresAt.Unix()
	}

	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: begin write: %w", err)
	}
	defer tx.Rollback()

	// Bump the counter until it yields a code that does not shadow a fixed
	// route. Burning a sequence number is cheaper than a lookup-time exception.
	for {
		var seq uint64
		if err := tx.QueryRowContext(ctx,
			`UPDATE counters SET value = value + 1 WHERE name = 'paste_seq' RETURNING value`,
		).Scan(&seq); err != nil {
			return nil, fmt.Errorf("store: allocate sequence: %w", err)
		}
		code, err := s.opts.IDs.Code(seq)
		if err != nil {
			return nil, err
		}
		if !s.opts.IDs.IsReserved(code) {
			p.Seq, p.Code = seq, code
			break
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO pastes (seq, code, body, codec, chars, raw_bytes, language, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Seq, p.Code, body, codec, p.Chars, p.RawBytes, p.Language, p.CreatedAt.Unix(), expires,
	); err != nil {
		return nil, fmt.Errorf("store: insert paste: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit paste: %w", err)
	}
	return p, nil
}

// Get returns a paste and its decompressed body. Expired rows are treated as
// missing even before the sweeper has removed them.
func (s *Store) Get(ctx context.Context, code string) (*Paste, string, error) {
	if !id.Valid(code) {
		return nil, "", ErrNotFound
	}

	var (
		p         Paste
		body      []byte
		codec     int
		created   int64
		expires   int64
	)
	err := s.r.QueryRowContext(ctx,
		`SELECT seq, code, body, codec, chars, raw_bytes, language, created_at, expires_at
		   FROM pastes
		  WHERE code = ? AND (expires_at = 0 OR expires_at > ?)`,
		code, time.Now().Unix(),
	).Scan(&p.Seq, &p.Code, &body, &codec, &p.Chars, &p.RawBytes, &p.Language, &created, &expires)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, "", ErrNotFound
	case err != nil:
		return nil, "", fmt.Errorf("store: read paste: %w", err)
	}

	p.StoredSize = len(body)
	p.CreatedAt = time.Unix(created, 0).UTC()
	if expires > 0 {
		p.ExpiresAt = time.Unix(expires, 0).UTC()
	}

	// The limit is the recorded size: anything else is a corrupt row, not a
	// decode this process should spend memory on.
	content, err := s.opts.Codec.Decompress(body, codec, p.RawBytes)
	if err != nil {
		return nil, "", err
	}
	return &p, string(content), nil
}

// PurgeExpired deletes everything past its retention window and reports how
// many rows went. It also checkpoints the WAL, so a large sweep actually
// returns space instead of parking it in the log.
func (s *Store) PurgeExpired(ctx context.Context) (int64, error) {
	res, err := s.w.ExecContext(ctx,
		`DELETE FROM pastes WHERE expires_at > 0 AND expires_at <= ?`, time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("store: purge expired: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: purge result: %w", err)
	}
	if n > 0 {
		if _, err := s.w.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			return n, fmt.Errorf("store: checkpoint after purge: %w", err)
		}
	}
	// Cheap, bounded re-analysis; keeps the planner honest as the table grows.
	if _, err := s.w.ExecContext(ctx, `PRAGMA optimize`); err != nil {
		return n, fmt.Errorf("store: optimize after purge: %w", err)
	}
	return n, nil
}

// Stats summarises live pastes.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.r.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(raw_bytes), 0), COALESCE(SUM(LENGTH(body)), 0)
		   FROM pastes
		  WHERE expires_at = 0 OR expires_at > ?`, time.Now().Unix(),
	).Scan(&st.Count, &st.RawBytes, &st.StoredSize)
	if err != nil {
		return Stats{}, fmt.Errorf("store: stats: %w", err)
	}
	return st, nil
}

// dsn builds a connection string. Pragmas set here apply per connection, which
// is what makes the reader pool and the writer independently tunable.
func dsn(path string, cacheMB int, readOnly bool) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	// Negative cache_size is in KiB rather than pages, so the budget holds
	// regardless of page size.
	q.Add("_pragma", fmt.Sprintf("cache_size(-%d)", cacheMB*1024))
	q.Add("_pragma", "temp_store(MEMORY)")
	q.Add("_pragma", fmt.Sprintf("mmap_size(%d)", mmapBytes))
	if readOnly {
		q.Add("_pragma", "query_only(true)")
	} else {
		// Take the write lock when the transaction opens rather than when it
		// first writes, so a busy database fails fast instead of mid-statement.
		q.Set("_txlock", "immediate")
	}
	return "file:" + path + "?" + q.Encode()
}
