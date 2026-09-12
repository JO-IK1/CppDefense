package postgres

import "testing"

func TestMigrationsAreEmbedded(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 {
		t.Fatalf("embedded migration count = %d", len(migrations))
	}
	if migrations[0].name != "000001_initial.sql" || migrations[1].name != "000002_storage_immutability.sql" {
		t.Fatalf("unexpected embedded migrations: %q, %q", migrations[0].name, migrations[1].name)
	}
}
