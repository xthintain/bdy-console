package repo

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

type Index struct {
	Added   []string          `json:"added,omitempty"`
	Removed []string          `json:"removed,omitempty"`
	Moved   map[string]string `json:"moved,omitempty"`
}

func LoadIndex(path string) (Index, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Index{}, nil
	}
	if err != nil {
		return Index{}, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, err
	}
	return idx, nil
}

func SaveIndex(path string, idx Index) error {
	return writeJSON(path, idx)
}

func (idx *Index) Add(path string) {
	path = CleanPath(path)
	if path == "" || contains(idx.Added, path) {
		return
	}
	idx.Added = append(idx.Added, path)
}

func (idx *Index) Remove(path string) {
	path = CleanPath(path)
	if path == "" || contains(idx.Removed, path) {
		return
	}
	idx.Removed = append(idx.Removed, path)
}

func (idx *Index) Move(oldPath, newPath string) {
	oldPath = CleanPath(oldPath)
	newPath = CleanPath(newPath)
	if idx.Moved == nil {
		idx.Moved = map[string]string{}
	}
	idx.Moved[oldPath] = newPath
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func ApplyIndex(base, current Manifest, idx Index) Manifest {
	baseMap := base.Map()
	curMap := current.Map()
	include := func(stagePath, entryPath string) bool {
		return stagePath == entryPath || strings.HasPrefix(entryPath, stagePath+"/")
	}
	for _, p := range idx.Removed {
		p = CleanPath(p)
		for existing := range baseMap {
			if include(p, existing) {
				delete(baseMap, existing)
			}
		}
	}
	for oldPath := range idx.Moved {
		oldPath = CleanPath(oldPath)
		for existing := range baseMap {
			if include(oldPath, existing) {
				delete(baseMap, existing)
			}
		}
	}
	for _, p := range idx.Added {
		p = CleanPath(p)
		for curPath, entry := range curMap {
			if include(p, curPath) {
				baseMap[curPath] = entry
			}
		}
	}
	for _, newPath := range idx.Moved {
		newPath = CleanPath(newPath)
		for curPath, entry := range curMap {
			if include(newPath, curPath) {
				baseMap[curPath] = entry
			}
		}
	}
	paths := make([]string, 0, len(baseMap))
	for p := range baseMap {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := Manifest{Entries: make([]Entry, 0, len(paths))}
	for _, p := range paths {
		out.Entries = append(out.Entries, baseMap[p])
	}
	return out
}
