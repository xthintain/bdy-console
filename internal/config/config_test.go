package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveUsesConfigHomeAndMode0600(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BDY_CONFIG_HOME", dir)
	cfg := Config{AccessToken: "token", ExpiresAt: time.Now().Add(time.Hour)}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 0600", got)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessToken != "token" {
		t.Fatalf("loaded config mismatch: %+v", loaded)
	}
}

func TestTemporaryConfigIsSeparateAndPreferredWhileValid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BDY_CONFIG_HOME", dir)
	base := Config{AccessToken: "base-token", RefreshToken: "base-refresh", ExpiresAt: time.Now().Add(time.Hour)}
	if err := Save(base); err != nil {
		t.Fatal(err)
	}
	temp := base
	temp.AccessToken = "temporary-token"
	temp.Temporary = true
	temp.ReadOnly = true
	temp.TemporaryExpiresAt = time.Now().Add(24 * time.Hour)
	if err := SaveTemporary(temp); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "temporary.json")); err != nil {
		t.Fatal(err)
	}
	active, err := LoadActive()
	if err != nil {
		t.Fatal(err)
	}
	if active.AccessToken != "temporary-token" || !active.Temporary || !active.ReadOnly {
		t.Fatalf("active=%+v", active)
	}
}

func TestLoadActivePrefersEnvToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BDY_CONFIG_HOME", dir)
	t.Setenv("BDY_ACCESS_TOKEN", "env-token")
	t.Setenv("BDY_REFRESH_TOKEN", "env-refresh")
	t.Setenv("BDY_TOKEN_EXPIRES_IN", "3600")
	t.Setenv("BDY_READ_ONLY", "1")
	base := Config{AccessToken: "base-token", ExpiresAt: time.Now().Add(time.Hour)}
	if err := Save(base); err != nil {
		t.Fatal(err)
	}
	active, err := LoadActive()
	if err != nil {
		t.Fatal(err)
	}
	if active.AccessToken != "env-token" || active.RefreshToken != "env-refresh" || !active.ReadOnly {
		t.Fatalf("active=%+v", active)
	}
}

func TestHasAppReportsAppCredentials(t *testing.T) {
	empty := Config{}
	if empty.HasApp() {
		t.Fatal("empty config should not have app credentials")
	}
	noSecret := Config{AppKey: "ak"}
	if noSecret.HasApp() {
		t.Fatal("missing secret key should not have app credentials")
	}
	full := Config{AppKey: "ak", SecretKey: "sk"}
	if !full.HasApp() {
		t.Fatal("app credentials should be reported")
	}
}
