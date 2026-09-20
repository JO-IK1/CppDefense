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

	"github.com/JO-IK1/CppDefense/backend/internal/application/audit"
	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	appstorage "github.com/JO-IK1/CppDefense/backend/internal/application/storage"
	"github.com/JO-IK1/CppDefense/backend/internal/config"
	githuboauth "github.com/JO-IK1/CppDefense/backend/internal/infrastructure/github"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/objectstore/local"
	s3storage "github.com/JO-IK1/CppDefense/backend/internal/infrastructure/objectstore/s3"
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
		fileStorage, err := openStorage(ctx, cfg.Storage)
		if err != nil {
			return fmt.Errorf("storage: %w", err)
		}
		checkCtx, cancel := context.WithTimeout(ctx, cfg.Database.HealthTimeout)
		defer cancel()
		if err := postgres.Ready(checkCtx, db); err != nil {
			return fmt.Errorf("not ready: %w", err)
		}
		if err := fileStorage.Check(checkCtx); err != nil {
			return fmt.Errorf("storage not ready: %w", err)
		}
		fmt.Println("ready")
		return nil
	case "reconcile-storage":
		fileStorage, err := openStorage(ctx, cfg.Storage)
		if err != nil {
			return fmt.Errorf("storage: %w", err)
		}
		report, err := appstorage.Reconcile(ctx, fileStorage, postgres.NewStorageRepository(db))
		if err != nil {
			return fmt.Errorf("reconcile storage: %w", err)
		}
		fmt.Printf("checked=%d missing=%d orphaned=%d\n", report.Checked, len(report.Missing), len(report.Orphaned))
		if len(report.Missing) != 0 {
			return errors.New("storage reconciliation found missing objects")
		}
		return nil
	case "api":
		fileStorage, err := openStorage(ctx, cfg.Storage)
		if err != nil {
			return fmt.Errorf("storage: %w", err)
		}
		return serve(ctx, cfg, logger, db, fileStorage)
	default:
		return usageError()
	}
}

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger, db *postgres.Database, fileStorage appstorage.FileStorage) error {
	if cfg.AutoMigrate {
		if err := postgres.Migrate(ctx, db); err != nil {
			return fmt.Errorf("automatic migration: %w", err)
		}
	}

	auditService := audit.New(postgres.NewAuditRepository(db))
	authService, err := appauth.New(appauth.Config{
		ClientID: cfg.GitHub.ClientID, ClientSecret: cfg.GitHub.ClientSecret,
		RedirectURL:  cfg.HTTP.PublicOrigin + "/api/v1/auth/github/callback",
		AuthorizeURL: cfg.GitHub.AuthorizeURL, FlowKey: cfg.GitHub.FlowKey, FlowTTL: cfg.GitHub.FlowTTL,
	}, postgres.NewAuthRepository(db, cfg.GitHub.BootstrapAdminID), githuboauth.New(&http.Client{Timeout: 10 * time.Second}, cfg.GitHub.TokenURL, cfg.GitHub.APIURL))
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	sessions := postgres.NewSessionRepository(db, cfg.Session.HashKey, cfg.Session.CSRFKey)
	storageRepository := postgres.NewStorageRepository(db)
	handler := httpapi.New(httpapi.Dependencies{
		Logger: logger,
		Ready: func(ctx context.Context) error {
			return errors.Join(postgres.Ready(ctx, db), fileStorage.Check(ctx))
		},
		Audit:         auditService,
		Auth:          authService,
		Sessions:      sessions,
		SessionConfig: cfg.Session,
		Config:        cfg.HTTP,
		Catalog:       postgres.NewCatalogRepository(db),
		Imports:       postgres.NewImportRepository(db),
		Files:         fileStorage,
		Submissions:   appstorage.NewSubmissionService(fileStorage, storageRepository),
		Downloads:     appstorage.NewDownloadService(fileStorage, storageRepository),
		Defenses:      postgres.NewDefenseRepository(db),
		RunnerToken:   cfg.RunnerToken,
		Admin:         postgres.NewAdminRepository(db),
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
	return errors.New("usage: cppdefense <api|migrate|healthcheck|reconcile-storage>")
}

func openStorage(ctx context.Context, cfg config.Storage) (appstorage.FileStorage, error) {
	if cfg.Mode == "local" {
		return local.New(cfg.LocalRoot)
	}
	storage, err := s3storage.New(s3storage.Config{
		Endpoint: cfg.Endpoint, Region: cfg.Region, Bucket: cfg.Bucket,
		AccessKey: cfg.AccessKey, SecretKey: cfg.SecretKey, Secure: cfg.Secure, SpoolDir: cfg.SpoolDir,
	})
	if err != nil {
		return nil, err
	}
	if err := storage.EnsurePrivateBucket(ctx); err != nil {
		return nil, err
	}
	return storage, nil
}
