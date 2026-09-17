package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/config"
	"github.com/JO-IK1/CppDefense/backend/internal/domain"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
	"github.com/jackc/pgx/v5/pgconn"
)

func database(t *testing.T) *postgres.Database {
	t.Helper()
	url := os.Getenv("CPPDEFENSE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CPPDEFENSE_TEST_DATABASE_URL is not set")
	}
	db, err := postgres.Open(context.Background(), config.Database{
		URL: url, MaxConnections: 4, MinConnections: 1,
		MaxConnectionLife: time.Minute, MaxConnectionIdle: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := postgres.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMigrationsAreRepeatable(t *testing.T) {
	db := database(t)
	if err := postgres.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Pool().QueryRow(context.Background(), "select count(*) from cppdefense_schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("migration count = %d", count)
	}
	var legacyTable *string
	if err := db.Pool().QueryRow(context.Background(), "select to_regclass('public.legacy_telegram_identities')::text").Scan(&legacyTable); err != nil {
		t.Fatal(err)
	}
	if legacyTable == nil {
		t.Fatal("legacy Telegram identity data was not retained")
	}
}

func TestIdentityUniquenessAndStateTransition(t *testing.T) {
	db := database(t)
	ctx := context.Background()
	userA, _ := domain.NewUUIDv7()
	userB, _ := domain.NewUUIDv7()
	identityA, _ := domain.NewUUIDv7()
	identityB, _ := domain.NewUUIDv7()
	loginSuffix, _ := domain.NewUUIDv7()
	githubUserID := time.Now().UnixNano()
	if _, err := db.Pool().Exec(ctx, "insert into users(id) values ($1),($2)", userA, userB); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, `insert into github_identities(id,user_id,github_user_id,login,verified_at) values($1,$2,$3,$4,clock_timestamp())`, identityA, userA, githubUserID, "stage2-"+loginSuffix); err != nil {
		t.Fatal(err)
	}
	_, err := db.Pool().Exec(ctx, `insert into github_identities(id,user_id,github_user_id,login,verified_at) values($1,$2,$3,$4,clock_timestamp())`, identityB, userB, githubUserID, "other-"+loginSuffix)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("expected unique violation, got %v", err)
	}

	groupID, _ := domain.NewUUIDv7()
	importID, _ := domain.NewUUIDv7()
	if _, err := db.Pool().Exec(ctx, "insert into groups(id,code,name) values($1,$2,$3)", groupID, "g-"+loginSuffix, "Stage 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, `insert into imports(id,group_id,uploaded_by,kind,original_object_key,original_sha256,compressed_size,schema_version) values($1,$2,$3,'group',$4,$5,0,1)`, importID, groupID, userA, "test/"+importID, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	_, err = db.Pool().Exec(ctx, "update imports set state='completed' where id=$1", importID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23000" {
		t.Fatalf("expected transition violation, got %v", err)
	}
}
