// Command haste runs the paste server: JSON API, raw endpoints, and the
// embedded single-page frontend, all from one binary.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/YuYu1015/haste-server/internal/compress"
	"github.com/YuYu1015/haste-server/internal/config"
	"github.com/YuYu1015/haste-server/internal/httpapi"
	"github.com/YuYu1015/haste-server/internal/id"
	"github.com/YuYu1015/haste-server/internal/store"
	"github.com/YuYu1015/haste-server/internal/webui"
)

// shutdownGrace bounds how long in-flight requests get once a signal arrives.
const shutdownGrace = 15 * time.Second

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	codec, err := compress.New(cfg.ZstdLevel)
	if err != nil {
		return err
	}
	defer codec.Close()

	st, err := store.Open(ctx, store.Options{
		Path:      cfg.DBPath,
		CacheMB:   cfg.SQLiteCacheMB,
		ReadPool:  cfg.ReadPool,
		MaxChars:  cfg.MaxChars,
		MaxBytes:  cfg.MaxBytes,
		TTLAccess: cfg.TTLAccess,
		TTLCreate: cfg.TTLCreate,

		WriteConcurrency: cfg.WriteConcurrency,
		WriteQueue:       cfg.WriteQueue,

		Codec: codec,
		IDs:   id.NewGenerator(cfg.IDSecret, cfg.CodeMinLen, httpapi.ReservedCodes),
	})
	if err != nil {
		return err
	}
	defer st.Close()

	ui, err := webui.FS()
	if err != nil {
		return fmt.Errorf("mount frontend: %w", err)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(cfg, st, ui, log).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Sweep once at startup so a server that was down past a retention window
	// does not serve stale pastes until the first tick.
	go sweeper(ctx, st, cfg.CleanupInterval, log)

	errc := make(chan error, 1)
	go func() {
		log.Info("listening",
			"addr", cfg.Addr,
			"db", cfg.DBPath,
			"maxChars", cfg.MaxChars,
			"codeMinLen", cfg.CodeMinLen,
			"zstd", cfg.ZstdLevel,
			"cacheMB", cfg.SQLiteCacheMB,
			"readPool", cfg.ReadPool,
			"maxBytes", byteLabel(cfg.MaxBytes),
			"ttlAccess", ttlLabel(cfg.TTLAccess),
			"ttlCreate", ttlLabel(cfg.TTLCreate),
			"cleanupEvery", cfg.CleanupInterval.String(),
			"writeConcurrency", cfg.WriteConcurrency,
			"writeQueue", cfg.WriteQueue,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

// accessFlushInterval bounds how stale the LRU ordering can get. Access times
// are batched rather than written per read, so this is the window in which a
// crash could lose recency information — never data.
const accessFlushInterval = time.Minute

// sweeper applies the retention rules on a fixed interval until the context
// ends, flushing access times far more often so eviction ranks rows by their
// real recency rather than by whenever the last sweep happened to run.
func sweeper(ctx context.Context, st *store.Store, every time.Duration, log *slog.Logger) {
	sweep := func() {
		result, err := st.Sweep(ctx)
		if err != nil {
			if ctx.Err() == nil {
				log.Error("cleanup failed", "err", err)
			}
			return
		}
		if result.Removed() > 0 {
			log.Info("cleanup",
				"expiredByLifetime", result.Expired,
				"evictedForSpace", result.SpaceEvicted,
				"expiredByAccess", result.AccessExpired,
				"expiredByAge", result.CreateExpired,
				"storedBytes", result.StoredBytes,
			)
		}
	}

	sweep()

	sweepTick := time.NewTicker(every)
	defer sweepTick.Stop()

	flushEvery := accessFlushInterval
	if every < flushEvery {
		flushEvery = every
	}
	flushTick := time.NewTicker(flushEvery)
	defer flushTick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sweepTick.C:
			sweep()
		case <-flushTick.C:
			if _, err := st.FlushAccess(ctx); err != nil && ctx.Err() == nil {
				log.Error("access flush failed", "err", err)
			}
		}
	}
}

func ttlLabel(d time.Duration) string {
	if d <= 0 {
		return "off"
	}
	return d.String()
}

func byteLabel(n int64) string {
	if n <= 0 {
		return "unlimited"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGT"[exp])
}
