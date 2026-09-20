package postgres

import "testing"

func TestMigrationsAreEmbedded(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 8 {
		t.Fatalf("embedded migration count = %d", len(migrations))
	}
	if migrations[0].name != "000001_initial.sql" || migrations[1].name != "000002_storage_immutability.sql" || migrations[2].name != "000003_github_only_auth.sql" || migrations[3].name != "000004_candidate_signature.sql" || migrations[4].name != "000005_candidate_display_signature.sql" || migrations[5].name != "000006_runner_expiry_retry.sql" || migrations[6].name != "000007_defense_idempotency.sql" || migrations[7].name != "000008_masked_source.sql" {
		t.Fatalf("unexpected embedded migrations: %q, %q, %q, %q", migrations[0].name, migrations[1].name, migrations[2].name, migrations[3].name)
	}
}
