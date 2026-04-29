package repo

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	DirName           = ".bdy"
	AppRoot           = "/apps/baiduyunStorage"
	DefaultRemoteRoot = AppRoot + "/workspace"
	CmdRoot           = AppRoot
)

type Config struct {
	RemoteRoot string `json:"remote_root"`
}

type Repo struct {
	Root   string
	Dir    string
	Config Config
}

func Init(root, remoteRoot string) (Repo, error) {
	if remoteRoot == "" {
		remoteRoot = DefaultRemoteRoot
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Repo{}, err
	}
	dir := filepath.Join(abs, DirName)
	r := Repo{Root: abs, Dir: dir, Config: Config{RemoteRoot: remoteRoot}}
	if err := os.MkdirAll(filepath.Join(dir, "commits"), 0o755); err != nil {
		return Repo{}, err
	}
	if err := writeJSON(filepath.Join(dir, "config.json"), r.Config); err != nil {
		return Repo{}, err
	}
	if _, err := os.Stat(filepath.Join(dir, "index.json")); errors.Is(err, os.ErrNotExist) {
		if err := writeJSON(filepath.Join(dir, "index.json"), Index{}); err != nil {
			return Repo{}, err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); errors.Is(err, os.ErrNotExist) {
		if err := SaveManifest(filepath.Join(dir, "manifest.json"), Manifest{}); err != nil {
			return Repo{}, err
		}
	}
	return r, nil
}

func Open(start string) (Repo, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return Repo{}, err
	}
	for {
		dir := filepath.Join(abs, DirName)
		data, err := os.ReadFile(filepath.Join(dir, "config.json"))
		if err == nil {
			var cfg Config
			if err := json.Unmarshal(data, &cfg); err != nil {
				return Repo{}, err
			}
			return Repo{Root: abs, Dir: dir, Config: cfg}, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return Repo{}, errors.New("not a bdy repo; run bdy init")
		}
		abs = parent
	}
}

func (r Repo) ManifestPath() string {
	return filepath.Join(r.Dir, "manifest.json")
}

func (r Repo) IndexPath() string {
	return filepath.Join(r.Dir, "index.json")
}

func (r Repo) CommitsDir() string {
	return filepath.Join(r.Dir, "commits")
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
