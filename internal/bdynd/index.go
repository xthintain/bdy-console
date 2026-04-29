package bdynd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	KindBlob = "blob"
	KindLFS  = "lfs"
)

type Index struct {
	Entries map[string]IndexEntry `json:"entries"`
}

type IndexEntry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	OID    string `json:"oid,omitempty"`
	LFSOID string `json:"lfs_oid,omitempty"`
	Size   int64  `json:"size"`
}

func LoadIndex(r Repo) (Index, error) {
	data, err := os.ReadFile(r.IndexPath())
	if os.IsNotExist(err) {
		return Index{Entries: map[string]IndexEntry{}}, nil
	}
	if err != nil {
		return Index{}, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, err
	}
	if idx.Entries == nil {
		idx.Entries = map[string]IndexEntry{}
	}
	return idx, nil
}

func SaveIndex(r Repo, idx Index) error {
	if idx.Entries == nil {
		idx.Entries = map[string]IndexEntry{}
	}
	return writeJSON(r.IndexPath(), idx)
}

func Add(r Repo, paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("add requires at least one path")
	}
	idx, err := LoadIndex(r)
	if err != nil {
		return err
	}
	ignore, err := LoadIgnore(r)
	if err != nil {
		return err
	}
	for _, p := range paths {
		if err := addPath(r, idx, p, ignore); err != nil {
			return err
		}
	}
	return SaveIndex(r, idx)
}

func addPath(r Repo, idx Index, p string, ignore IgnoreMatcher) error {
	abs := filepath.Join(r.Root, cleanWorktreePath(p))
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(r.Root, abs)
	if err != nil {
		return err
	}
	if ignore.Ignored(rel, info.IsDir()) {
		return nil
	}
	if info.IsDir() {
		return filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(r.Root, path)
			if err != nil {
				return err
			}
			if d.IsDir() {
				if ignore.Ignored(rel, true) {
					return filepath.SkipDir
				}
				return nil
			}
			return addFile(r, idx, path, ignore)
		})
	}
	return addFile(r, idx, abs, ignore)
}

func addFile(r Repo, idx Index, abs string, ignore IgnoreMatcher) error {
	rel, err := filepath.Rel(r.Root, abs)
	if err != nil {
		return err
	}
	rel = cleanWorktreePath(rel)
	if rel == "" || ignore.Ignored(rel, false) {
		return nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	isLFS, err := MatchesLFSPattern(r, rel)
	if err != nil {
		return err
	}
	if isLFS {
		p, err := StoreLFSFile(r, abs)
		if err != nil {
			return err
		}
		idx.Entries[rel] = IndexEntry{Path: rel, Kind: KindLFS, LFSOID: p.OID, Size: p.Size}
		return nil
	}
	oid, err := WriteBlob(r, data)
	if err != nil {
		return err
	}
	idx.Entries[rel] = IndexEntry{Path: rel, Kind: KindBlob, OID: oid, Size: int64(len(data))}
	return nil
}

func (r Repo) IndexPath() string {
	return filepath.Join(r.Dir, "index.json")
}

func cleanWorktreePath(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	p = strings.TrimPrefix(p, "./")
	if p == "." {
		return ""
	}
	return p
}

func sortedIndexEntries(idx Index) []IndexEntry {
	paths := make([]string, 0, len(idx.Entries))
	for path := range idx.Entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	out := make([]IndexEntry, 0, len(paths))
	for _, path := range paths {
		out = append(out, idx.Entries[path])
	}
	return out
}
