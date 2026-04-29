package bdynd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func Init(root string, opts InitOptions) (Repo, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Repo{}, err
	}
	cfg := Config{DefaultBranch: DefaultBranch, Remotes: map[string]string{}}
	if opts.RemoteRoot != "" {
		name := opts.RemoteName
		if name == "" {
			name = DefaultRemote
		}
		cfg.Remotes[name] = opts.RemoteRoot
	}
	r := Repo{Root: abs, Dir: filepath.Join(abs, DirName), Config: cfg}
	dirs := []string{
		filepath.Join(r.Dir, "refs", "heads"),
		filepath.Join(r.Dir, "refs", "tags"),
		filepath.Join(r.Dir, "refs", "remotes"),
		filepath.Join(r.Dir, "objects", "blobs"),
		filepath.Join(r.Dir, "objects", "trees"),
		filepath.Join(r.Dir, "objects", "commits"),
		filepath.Join(r.Dir, "lfs", "objects"),
		filepath.Join(r.Dir, "logs"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Repo{}, err
		}
	}
	if err := writeJSON(r.ConfigPath(), cfg); err != nil {
		return Repo{}, err
	}
	if err := writeFileIfMissing(r.HeadPath(), []byte("ref: refs/heads/main\n")); err != nil {
		return Repo{}, err
	}
	if err := writeFileIfMissing(filepath.Join(r.Dir, "refs", "heads", "main"), nil); err != nil {
		return Repo{}, err
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
			if cfg.DefaultBranch == "" {
				cfg.DefaultBranch = DefaultBranch
			}
			if cfg.Remotes == nil {
				cfg.Remotes = map[string]string{}
			}
			return Repo{Root: abs, Dir: dir, Config: cfg}, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return Repo{}, errors.New("not a bdy nd repo; run bdy nd init")
		}
		abs = parent
	}
}

func (r Repo) ConfigPath() string {
	return filepath.Join(r.Dir, "config.json")
}

func (r Repo) HeadPath() string {
	return filepath.Join(r.Dir, "HEAD")
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeFileIfMissing(path string, data []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
