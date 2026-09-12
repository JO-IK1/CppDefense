package postgres

import (
	"context"
	"fmt"

	"github.com/JO-IK1/CppDefense/backend/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Database struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, cfg config.Database) (*Database, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse connection URL: %w", err)
	}
	poolConfig.MaxConns = cfg.MaxConnections
	poolConfig.MinConns = cfg.MinConnections
	poolConfig.MaxConnLifetime = cfg.MaxConnectionLife
	poolConfig.MaxConnIdleTime = cfg.MaxConnectionIdle
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "cppdefense-backend"
	poolConfig.ConnConfig.RuntimeParams["timezone"] = "UTC"

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	return &Database{pool: pool}, nil
}

func (db *Database) Close() { db.pool.Close() }

func (db *Database) Pool() *pgxpool.Pool { return db.pool }

func Ready(ctx context.Context, db *Database) error {
	if err := db.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		var checksum string
		var success bool
		err := db.pool.QueryRow(ctx,
			"select checksum, success from cppdefense_schema_migrations where version = $1",
			migration.version,
		).Scan(&checksum, &success)
		if err == pgx.ErrNoRows {
			return fmt.Errorf("migration %s is not applied", migration.name)
		}
		if err != nil {
			return fmt.Errorf("migration state: %w", err)
		}
		if !success || checksum != migration.checksum {
			return fmt.Errorf("migration %s has invalid state", migration.name)
		}
	}
	return nil
}
