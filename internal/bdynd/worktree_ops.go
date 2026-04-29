package bdynd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ResetSoft  = "soft"
	ResetMixed = "mixed"
	ResetHard  = "hard"
)

func Diff(r Repo) (StatusResult, error) {
	worktree, err := scanWorktreeEntries(r)
	if err != nil {
		return StatusResult{}, err
	}
	worktreeIndex := indexFromEntries(worktree)
	head, err := HeadCommit(r)
	if err != nil || head == "" {
		var st StatusResult
		for _, entry := range sortedIndexEntries(worktreeIndex) {
			st.Added = append(st.Added, entry.Path)
		}
		return st, nil
	}
	c, err := ReadCommit(r, head)
	if err != nil {
		return StatusResult{}, err
	}
	return compareEntries(c.Entries, sortedIndexEntries(worktreeIndex)), nil
}

func Remove(r Repo, paths []string) error {
	if len(paths) == 0 {
		return errors.New("rm requires at least one path")
	}
	idx, err := LoadIndex(r)
	if err != nil {
		return err
	}
	for _, p := range paths {
		rel := cleanWorktreePath(p)
		if rel == "" || rel == DirName || strings.HasPrefix(rel, DirName+"/") {
			return fmt.Errorf("invalid path %q", p)
		}
		if err := os.RemoveAll(filepath.Join(r.Root, rel)); err != nil {
			return err
		}
		removeIndexPath(idx, rel)
	}
	return SaveIndex(r, idx)
}

func Move(r Repo, oldPath, newPath string) error {
	oldRel := cleanWorktreePath(oldPath)
	newRel := cleanWorktreePath(newPath)
	if oldRel == "" || newRel == "" {
		return errors.New("mv requires old and new paths")
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(r.Root, newRel)), 0o755); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(r.Root, oldRel), filepath.Join(r.Root, newRel)); err != nil {
		return err
	}
	idx, err := LoadIndex(r)
	if err != nil {
		return err
	}
	removeIndexPath(idx, oldRel)
	ignore, err := LoadIgnore(r)
	if err != nil {
		return err
	}
	if err := addPath(r, idx, newRel, ignore); err != nil {
		return err
	}
	return SaveIndex(r, idx)
}

func Restore(r Repo, paths []string) error {
	if len(paths) == 0 {
		return errors.New("restore requires at least one path")
	}
	head, err := HeadCommit(r)
	if err != nil || head == "" {
		return errors.New("restore requires at least one commit")
	}
	c, err := ReadCommit(r, head)
	if err != nil {
		return err
	}
	headEntries := map[string]IndexEntry{}
	for _, entry := range c.Entries {
		headEntries[entry.Path] = entry
	}
	idx, err := LoadIndex(r)
	if err != nil {
		return err
	}
	for _, p := range paths {
		rel := cleanWorktreePath(p)
		entry, ok := headEntries[rel]
		if !ok {
			if err := os.RemoveAll(filepath.Join(r.Root, rel)); err != nil {
				return err
			}
			removeIndexPath(idx, rel)
			continue
		}
		if err := writeIndexEntryToWorktree(r, entry); err != nil {
			return err
		}
		idx.Entries[rel] = entry
	}
	return SaveIndex(r, idx)
}

func Reset(r Repo, ref, mode string) error {
	if strings.TrimSpace(ref) == "" {
		return errors.New("reset requires a ref")
	}
	if mode == "" {
		mode = ResetMixed
	}
	if mode != ResetSoft && mode != ResetMixed && mode != ResetHard {
		return fmt.Errorf("unsupported reset mode %q", mode)
	}
	oid, err := ResolveRef(r, ref)
	if err != nil {
		return err
	}
	c, err := ReadCommit(r, oid)
	if err != nil {
		return err
	}
	if err := UpdateHead(r, oid); err != nil {
		return err
	}
	if mode == ResetSoft {
		return nil
	}
	if err := SaveIndex(r, indexFromEntries(c.Entries)); err != nil {
		return err
	}
	if mode == ResetMixed {
		return nil
	}
	return checkoutEntries(r, c.Entries)
}

func compareEntries(base, next []IndexEntry) StatusResult {
	baseMap := map[string]IndexEntry{}
	for _, entry := range base {
		baseMap[entry.Path] = entry
	}
	var st StatusResult
	for _, entry := range next {
		baseEntry, ok := baseMap[entry.Path]
		if !ok {
			st.Added = append(st.Added, entry.Path)
			continue
		}
		if entry.Kind != baseEntry.Kind || entry.OID != baseEntry.OID || entry.LFSOID != baseEntry.LFSOID {
			st.Modified = append(st.Modified, entry.Path)
		}
		delete(baseMap, entry.Path)
	}
	for path := range baseMap {
		st.Deleted = append(st.Deleted, path)
	}
	sort.Strings(st.Added)
	sort.Strings(st.Modified)
	sort.Strings(st.Deleted)
	return st
}

func removeIndexPath(idx Index, rel string) {
	delete(idx.Entries, rel)
	prefix := strings.TrimSuffix(rel, "/") + "/"
	for path := range idx.Entries {
		if strings.HasPrefix(path, prefix) {
			delete(idx.Entries, path)
		}
	}
}

func writeIndexEntryToWorktree(r Repo, entry IndexEntry) error {
	var data []byte
	switch entry.Kind {
	case KindBlob:
		var err error
		data, err = ReadBlob(r, entry.OID)
		if err != nil {
			return err
		}
	case KindLFS:
		var err error
		data, err = os.ReadFile(LFSObjectPath(r, entry.LFSOID))
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported entry kind %q", entry.Kind)
	}
	dest := filepath.Join(r.Root, entry.Path)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}
