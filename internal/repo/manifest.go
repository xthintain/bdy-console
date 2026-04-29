package repo

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Entry struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
	MD5   string `json:"md5,omitempty"`
	IsDir bool   `json:"is_dir"`
}

type Manifest struct {
	Entries []Entry `json:"entries"`
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Manifest{}, nil
	}
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	m.Sort()
	return m, nil
}

func SaveManifest(path string, m Manifest) error {
	m.Sort()
	return writeJSON(path, m)
}

func (m *Manifest) Sort() {
	sort.Slice(m.Entries, func(i, j int) bool {
		return m.Entries[i].Path < m.Entries[j].Path
	})
}

func (m Manifest) Map() map[string]Entry {
	out := make(map[string]Entry, len(m.Entries))
	for _, e := range m.Entries {
		out[e.Path] = e
	}
	return out
}

func (m Manifest) Hash() (string, error) {
	m.Sort()
	raw, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := md5.Sum(raw)
	return hex.EncodeToString(sum[:]), nil
}

func Scan(root string) (Manifest, error) {
	var entries []Entry
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == DirName || strings.HasPrefix(rel, DirName+"/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		e := Entry{Path: rel, Size: info.Size(), MTime: info.ModTime().Unix(), IsDir: d.IsDir()}
		if !d.IsDir() {
			sum, err := fileMD5(path)
			if err != nil {
				return err
			}
			e.MD5 = sum
		}
		entries = append(entries, e)
		return nil
	})
	if err != nil {
		return Manifest{}, err
	}
	m := Manifest{Entries: entries}
	m.Sort()
	return m, nil
}

func fileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func CleanPath(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	p = strings.TrimPrefix(p, "./")
	if p == "." {
		return ""
	}
	return p
}
