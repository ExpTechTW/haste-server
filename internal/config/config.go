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

	"github.com/joho/godotenv"
)

// zstd's own accepted range. 19 is the "extreme" preset this server defaults to:
// pastes are capped at a few kilobytes, so even the slowest levels cost
// microseconds, and every byte saved is a byte never re-read from disk.
const (
	MinZstdLevel     = 1
	MaxZstdLevel     = 22
	DefaultZstdLevel = 19
)

// Config is the fully resolved, validated server configuration.
type Config struct {
	Addr    string
	BaseURL string // absolute origin used to build share links; empty = derive per request

	DBPath        string
	ReadPool      int // read-only connections; the writer is always exactly 1
	SQLiteCacheMB int // page cache per connection

	MaxChars        int
	ZstdLevel       int
	Retention       time.Duration // 0 = keep forever
	CleanupInterval time.Duration

	IDSecret []byte

	RateRPS    float64 // sustained paste creations per client IP; 0 = unlimited
	RateBurst  int
	TrustProxy bool // honour X-Forwarded-For / X-Real-IP when rate limiting

	CORSOrigins []string // "*" allows any origin
}

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
		MaxChars:      envInt("HASTE_MAX_CHARS", 4000),
		ZstdLevel:     envInt("HASTE_ZSTD_LEVEL", DefaultZstdLevel),
		RateRPS:       envFloat("HASTE_RATE_RPS", 1),
		RateBurst:     envInt("HASTE_RATE_BURST", 20),
		TrustProxy:    envBool("HASTE_TRUST_PROXY", false),
		CORSOrigins:   envList("HASTE_CORS_ORIGINS", "*"),
	}

	var err error
	if cfg.Retention, err = envDur("HASTE_RETENTION", "30d"); err != nil {
		return nil, err
	}
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
	case c.Retention < 0:
		return fmt.Errorf("config: HASTE_RETENTION must not be negative")
	case c.CleanupInterval <= 0:
		return fmt.Errorf("config: HASTE_CLEANUP_INTERVAL must be > 0")
	case c.RateRPS < 0:
		return fmt.Errorf("config: HASTE_RATE_RPS must not be negative")
	}
	return nil
}

// MaxBodyBytes is the hard cap applied to request bodies. A rune is at most 4
// bytes in UTF-8, plus headroom for the surrounding JSON envelope.
func (c *Config) MaxBodyBytes() int64 {
	return int64(c.MaxChars)*4 + 4096
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
