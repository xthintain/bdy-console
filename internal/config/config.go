package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	AppID              string    `json:"app_id,omitempty"`
	AppKey             string    `json:"app_key,omitempty"`
	SecretKey          string    `json:"secret_key,omitempty"`
	SignKey            string    `json:"sign_key,omitempty"`
	AccessToken        string    `json:"access_token,omitempty"`
	RefreshToken       string    `json:"refresh_token,omitempty"`
	ExpiresAt          time.Time `json:"expires_at,omitempty"`
	Temporary          bool      `json:"temporary,omitempty"`
	ReadOnly           bool      `json:"read_only,omitempty"`
	TemporaryExpiresAt time.Time `json:"temporary_expires_at,omitempty"`
}

func Path() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func TemporaryPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "temporary.json"), nil
}

func configDir() (string, error) {
	if dir := os.Getenv("BDY_CONFIG_HOME"); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "bdy"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "bdy"), nil
}

func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	cfg.Temporary = false
	cfg.ReadOnly = false
	cfg.TemporaryExpiresAt = time.Time{}
	return savePath(path, cfg)
}

func LoadTemporary() (Config, error) {
	path, err := TemporaryPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("read temporary config: %w", err)
	}
	return cfg, nil
}

func SaveTemporary(cfg Config) error {
	path, err := TemporaryPath()
	if err != nil {
		return err
	}
	cfg.Temporary = true
	cfg.ReadOnly = true
	return savePath(path, cfg)
}

func LoadActive() (Config, error) {
	temp, err := LoadTemporary()
	if err != nil {
		return Config{}, err
	}
	if temp.Temporary && temp.ReadOnly && temp.HasToken() && time.Now().Before(temp.TemporaryExpiresAt) {
		return temp, nil
	}
	return Load()
}

func savePath(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c Config) HasApp() bool {
	return c.AppKey != "" && c.SecretKey != ""
}

func (c Config) HasToken() bool {
	return c.AccessToken != "" && time.Now().Before(c.ExpiresAt.Add(-5*time.Minute))
}

func (c Config) IsTemporaryReadOnly() bool {
	return c.Temporary && c.ReadOnly && time.Now().Before(c.TemporaryExpiresAt)
}
