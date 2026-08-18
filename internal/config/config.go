// Package config loads runtime configuration from the process environment,
// seeded by an optional .env file. Real environment variables always win over
// .env so container deployments can override anything without editing files.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/YuYu1015/haste-server/internal/compress"
	"github.com/YuYu1015/haste-server/internal/id"

	"github.com/joho/godotenv"
)

// zstd's own accepted range. The default lives in the compress package, next to
// the measurements that chose it.
const (
	MinZstdLevel = 1
	MaxZstdLevel = 22
)

// MinMaxBytes is an absolute floor for the space cap. The real floor is
// derived from HASTE_MAX_CHARS in validate(), because a cap that cannot hold a
// single maximum-size paste would evict every write the moment it landed.
const MinMaxBytes = 1 << 20

// MinStatsTokenLen keeps a token from being guessable. The endpoint is not rate
// limited for an authorized caller, so a four-character token would be a
// formality rather than a control.
const MinStatsTokenLen = 16

// DefaultMaxChars is a tenth of the classic haste-server's `maxLength: 400000`,
// and the difference is deliberate.
//
// 40k characters is roughly 300 lines of structured log, 1,500 lines of source,
// or 40,000 Chinese characters — about thirty A4 pages. It is also, measured on
// this frontend, the last size at which the editor can still colour the text on
// every keystroke inside a 16ms frame: 40k costs about 14ms, 60k about 21ms.
// Past that the editor has to stop highlighting to stay responsive, which is a
// worse deal than the extra room is worth.
const DefaultMaxChars = 40_000

// Config is the fully resolved, validated server configuration.
type Config struct {
	Addr    string
	BaseURL string // absolute origin used to build share links; empty = derive per request

	DBPath        string
	ReadPool      int // read-only connections; the writer is always exactly 1
	SQLiteCacheMB int // page cache per connection

	MaxChars  int
	ZstdLevel int

	// Retention is a budget, not a promise. MaxBytes is the hard guarantee and
	// is enforced on every write; the two TTLs are optional extra trimming and
	// are both disabled by default.
	MaxBytes        int64         // 0 = unlimited
	TTLAccess       time.Duration // 0 = never expire on idleness
	TTLCreate       time.Duration // 0 = never expire on age
	CleanupInterval time.Duration

	// Admission control for writes. Compressing at zstd-19 costs roughly a
	// millisecond of CPU per paste and runs before the transaction, so it — not
	// SQLite — is what unbounded load actually saturates.
	WriteConcurrency int
	WriteQueue       int

	CodeMinLen int // shortest share code issued
	IDSecret   []byte

	RateRPS    float64 // sustained paste creations per client IP; 0 = unlimited
	RateBurst  int
	TrustProxy bool // honour X-Forwarded-For / X-Real-IP when rate limiting

	CORSOrigins []string // "*" allows any origin

	// Stats is an operational endpoint, not a public one, and it is off unless
	// an operator says otherwise. See StatsMode.
	Stats      StatsMode
	StatsToken string
}

// StatsMode decides who, if anyone, may read /api/stats.
//
// Off by default because the numbers are far more useful to someone attacking
// the instance than to anyone using it. The corpus totals move by exactly one
// paste at a time, so polling them reveals the size of every paste as it
// arrives; usedFraction turns filling the storage cap into a task with a
// progress bar; and a falling count confirms that other people's pastes are
// being evicted. None of that is information a stranger needs.
type StatsMode string

const (
	// StatsOff answers 404, as if the route did not exist.
	StatsOff StatsMode = "off"
	// StatsToken requires a bearer token, for a monitoring system.
	StatsToken StatsMode = "token"
	// StatsPublic serves it to anyone, which is a deliberate choice.
	StatsPublic StatsMode = "public"
)

// Load reads configuration, applying defaults for everything that is unset.
func Load() (*Config, error) {
	// Optional: a missing .env is the normal case in production.
	_ = godotenv.Load()

	cfg := &Config{
		Addr:          envStr("HASTE_ADDR", ":8080"),
		BaseURL:       strings.TrimRight(envStr("HASTE_BASE_URL", ""), "/"),
		DBPath:        envStr("HASTE_DB_PATH", "data/haste.db"),
		ReadPool:      envInt("HASTE_READ_POOL", min(runtime.NumCPU(), 8)),
		SQLiteCacheMB: envInt("HASTE_SQLITE_CACHE_MB", 48),
		MaxChars:      envInt("HASTE_MAX_CHARS", DefaultMaxChars),
		ZstdLevel:     envInt("HASTE_ZSTD_LEVEL", compress.DefaultLevel),
		CodeMinLen:    envInt("HASTE_CODE_MIN_LEN", id.DefaultMinLen),
		// Compression is CPU-bound, so more simultaneous writes than cores buys
		// context switching rather than throughput.
		WriteConcurrency: envInt("HASTE_WRITE_CONCURRENCY", runtime.NumCPU()),
		WriteQueue:       envInt("HASTE_WRITE_QUEUE", 512),
		RateRPS:          envFloat("HASTE_RATE_RPS", 1),
		RateBurst:        envInt("HASTE_RATE_BURST", 20),
		TrustProxy:       envBool("HASTE_TRUST_PROXY", false),
		CORSOrigins:      envList("HASTE_CORS_ORIGINS", "*"),
	}

	var err error
	if cfg.MaxBytes, err = envBytes("HASTE_MAX_BYTES", "1GiB"); err != nil {
		return nil, err
	}
	// Both TTLs default to off: the space cap alone decides what gets removed
	// unless an operator asks for an age policy as well.
	if cfg.TTLAccess, err = envDur("HASTE_TTL_ACCESS", "0"); err != nil {
		return nil, err
	}
	if cfg.TTLCreate, err = envDur("HASTE_TTL_CREATE", "0"); err != nil {
		return nil, err
	}
	cfg.Stats = StatsMode(strings.ToLower(strings.TrimSpace(envStr("HASTE_STATS", string(StatsOff)))))
	cfg.StatsToken = envStr("HASTE_STATS_TOKEN", "")

	if cfg.CleanupInterval, err = envDur("HASTE_CLEANUP_INTERVAL", "1h"); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.DBPath, err = filepath.Abs(cfg.DBPath); err != nil {
		return nil, fmt.Errorf("config: resolve HASTE_DB_PATH: %w", err)
	}
	if cfg.IDSecret, err = loadIDSecret(cfg.DBPath); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	switch {
	case c.MaxChars < 1:
		return fmt.Errorf("config: HASTE_MAX_CHARS must be >= 1, got %d", c.MaxChars)
	case c.ZstdLevel < MinZstdLevel || c.ZstdLevel > MaxZstdLevel:
		return fmt.Errorf("config: HASTE_ZSTD_LEVEL must be %d-%d, got %d", MinZstdLevel, MaxZstdLevel, c.ZstdLevel)
	case c.SQLiteCacheMB < 1:
		return fmt.Errorf("config: HASTE_SQLITE_CACHE_MB must be >= 1, got %d", c.SQLiteCacheMB)
	case c.ReadPool < 1:
		return fmt.Errorf("config: HASTE_READ_POOL must be >= 1, got %d", c.ReadPool)
	case c.CodeMinLen < 1 || c.CodeMinLen > id.MaxLen:
		return fmt.Errorf("config: HASTE_CODE_MIN_LEN must be 1-%d, got %d", id.MaxLen, c.CodeMinLen)
	case c.MaxBytes < 0:
		return fmt.Errorf("config: HASTE_MAX_BYTES must not be negative")
	// A cap below one worst-case paste would evict every write immediately, so
	// the floor tracks the character limit rather than sitting at a constant.
	case c.MaxBytes > 0 && c.MaxBytes < c.minStorageBytes():
		return fmt.Errorf("config: HASTE_MAX_BYTES must be 0 or at least %d bytes for a %d character limit, got %d",
			c.minStorageBytes(), c.MaxChars, c.MaxBytes)
	case c.TTLAccess < 0:
		return fmt.Errorf("config: HASTE_TTL_ACCESS must not be negative")
	case c.TTLCreate < 0:
		return fmt.Errorf("config: HASTE_TTL_CREATE must not be negative")
	case c.Stats != StatsOff && c.Stats != StatsToken && c.Stats != StatsPublic:
		return fmt.Errorf("config: HASTE_STATS must be off, token or public, got %q", c.Stats)
	case c.Stats == StatsToken && len(c.StatsToken) < MinStatsTokenLen:
		return fmt.Errorf("config: HASTE_STATS=token needs a HASTE_STATS_TOKEN of at least %d characters", MinStatsTokenLen)
	case c.CleanupInterval <= 0:
		return fmt.Errorf("config: HASTE_CLEANUP_INTERVAL must be > 0")
	case c.WriteConcurrency < 0:
		return fmt.Errorf("config: HASTE_WRITE_CONCURRENCY must not be negative")
	case c.WriteQueue < 0:
		return fmt.Errorf("config: HASTE_WRITE_QUEUE must not be negative")
	case c.RateRPS < 0:
		return fmt.Errorf("config: HASTE_RATE_RPS must not be negative")
	}
	return nil
}

// minStorageBytes is the smallest cap that can hold one paste of the maximum
// size. A rune is at most 4 bytes in UTF-8 and compression never meaningfully
// expands, so that product bounds a single row.
func (c *Config) minStorageBytes() int64 {
	floor := int64(c.MaxChars) * 4
	if floor < MinMaxBytes {
		return MinMaxBytes
	}
	return floor
}

// bytesPerChar bounds how many request bytes one accepted character may cost.
//
// Raw UTF-8 needs at most 4, but JSON lets a client escape any character as
// \uXXXX, and escaping is not exotic: Python's json.dumps does it by default.
// One escape is 6 bytes, and a character outside the BMP is written as a
// surrogate pair — 12 bytes for what MaxChars counts as a single code point.
//
// So the body cap has to allow 12, or a full-size CJK paste from a perfectly
// ordinary API client is rejected as too large before anything ever counts its
// characters. This is only a guard against unbounded reads; the real limit is
// still MaxChars, checked after decoding.
const bytesPerChar = 12

// MaxBodyBytes is the hard cap applied to request bodies: the worst-case
// encoding of MaxChars characters, plus headroom for the JSON envelope.
func (c *Config) MaxBodyBytes() int64 {
	return int64(c.MaxChars)*bytesPerChar + 4096
}

// loadIDSecret keys the share-code permutation. It is persisted next to the
// database so restarts keep handing out codes in the same shuffled order;
// losing it cannot break existing pastes, because codes are stored, not derived.
func loadIDSecret(dbPath string) ([]byte, error) {
	if v := os.Getenv("HASTE_ID_SECRET"); v != "" {
		return []byte(v), nil
	}
	dir := filepath.Dir(dbPath)
	path := filepath.Join(dir, ".id_secret")
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return b, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("config: generate id secret: %w", err)
	}
	secret := []byte(hex.EncodeToString(buf))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("config: create data dir: %w", err)
	}
	if err := os.WriteFile(path, secret, 0o600); err != nil {
		return nil, fmt.Errorf("config: persist id secret: %w", err)
	}
	return secret, nil
}

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(envStr(key, "")); err == nil {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(envStr(key, ""), 64); err == nil {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, err := strconv.ParseBool(envStr(key, "")); err == nil {
		return v
	}
	return def
}

func envList(key, def string) []string {
	var out []string
	for _, part := range strings.Split(envStr(key, def), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func envDur(key, def string) (time.Duration, error) {
	raw := envStr(key, def)
	d, err := ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s: %w", key, err)
	}
	return d, nil
}

func envBytes(key, def string) (int64, error) {
	raw := envStr(key, def)
	n, err := ParseBytes(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s: %w", key, err)
	}
	return n, nil
}

// byteUnits are ordered longest-suffix-first so "GiB" is matched before "B".
var byteUnits = []struct {
	suffix string
	scale  int64
}{
	{"KIB", 1 << 10}, {"MIB", 1 << 20}, {"GIB", 1 << 30}, {"TIB", 1 << 40},
	{"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12},
	{"K", 1 << 10}, {"M", 1 << 20}, {"G", 1 << 30}, {"T", 1 << 40},
	{"B", 1},
}

// ParseBytes reads a storage size. Both conventions are accepted and mean what
// they say: "1GB" is 1e9 bytes, "1GiB" is 2^30. A bare number is bytes.
func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}

	upper := strings.ToUpper(s)
	for _, unit := range byteUnits {
		if !strings.HasSuffix(upper, unit.suffix) {
			continue
		}
		digits := strings.TrimSpace(upper[:len(upper)-len(unit.suffix)])
		if digits == "" {
			continue
		}
		n, err := strconv.ParseFloat(digits, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid size %q", s)
		}
		if n < 0 {
			return 0, fmt.Errorf("invalid size %q: must not be negative", s)
		}
		return int64(n * float64(unit.scale)), nil
	}

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return n, nil
}

// ParseDuration extends time.ParseDuration with the day and week suffixes that
// retention policies are naturally written in ("30d", "2w").
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	unit := time.Duration(0)
	switch s[len(s)-1] {
	case 'd', 'D':
		unit = 24 * time.Hour
	case 'w', 'W':
		unit = 7 * 24 * time.Hour
	}
	if unit == 0 {
		return time.ParseDuration(s)
	}
	n, err := strconv.ParseFloat(s[:len(s)-1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return time.Duration(n * float64(unit)), nil
}
