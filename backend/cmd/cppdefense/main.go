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

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	"github.com/JO-IK1/CppDefense/backend/internal/config"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
	"github.com/JO-IK1/CppDefense/backend/internal/transport/httpapi"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return usageError()
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	logger := newLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Open(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer db.Close()

	switch args[0] {
	case "migrate":
		if err := postgres.Migrate(ctx, db); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		logger.Info("database migrations applied")
		return nil
	case "healthcheck":
		checkCtx, cancel := context.WithTimeout(ctx, cfg.Database.HealthTimeout)
		defer cancel()
		if err := postgres.Ready(checkCtx, db); err != nil {
			return fmt.Errorf("not ready: %w", err)
		}
		fmt.Println("ready")
		return nil
	case "api":
		return serve(ctx, cfg, logger, db)
	default:
		return usageError()
	}
}

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger, db *postgres.Database) error {
	if cfg.AutoMigrate {
		if err := postgres.Migrate(ctx, db); err != nil {
			return fmt.Errorf("automatic migration: %w", err)
		}
	}

	auditService := audit.New(postgres.NewAuditRepository(db))
	handler := httpapi.New(httpapi.Dependencies{
		Logger: logger,
		Ready: func(ctx context.Context) error {
			return postgres.Ready(ctx, db)
		},
		Audit:  auditService,
		Config: cfg.HTTP,
	})

	server := &http.Server{
		Addr:              cfg.HTTP.Address,
		Handler:           handler,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		MaxHeaderBytes:    cfg.HTTP.MaxHeaderBytes,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api listening", "address", cfg.HTTP.Address)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		logger.Info("api shutting down")
		return server.Shutdown(shutdownCtx)
	}
}

func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

func usageError() error {
	return errors.New("usage: cppdefense <api|migrate|healthcheck>")
}
