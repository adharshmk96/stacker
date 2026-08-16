package server

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"stacker/internal/config"
	"stacker/internal/database"
	"stacker/internal/logger"
)

const shutdownTimeout = 10 * time.Second

// Run starts the HTTP server and blocks until it is interrupted.
func Run(cfg config.Config) error {
	log := logger.New(cfg.LogLevel, cfg.Env)

	db, err := database.Open(cfg.DBPath, log)
	if err != nil {
		return err
	}
	defer database.Close(db) //nolint:errcheck // nothing useful to do on exit

	if err := database.Migrate(db, log); err != nil {
		return err
	}

	handler, err := newRouter(cfg, db, log)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", cfg.Addr, "dataDir", cfg.DataDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
