package config

import (
	"encoding/base64"
	"testing"
)

func TestLoad(t *testing.T) {
	secret := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	t.Setenv("CPPDEFENSE_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("CPPDEFENSE_SESSION_HASH_KEY", secret)
	t.Setenv("CPPDEFENSE_CSRF_HASH_KEY", secret)
	t.Setenv("CPPDEFENSE_OAUTH_FLOW_KEY", secret)
	t.Setenv("CPPDEFENSE_GITHUB_CLIENT_ID", "test-client")
	t.Setenv("CPPDEFENSE_GITHUB_CLIENT_SECRET", "test-secret")
	t.Setenv("CPPDEFENSE_BOOTSTRAP_ADMIN_GITHUB_ID", "42")
	t.Setenv("CPPDEFENSE_RUNNER_TOKEN", "01234567890123456789012345678901")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Address != "127.0.0.1:8080" {
		t.Fatalf("address = %q", cfg.HTTP.Address)
	}
}

func TestRequiredSecrets(t *testing.T) {
	t.Setenv("CPPDEFENSE_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("CPPDEFENSE_SESSION_HASH_KEY", "")
	t.Setenv("CPPDEFENSE_CSRF_HASH_KEY", "")
	t.Setenv("CPPDEFENSE_OAUTH_FLOW_KEY", "")
	t.Setenv("CPPDEFENSE_GITHUB_CLIENT_ID", "")
	t.Setenv("CPPDEFENSE_GITHUB_CLIENT_SECRET", "")
	t.Setenv("CPPDEFENSE_BOOTSTRAP_ADMIN_GITHUB_ID", "")
	t.Setenv("CPPDEFENSE_RUNNER_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing secrets accepted")
	}
}
