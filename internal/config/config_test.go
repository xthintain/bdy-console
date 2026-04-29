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
	cfg := Config{AppID: "1", AppKey: "key", SecretKey: "secret", ExpiresAt: time.Now().Add(time.Hour)}
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
	if loaded.AppKey != "key" || loaded.SecretKey != "secret" {
		t.Fatalf("loaded config mismatch: %+v", loaded)
	}
}
