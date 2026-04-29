package bdynd

import (
	"os"
	"path/filepath"
)

func CheckoutTree(r Repo, treeOID string) error {
	entries, err := ReadTree(r, treeOID)
	if err != nil {
		return err
	}
	current, _ := LoadIndex(r)
	next := map[string]IndexEntry{}
	for _, entry := range entries {
		next[entry.Path] = entry
	}
	for _, entry := range sortedIndexEntries(current) {
		if _, ok := next[entry.Path]; ok {
			continue
		}
		_ = os.Remove(filepath.Join(r.Root, entry.Path))
	}
	for _, entry := range entries {
		if entry.Kind != KindBlob {
			continue
		}
		data, err := ReadBlob(r, entry.OID)
		if err != nil {
			return err
		}
		dest := filepath.Join(r.Root, entry.Path)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
