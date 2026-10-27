package integration

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/config"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
	"github.com/jackc/pgx/v5"
)

func isolatedDatabase(t *testing.T) *postgres.Database {
	t.Helper()
	parent := database(t)
	ctx := context.Background()
	id, _ := domain.NewUUIDv7()
	schema := "test_" + strings.ReplaceAll(id, "-", "")
	if _, err := parent.Pool().Exec(ctx, "create schema "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { parent.Pool().Exec(ctx, "drop schema "+pgx.Identifier{schema}.Sanitize()+" cascade") })
	parsed, err := url.Parse(os.Getenv("CPPDEFENSE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema+",public")
	parsed.RawQuery = query.Encode()
	db, err := postgres.Open(ctx, config.Database{URL: parsed.String(), MaxConnections: 2, MinConnections: 1, MaxConnectionLife: time.Minute, MaxConnectionIdle: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = postgres.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRunnerLongPollWakesOnCommittedJob(t *testing.T) {
	testHideSeekLifecycleWithWait(t, false, true)
}
