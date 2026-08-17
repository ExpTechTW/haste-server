// Package store persists pastes in SQLite with separate read and write pools.
//
// SQLite in WAL mode allows one writer and many concurrent readers. Modelling
// that as two *sql.DB handles — a single-connection writer holding an immediate
// transaction lock, and a pool of query-only readers — keeps reads off the
// writer's lock entirely and removes the "database is locked" class of error
// that comes from letting a shared pool interleave both.
//
// Retention is a budget rather than a promise. A byte cap is enforced on every
// write, evicting least-recently-used pastes to make room, so the database
// cannot outgrow its allowance no matter how fast pastes arrive. Two optional
// TTLs — one on last access, one on creation — trim further; both are off
// unless configured.
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
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/YuYu1015/haste-server/internal/compress"
	"github.com/YuYu1015/haste-server/internal/id"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

const (
	// mmapBytes is shared across connections (unlike the page cache), so it is
	// set generously and independently of the configured cache size.
	mmapBytes = 256 << 20

	// evictBatch bounds how many candidates are examined per eviction round. In
	// the steady state of a full database only one or two rows have to go, so
	// this is a ceiling rather than a typical cost.
	evictBatch = 64
)

var (
	// ErrNotFound means the code was never issued, or its paste is gone.
	ErrNotFound = errors.New("store: paste not found")
	// ErrEmpty means the submitted content had no content.
	ErrEmpty = errors.New("store: paste is empty")
	// ErrTooLarge means the submission exceeded the configured character limit.
	ErrTooLarge = errors.New("store: paste exceeds character limit")
	// ErrNoRoom means a single paste could not fit inside the whole byte cap.
	ErrNoRoom = errors.New("store: paste does not fit within the storage cap")
	// ErrBusy means too many writes are already queued and this one was shed
	// rather than added to a line that is no longer worth joining.
	ErrBusy = errors.New("store: write queue is full")
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
}

// Stats summarises the corpus, for the stats endpoint.
type Stats struct {
	Count      int64
	RawBytes   int64
	StoredSize int64
	MaxBytes   int64
}

// SweepResult reports what one cleanup pass removed.
type SweepResult struct {
	AccessFlushed int   // rows whose access time was written back
	SpaceEvicted  int64 // rows removed to stay under the byte cap
	AccessExpired int64 // rows removed by the last-access TTL
	CreateExpired int64 // rows removed by the creation TTL
	StoredBytes   int64 // live total once the pass finished
}

// Removed reports whether the pass deleted anything.
func (r SweepResult) Removed() int64 {
	return r.SpaceEvicted + r.AccessExpired + r.CreateExpired
}

// Options configures a Store.
type Options struct {
	Path     string
	CacheMB  int // page cache per connection
	ReadPool int // read-only connections
	MaxChars int

	MaxBytes  int64         // hard cap on stored bytes; 0 = unlimited
	TTLAccess time.Duration // evict when untouched for this long; 0 = disabled
	TTLCreate time.Duration // evict when older than this; 0 = disabled

	// Admission control for writes. The database is not the constraint here —
	// SetMaxOpenConns(1) already serialises transactions and an insert costs
	// about 100µs — but compressing a paste at zstd-19 costs closer to a
	// millisecond of CPU, and that runs before the transaction with no natural
	// bound. Without a limit, load turns into unbounded parallel compression and
	// an unbounded queue of goroutines holding request buffers.
	WriteConcurrency int // simultaneous writes; 0 = unlimited
	WriteQueue       int // callers allowed to wait; beyond this, ErrBusy

	Codec *compress.Codec
	IDs   *id.Generator
}

// Store owns both database handles and the compression codec.
type Store struct {
	w    *sql.DB // exactly one connection: SQLite permits a single writer
	r    *sql.DB // query_only pool
	opts Options

	// Reads record access times here instead of writing through. Touching a row
	// on every read would funnel reads through the single writer and undo the
	// point of the split; batching means one small transaction per flush.
	accessMu sync.Mutex
	access   map[uint64]int64

	// writeSlots bounds how many writes compress at once; queued counts the
	// callers waiting for one, so the line itself has a length.
	writeSlots chan struct{}
	queued     atomic.Int64
	maxQueued  int64
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
	// and recycling it would drop a warm page cache on the floor.
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
	// One scan at startup so the running total can never drift from reality.
	if _, err := w.ExecContext(ctx,
		`UPDATE counters SET value = COALESCE((SELECT SUM(bytes) FROM pastes), 0)
		  WHERE name = 'stored_bytes'`); err != nil {
		w.Close()
		return nil, fmt.Errorf("store: reconcile stored bytes: %w", err)
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
	s := &Store{w: w, r: r, opts: opts, access: make(map[uint64]int64)}
	if opts.WriteConcurrency > 0 {
		s.writeSlots = make(chan struct{}, opts.WriteConcurrency)
		s.maxQueued = int64(opts.WriteQueue)
	}
	return s, nil
}

// acquireWrite admits one write, or refuses immediately when the queue is
// already longer than it is worth joining. Shedding beats queueing here: a
// caller that waits past its own deadline costs the server the work anyway and
// gives the client nothing.
func (s *Store) acquireWrite(ctx context.Context) error {
	if s.writeSlots == nil {
		return nil
	}
	queued := s.queued.Add(1)
	defer s.queued.Add(-1)
	if s.maxQueued > 0 && queued > s.maxQueued {
		return ErrBusy
	}

	select {
	case s.writeSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		// The client gave up; releasing the place in line costs nothing.
		return ctx.Err()
	}
}

func (s *Store) releaseWrite() {
	if s.writeSlots != nil {
		<-s.writeSlots
	}
}

// Close flushes pending access times and releases both pools.
func (s *Store) Close() error {
	flushErr := error(nil)
	if _, err := s.FlushAccess(context.Background()); err != nil {
		flushErr = err
	}
	return errors.Join(flushErr, s.r.Close(), s.w.Close())
}

// Create compresses and stores content, allocating the shortest unused code and
// evicting least-recently-used pastes if the byte cap needs the room.
func (s *Store) Create(ctx context.Context, content, language string) (*Paste, error) {
	chars := utf8.RuneCountInString(content)
	switch {
	case chars == 0:
		return nil, ErrEmpty
	case chars > s.opts.MaxChars:
		return nil, fmt.Errorf("%w: %d > %d", ErrTooLarge, chars, s.opts.MaxChars)
	}

	// Admission is taken before compressing, because compression is the
	// expensive half and the part that has no bound of its own.
	if err := s.acquireWrite(ctx); err != nil {
		return nil, err
	}
	defer s.releaseWrite()

	raw := []byte(content)
	body, codec := s.opts.Codec.Compress(raw)
	size := int64(len(body))

	if s.opts.MaxBytes > 0 && size > s.opts.MaxBytes {
		return nil, fmt.Errorf("%w: %d > %d", ErrNoRoom, size, s.opts.MaxBytes)
	}

	now := time.Now().UTC().Truncate(time.Second)
	p := &Paste{
		Language:   language,
		Chars:      chars,
		RawBytes:   len(raw),
		StoredSize: len(body),
		CreatedAt:  now,
	}

	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: begin write: %w", err)
	}
	defer tx.Rollback()

	// Make room before writing, so the cap holds continuously rather than only
	// at whatever moment the sweeper happens to run.
	if s.opts.MaxBytes > 0 {
		stored, err := storedBytes(ctx, tx)
		if err != nil {
			return nil, err
		}
		if over := stored + size - s.opts.MaxBytes; over > 0 {
			if _, _, err := evict(ctx, tx, over); err != nil {
				return nil, err
			}
		}
	}

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
		`INSERT INTO pastes (seq, code, body, codec, bytes, chars, raw_bytes, language, created_at, accessed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Seq, p.Code, body, codec, size, p.Chars, p.RawBytes, p.Language,
		p.CreatedAt.Unix(), p.CreatedAt.Unix(),
	); err != nil {
		return nil, fmt.Errorf("store: insert paste: %w", err)
	}
	if err := addStoredBytes(ctx, tx, size); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit paste: %w", err)
	}
	return p, nil
}

// Get returns a paste and its decompressed body. Rows past either TTL are
// treated as missing even before the sweeper has removed them.
func (s *Store) Get(ctx context.Context, code string) (*Paste, string, error) {
	if !id.Valid(code) {
		return nil, "", ErrNotFound
	}

	now := time.Now()
	accessCutoff := cutoff(now, s.opts.TTLAccess)
	createCutoff := cutoff(now, s.opts.TTLCreate)

	var (
		p       Paste
		body    []byte
		codec   int
		created int64
	)
	err := s.r.QueryRowContext(ctx,
		`SELECT seq, code, body, codec, chars, raw_bytes, language, created_at
		   FROM pastes
		  WHERE code = ?
		    AND (? = 0 OR accessed_at > ?)
		    AND (? = 0 OR created_at > ?)`,
		code, accessCutoff, accessCutoff, createCutoff, createCutoff,
	).Scan(&p.Seq, &p.Code, &body, &codec, &p.Chars, &p.RawBytes, &p.Language, &created)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, "", ErrNotFound
	case err != nil:
		return nil, "", fmt.Errorf("store: read paste: %w", err)
	}

	p.StoredSize = len(body)
	p.CreatedAt = time.Unix(created, 0).UTC()

	// The limit is the recorded size: anything else is a corrupt row, not a
	// decode this process should spend memory on.
	content, err := s.opts.Codec.Decompress(body, codec, p.RawBytes)
	if err != nil {
		return nil, "", err
	}

	s.recordAccess(p.Seq, now)
	return &p, string(content), nil
}

// Meta returns a paste's metadata without its body.
//
// Rendering a link preview needs the language and the size, not the content, and
// skipping the body means the blob pages are never read and nothing is
// decompressed — the difference between a lookup and a full fetch.
func (s *Store) Meta(ctx context.Context, code string) (*Paste, error) {
	if !id.Valid(code) {
		return nil, ErrNotFound
	}

	now := time.Now()
	accessCutoff := cutoff(now, s.opts.TTLAccess)
	createCutoff := cutoff(now, s.opts.TTLCreate)

	var (
		p       Paste
		created int64
	)
	err := s.r.QueryRowContext(ctx,
		`SELECT seq, code, bytes, chars, raw_bytes, language, created_at
		   FROM pastes
		  WHERE code = ?
		    AND (? = 0 OR accessed_at > ?)
		    AND (? = 0 OR created_at > ?)`,
		code, accessCutoff, accessCutoff, createCutoff, createCutoff,
	).Scan(&p.Seq, &p.Code, &p.StoredSize, &p.Chars, &p.RawBytes, &p.Language, &created)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("store: read paste metadata: %w", err)
	}

	p.CreatedAt = time.Unix(created, 0).UTC()
	return &p, nil
}

// recordAccess queues a row's access time for the next flush.
func (s *Store) recordAccess(seq uint64, when time.Time) {
	s.accessMu.Lock()
	s.access[seq] = when.Unix()
	s.accessMu.Unlock()
}

// FlushAccess writes queued access times back and reports how many rows moved.
// Losing a flush costs recency accuracy for LRU, never data.
func (s *Store) FlushAccess(ctx context.Context) (int, error) {
	s.accessMu.Lock()
	pending := s.access
	s.access = make(map[uint64]int64, len(pending))
	s.accessMu.Unlock()

	if len(pending) == 0 {
		return 0, nil
	}

	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: begin access flush: %w", err)
	}
	defer tx.Rollback()

	// The guard keeps a delayed flush from dragging a row's recency backwards.
	stmt, err := tx.PrepareContext(ctx,
		`UPDATE pastes SET accessed_at = ? WHERE seq = ? AND accessed_at < ?`)
	if err != nil {
		return 0, fmt.Errorf("store: prepare access flush: %w", err)
	}
	defer stmt.Close()

	for seq, at := range pending {
		if _, err := stmt.ExecContext(ctx, at, seq, at); err != nil {
			return 0, fmt.Errorf("store: flush access time: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: commit access flush: %w", err)
	}
	return len(pending), nil
}

// Sweep applies the retention rules in priority order: the byte cap first,
// because it is the only hard guarantee, then the last-access TTL, then the
// creation TTL. A disabled rule is skipped entirely.
//
// The cap is already enforced on every write, so its pass here normally removes
// nothing; it exists to converge after the cap has been lowered.
func (s *Store) Sweep(ctx context.Context) (SweepResult, error) {
	var result SweepResult

	// Flush first so eviction ranks rows by their real recency.
	flushed, err := s.FlushAccess(ctx)
	if err != nil {
		return result, err
	}
	result.AccessFlushed = flushed

	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("store: begin sweep: %w", err)
	}
	defer tx.Rollback()

	if s.opts.MaxBytes > 0 {
		stored, err := storedBytes(ctx, tx)
		if err != nil {
			return result, err
		}
		if over := stored - s.opts.MaxBytes; over > 0 {
			_, removed, err := evict(ctx, tx, over)
			if err != nil {
				return result, err
			}
			result.SpaceEvicted = removed
		}
	}

	now := time.Now()
	if c := cutoff(now, s.opts.TTLAccess); c > 0 {
		removed, err := deleteWhere(ctx, tx, "accessed_at <= ?", c)
		if err != nil {
			return result, err
		}
		result.AccessExpired = removed
	}
	if c := cutoff(now, s.opts.TTLCreate); c > 0 {
		removed, err := deleteWhere(ctx, tx, "created_at <= ?", c)
		if err != nil {
			return result, err
		}
		result.CreateExpired = removed
	}

	if result.StoredBytes, err = storedBytes(ctx, tx); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("store: commit sweep: %w", err)
	}

	if result.Removed() > 0 {
		// Without a checkpoint a large sweep only moves the space into the WAL.
		if _, err := s.w.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			return result, fmt.Errorf("store: checkpoint after sweep: %w", err)
		}
	}
	// Cheap, bounded re-analysis; keeps the planner honest as the table grows.
	if _, err := s.w.ExecContext(ctx, `PRAGMA optimize`); err != nil {
		return result, fmt.Errorf("store: optimize after sweep: %w", err)
	}
	return result, nil
}

// Stats summarises live pastes.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	st := Stats{MaxBytes: s.opts.MaxBytes}
	err := s.r.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(raw_bytes), 0), COALESCE(SUM(bytes), 0) FROM pastes`,
	).Scan(&st.Count, &st.RawBytes, &st.StoredSize)
	if err != nil {
		return Stats{}, fmt.Errorf("store: stats: %w", err)
	}
	return st, nil
}

// cutoff converts a TTL into the unix second before which rows are stale, or 0
// when the TTL is disabled.
func cutoff(now time.Time, ttl time.Duration) int64 {
	if ttl <= 0 {
		return 0
	}
	return now.Add(-ttl).Unix()
}

func storedBytes(ctx context.Context, tx *sql.Tx) (int64, error) {
	var n int64
	if err := tx.QueryRowContext(ctx,
		`SELECT value FROM counters WHERE name = 'stored_bytes'`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: read stored bytes: %w", err)
	}
	return n, nil
}

func addStoredBytes(ctx context.Context, tx *sql.Tx, delta int64) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE counters SET value = MAX(0, value + ?) WHERE name = 'stored_bytes'`,
		delta); err != nil {
		return fmt.Errorf("store: update stored bytes: %w", err)
	}
	return nil
}

// evictLRU deletes the least-recently-used pastes whose sizes add up to at
// least need bytes.
//
// The running total is computed inside the statement so exactly the rows that
// cover need are removed and not one more. The innermost query caps how many
// rows the window ever sees, which is what keeps a full database costing a
// bounded index lookup per insert instead of a scan of everything.
const evictLRU = `
DELETE FROM pastes WHERE seq IN (
    SELECT seq FROM (
        SELECT seq, bytes, SUM(bytes) OVER (ORDER BY accessed_at, seq) AS running
          FROM (SELECT seq, bytes, accessed_at FROM pastes
                 ORDER BY accessed_at, seq
                 LIMIT ?)
    )
    WHERE running - bytes < ?
)
RETURNING bytes`

// evict frees at least need bytes, taking least-recently-used pastes first, and
// keeps the byte counter in step.
func evict(ctx context.Context, tx *sql.Tx, need int64) (freed int64, removed int64, err error) {
	for freed < need {
		batchFreed, batchRemoved, err := deleteReturning(ctx, tx, evictLRU, evictBatch, need-freed)
		if err != nil {
			return freed, removed, fmt.Errorf("store: evict: %w", err)
		}
		if batchRemoved == 0 {
			// Nothing left to give: the caller's own row is all that remains.
			break
		}
		freed += batchFreed
		removed += batchRemoved
	}

	if freed > 0 {
		if err := addStoredBytes(ctx, tx, -freed); err != nil {
			return freed, removed, err
		}
	}
	return freed, removed, nil
}

// deleteWhere removes matching rows and keeps the byte counter in step.
func deleteWhere(ctx context.Context, tx *sql.Tx, where string, args ...any) (int64, error) {
	freed, removed, err := deleteReturning(ctx,
		tx, `DELETE FROM pastes WHERE `+where+` RETURNING bytes`, args...)
	if err != nil {
		return 0, fmt.Errorf("store: delete expired rows: %w", err)
	}
	if removed == 0 {
		return 0, nil
	}
	if err := addStoredBytes(ctx, tx, -freed); err != nil {
		return 0, err
	}
	return removed, nil
}

// deleteReturning runs a DELETE that returns each removed row's size, summing
// them as they arrive. One statement measures and deletes together, so neither
// path has to scan the table twice or hold a list of keys in memory.
func deleteReturning(ctx context.Context, tx *sql.Tx, query string, args ...any) (freed, removed int64, err error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var size int64
		if err := rows.Scan(&size); err != nil {
			return 0, 0, err
		}
		freed += size
		removed++
	}
	return freed, removed, rows.Err()
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
