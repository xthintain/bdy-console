package bdynd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type CleanOptions struct {
	DryRun bool
}

func Clean(r Repo, opts CleanOptions) ([]string, error) {
	idx, err := LoadIndex(r)
	if err != nil {
		return nil, err
	}
	ignore, err := LoadIgnore(r)
	if err != nil {
		return nil, err
	}
	var removed []string
	err = filepath.WalkDir(r.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(r.Root, path)
		if err != nil {
			return err
		}
		rel = cleanWorktreePath(rel)
		if rel == "" {
			return nil
		}
		if rel == DirName || strings.HasPrefix(rel, DirName+"/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if ignore.Ignored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := idx.Entries[rel]; ok || ignore.Ignored(rel, false) {
			return nil
		}
		removed = append(removed, rel)
		if !opts.DryRun {
			return os.Remove(path)
		}
		return nil
	})
	sort.Strings(removed)
	return removed, err
}
