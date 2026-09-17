package postgres

import "testing"

func TestMigrationsAreEmbedded(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 3 {
		t.Fatalf("embedded migration count = %d", len(migrations))
	}
	if migrations[0].name != "000001_initial.sql" || migrations[1].name != "000002_storage_immutability.sql" || migrations[2].name != "000003_github_only_auth.sql" {
		t.Fatalf("unexpected embedded migrations: %q, %q, %q", migrations[0].name, migrations[1].name, migrations[2].name)
	}
}
