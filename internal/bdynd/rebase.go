package bdynd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type RebaseResult struct {
	Upstream  string
	Original  string
	Head      string
	Replayed  int
	Conflicts []string
}

func Rebase(r Repo, upstream string) (RebaseResult, error) {
	upstreamOID, err := ResolveRef(r, upstream)
	if err != nil {
		return RebaseResult{}, err
	}
	head, _ := HeadCommit(r)
	result := RebaseResult{Upstream: upstreamOID, Original: head}
	if head == "" || head == upstreamOID {
		result.Head = head
		return result, nil
	}
	if isAncestor(r, head, upstreamOID) {
		if err := fastForwardTo(r, upstreamOID); err != nil {
			return result, err
		}
		result.Head = upstreamOID
		return result, nil
	}
	baseOID, err := commonAncestor(r, head, upstreamOID)
	if err != nil {
		return result, err
	}
	commits, err := commitsAfter(r, head, baseOID)
	if err != nil {
		return result, err
	}
	if err := fastForwardTo(r, upstreamOID); err != nil {
		return result, err
	}
	for _, commit := range commits {
		conflicts, err := replayCommit(r, commit)
		if len(conflicts) > 0 {
			result.Conflicts = conflicts
			return result, fmt.Errorf("rebase conflicts: %v", conflicts)
		}
		if err != nil {
			return result, err
		}
		next, err := Commit(r, CommitOptions{Message: commit.Message})
		if err != nil {
			return result, err
		}
		result.Replayed++
		result.Head = next.OID
	}
	return result, nil
}

func commitsAfter(r Repo, head, base string) ([]CommitObject, error) {
	var reverse []CommitObject
	for oid := head; oid != "" && oid != base; {
		c, err := ReadCommit(r, oid)
		if err != nil {
			return nil, err
		}
		reverse = append(reverse, c)
		oid = c.Parent
	}
	commits := make([]CommitObject, 0, len(reverse))
	for i := len(reverse) - 1; i >= 0; i-- {
		commits = append(commits, reverse[i])
	}
	return commits, nil
}

func replayCommit(r Repo, commit CommitObject) ([]string, error) {
	parent := CommitObject{}
	if commit.Parent != "" {
		var err error
		parent, err = ReadCommit(r, commit.Parent)
		if err != nil {
			return nil, err
		}
	}
	baseEntries := entryMap(parent.Entries)
	nextEntries := entryMap(commit.Entries)
	idx, err := LoadIndex(r)
	if err != nil {
		return nil, err
	}
	var conflicts []string
	for _, path := range changedPaths(baseEntries, nextEntries) {
		baseEntry, hadBase := baseEntries[path]
		nextEntry, hasNext := nextEntries[path]
		currentEntry, hasCurrent := idx.Entries[path]
		if hasCurrent && (!hadBase || !sameEntry(currentEntry, baseEntry)) {
			conflicts = append(conflicts, path)
			continue
		}
		if !hasNext {
			if err := os.RemoveAll(filepath.Join(r.Root, path)); err != nil {
				return conflicts, err
			}
			delete(idx.Entries, path)
			continue
		}
		if err := writeIndexEntryToWorktree(r, nextEntry); err != nil {
			return conflicts, err
		}
		idx.Entries[path] = nextEntry
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return conflicts, nil
	}
	return nil, SaveIndex(r, idx)
}

func changedPaths(base, next map[string]IndexEntry) []string {
	seen := map[string]bool{}
	for path := range base {
		seen[path] = true
	}
	for path := range next {
		seen[path] = true
	}
	var paths []string
	for path := range seen {
		baseEntry, hadBase := base[path]
		nextEntry, hasNext := next[path]
		if hadBase != hasNext || !sameEntry(baseEntry, nextEntry) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}
