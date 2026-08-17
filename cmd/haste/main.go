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
		Retention: cfg.Retention,
		Codec:     codec,
		IDs:       id.NewGenerator(cfg.IDSecret, cfg.CodeMinLen, httpapi.ReservedCodes),
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
	// does not serve expired pastes until the first tick.
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
			"retention", retentionLabel(cfg.Retention),
			"cleanupEvery", cfg.CleanupInterval.String(),
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

// sweeper deletes expired pastes on a fixed interval until the context ends.
func sweeper(ctx context.Context, st *store.Store, every time.Duration, log *slog.Logger) {
	purge := func() {
		n, err := st.PurgeExpired(ctx)
		if err != nil {
			if ctx.Err() == nil {
				log.Error("cleanup failed", "err", err)
			}
			return
		}
		if n > 0 {
			log.Info("cleanup", "deleted", n)
		}
	}

	purge()

	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			purge()
		}
	}
}

func retentionLabel(d time.Duration) string {
	if d <= 0 {
		return "forever"
	}
	return d.String()
}
