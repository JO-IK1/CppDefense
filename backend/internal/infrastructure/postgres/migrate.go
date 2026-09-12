package postgres

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

var migrationFiles embed.FS

const migrationLockID int64 = 43677068020200

func Migrate(ctx context.Context, db *Database) error {
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "select pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer conn.Exec(context.Background(), "select pg_advisory_unlock($1)", migrationLockID)

	if _, err := conn.Exec(ctx, `
		create table if not exists cppdefense_schema_migrations (
			version bigint primary key,
			name text not null unique,
			checksum text not null,
			applied_at timestamptz not null default clock_timestamp(),
			success boolean not null
		)
	`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		if err := applyMigration(ctx, conn.Conn(), migration); err != nil {
			return err
		}
	}
	return nil
}

type migration struct {
	version  int64
	name     string
	contents string
	checksum string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("list embedded migrations: %w", err)
	}
	result := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration name %q", entry.Name())
		}
		version, parseErr := strconv.ParseInt(parts[0], 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid migration version %q: %w", entry.Name(), parseErr)
		}
		contents, readErr := migrationFiles.ReadFile("migrations/" + entry.Name())
		if readErr != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), readErr)
		}
		digest := sha256.Sum256(contents)
		result = append(result, migration{version, entry.Name(), string(contents), hex.EncodeToString(digest[:])})
	}
	slices.SortFunc(result, func(a, b migration) int {
		if a.version < b.version {
			return -1
		}
		if a.version > b.version {
			return 1
		}
		return 0
	})
	return result, nil
}

func applyMigration(ctx context.Context, conn *pgx.Conn, migration migration) error {
	var checksum string
	var success bool
	err := conn.QueryRow(ctx,
		"select checksum, success from cppdefense_schema_migrations where version = $1",
		migration.version,
	).Scan(&checksum, &success)
	if err == nil {
		if checksum != migration.checksum {
			return fmt.Errorf("migration %s checksum changed", migration.name)
		}
		if !success {
			return fmt.Errorf("migration %s is marked incomplete", migration.name)
		}
		return nil
	}
	if err != pgx.ErrNoRows {
		return fmt.Errorf("read migration %s: %w", migration.name, err)
	}

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", migration.name, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, migration.contents); err != nil {
		return fmt.Errorf("execute migration %s: %w", migration.name, err)
	}
	if _, err := tx.Exec(ctx, `
		insert into cppdefense_schema_migrations(version, name, checksum, success)
		values ($1, $2, $3, true)
	`, migration.version, migration.name, migration.checksum); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", migration.name, err)
	}
	return nil
}
