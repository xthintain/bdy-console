package bdynd

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	MergeFastForward = "fast-forward"
	MergeMerged      = "merged"
	MergeConflict    = "conflict"
)

type MergeResult struct {
	Kind      string
	Commit    string
	Changed   []string
	Conflicts []string
}

func Merge(r Repo, ref string) (MergeResult, error) {
	theirsOID, err := ResolveRef(r, ref)
	if err != nil {
		return MergeResult{}, err
	}
	oursOID, _ := HeadCommit(r)
	if oursOID == "" || isAncestor(r, oursOID, theirsOID) {
		if err := fastForwardTo(r, theirsOID); err != nil {
			return MergeResult{}, err
		}
		return MergeResult{Kind: MergeFastForward, Commit: theirsOID}, nil
	}
	baseOID, err := commonAncestor(r, oursOID, theirsOID)
	if err != nil {
		return MergeResult{}, err
	}
	ours, err := ReadCommit(r, oursOID)
	if err != nil {
		return MergeResult{}, err
	}
	theirs, err := ReadCommit(r, theirsOID)
	if err != nil {
		return MergeResult{}, err
	}
	base, err := ReadCommit(r, baseOID)
	if err != nil {
		return MergeResult{}, err
	}
	result, err := mergeTrees(r, base, ours, theirs)
	if err != nil {
		return result, err
	}
	if len(result.Conflicts) > 0 {
		if err := os.WriteFile(filepath.Join(r.Dir, "MERGE_HEAD"), []byte(theirsOID+"\n"), 0o644); err != nil {
			return result, err
		}
		result.Kind = MergeConflict
		result.Commit = theirsOID
		return result, fmt.Errorf("conflicts: %v", result.Conflicts)
	}
	if err := Add(r, result.Changed); err != nil {
		return result, err
	}
	c, err := Commit(r, CommitOptions{Message: "merge " + ref})
	if err != nil {
		return result, err
	}
	result.Kind = MergeMerged
	result.Commit = c.OID
	return result, nil
}

func fastForwardTo(r Repo, oid string) error {
	c, err := ReadCommit(r, oid)
	if err != nil {
		return err
	}
	if err := CheckoutTree(r, c.Tree); err != nil {
		return err
	}
	idx := Index{Entries: map[string]IndexEntry{}}
	for _, entry := range c.Entries {
		idx.Entries[entry.Path] = entry
	}
	if err := SaveIndex(r, idx); err != nil {
		return err
	}
	return UpdateHead(r, oid)
}

func commonAncestor(r Repo, a, b string) (string, error) {
	seen := map[string]bool{}
	for oid := a; oid != ""; {
		seen[oid] = true
		c, err := ReadCommit(r, oid)
		if err != nil {
			return "", err
		}
		oid = c.Parent
	}
	for oid := b; oid != ""; {
		if seen[oid] {
			return oid, nil
		}
		c, err := ReadCommit(r, oid)
		if err != nil {
			return "", err
		}
		oid = c.Parent
	}
	return "", fmt.Errorf("no common ancestor")
}

func mergeTrees(r Repo, base, ours, theirs CommitObject) (MergeResult, error) {
	baseEntries := entryMap(base.Entries)
	ourEntries := entryMap(ours.Entries)
	theirEntries := entryMap(theirs.Entries)
	paths := map[string]bool{}
	for path := range baseEntries {
		paths[path] = true
	}
	for path := range ourEntries {
		paths[path] = true
	}
	for path := range theirEntries {
		paths[path] = true
	}
	var result MergeResult
	for path := range paths {
		baseEntry, hasBase := baseEntries[path]
		ourEntry, hasOurs := ourEntries[path]
		theirEntry, hasTheirs := theirEntries[path]
		switch {
		case hasTheirs && (!hasOurs || sameEntry(ourEntry, baseEntry)):
			if err := checkoutEntry(r, theirEntry); err != nil {
				return result, err
			}
			result.Changed = append(result.Changed, path)
		case hasOurs && (!hasTheirs || sameEntry(theirEntry, baseEntry)):
			continue
		case hasBase && hasOurs && hasTheirs && !sameEntry(ourEntry, theirEntry):
			if err := writeConflictFile(r, path, ourEntry, theirEntry); err != nil {
				return result, err
			}
			result.Conflicts = append(result.Conflicts, path)
		}
	}
	return result, nil
}

func entryMap(entries []IndexEntry) map[string]IndexEntry {
	out := map[string]IndexEntry{}
	for _, entry := range entries {
		out[entry.Path] = entry
	}
	return out
}

func sameEntry(a, b IndexEntry) bool {
	return a.Kind == b.Kind && a.OID == b.OID && a.LFSOID == b.LFSOID && a.Size == b.Size
}

func checkoutEntry(r Repo, entry IndexEntry) error {
	if entry.Kind != KindBlob {
		return nil
	}
	data, err := ReadBlob(r, entry.OID)
	if err != nil {
		return err
	}
	dest := filepath.Join(r.Root, entry.Path)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

func writeConflictFile(r Repo, path string, ours, theirs IndexEntry) error {
	oursData, err := ReadBlob(r, ours.OID)
	if err != nil {
		return err
	}
	theirsData, err := ReadBlob(r, theirs.OID)
	if err != nil {
		return err
	}
	data := []byte("<<<<<<< HEAD\n" + string(oursData) + "=======\n" + string(theirsData) + ">>>>>>> MERGE_HEAD\n")
	dest := filepath.Join(r.Root, path)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}
