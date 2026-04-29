package bdynd

import (
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SearchOptions struct {
	Type  string
	Name  string
	Since time.Time
	Until time.Time
}

type SearchResult struct {
	PackID    string
	PackName  string
	Path      string
	Kind      string
	OID       string
	Size      int64
	CreatedAt time.Time
}

func SearchPacks(r Repo, opts SearchOptions) ([]SearchResult, error) {
	packs, err := ListPacks(r)
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	for _, pack := range packs {
		if !matchesCreatedAt(pack.CreatedAt, opts.Since, opts.Until) {
			continue
		}
		for _, entry := range pack.Entries {
			if !matchesType(entry.Path, opts.Type) {
				continue
			}
			if !matchesName(entry.Path, opts.Name) {
				continue
			}
			results = append(results, SearchResult{
				PackID:    pack.ID,
				PackName:  pack.Name,
				Path:      entry.Path,
				Kind:      entry.Kind,
				OID:       entry.OID,
				Size:      entry.Size,
				CreatedAt: pack.CreatedAt,
			})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].CreatedAt.Equal(results[j].CreatedAt) {
			return results[i].Path < results[j].Path
		}
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})
	return results, nil
}

func matchesCreatedAt(created, since, until time.Time) bool {
	if !since.IsZero() && created.Before(since) {
		return false
	}
	if !until.IsZero() && created.After(until) {
		return false
	}
	return true
}

func matchesType(path, typ string) bool {
	typ = strings.TrimSpace(strings.TrimPrefix(typ, "."))
	if typ == "" {
		return true
	}
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	return strings.EqualFold(ext, typ)
}

func matchesName(path, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	base := filepath.Base(path)
	return strings.Contains(strings.ToLower(base), strings.ToLower(name))
}
