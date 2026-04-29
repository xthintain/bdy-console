package bdynd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Stash struct {
	ID      string       `json:"id"`
	Message string       `json:"message"`
	Time    time.Time    `json:"time"`
	Base    string       `json:"base"`
	Entries []IndexEntry `json:"entries"`
}

func StashPush(r Repo, message string) (Stash, error) {
	base, err := HeadCommit(r)
	if err != nil || base == "" {
		return Stash{}, errors.New("stash requires at least one commit")
	}
	entries, err := scanWorktreeEntries(r)
	if err != nil {
		return Stash{}, err
	}
	now := time.Now().UTC()
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "WIP"
	}
	id := stashID(now, msg)
	stash := Stash{ID: id, Message: msg, Time: now, Base: base, Entries: entries}
	if err := os.MkdirAll(stashDir(r), 0o755); err != nil {
		return Stash{}, err
	}
	if err := writeJSON(stashPath(r, id), stash); err != nil {
		return Stash{}, err
	}
	c, err := ReadCommit(r, base)
	if err != nil {
		return Stash{}, err
	}
	if err := checkoutEntries(r, c.Entries); err != nil {
		return Stash{}, err
	}
	return stash, SaveIndex(r, indexFromEntries(c.Entries))
}

func StashList(r Repo) ([]Stash, error) {
	items, err := os.ReadDir(stashDir(r))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stashes []Stash
	for _, item := range items {
		if item.IsDir() || !strings.HasSuffix(item.Name(), ".json") {
			continue
		}
		stash, err := readStashFile(filepath.Join(stashDir(r), item.Name()))
		if err != nil {
			return nil, err
		}
		stashes = append(stashes, stash)
	}
	sort.Slice(stashes, func(i, j int) bool {
		return stashes[i].Time.After(stashes[j].Time)
	})
	return stashes, nil
}

func StashPop(r Repo, id string) error {
	stash, err := resolveStash(r, id)
	if err != nil {
		return err
	}
	if err := checkoutEntries(r, stash.Entries); err != nil {
		return err
	}
	if err := SaveIndex(r, indexFromEntries(stash.Entries)); err != nil {
		return err
	}
	return os.Remove(stashPath(r, stash.ID))
}

func scanWorktreeEntries(r Repo) ([]IndexEntry, error) {
	idx := Index{Entries: map[string]IndexEntry{}}
	if err := filepath.WalkDir(r.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == DirName {
				return filepath.SkipDir
			}
			return nil
		}
		return addFile(r, idx, path)
	}); err != nil {
		return nil, err
	}
	return sortedIndexEntries(idx), nil
}

func checkoutEntries(r Repo, entries []IndexEntry) error {
	current, err := scanWorktreeEntries(r)
	if err != nil {
		return err
	}
	next := map[string]IndexEntry{}
	for _, entry := range entries {
		next[entry.Path] = entry
	}
	for _, entry := range current {
		if _, ok := next[entry.Path]; ok {
			continue
		}
		_ = os.Remove(filepath.Join(r.Root, entry.Path))
	}
	for _, entry := range entries {
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
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func indexFromEntries(entries []IndexEntry) Index {
	idx := Index{Entries: map[string]IndexEntry{}}
	for _, entry := range entries {
		idx.Entries[entry.Path] = entry
	}
	return idx
}

func resolveStash(r Repo, id string) (Stash, error) {
	if id != "" {
		return readStashFile(stashPath(r, id))
	}
	stashes, err := StashList(r)
	if err != nil {
		return Stash{}, err
	}
	if len(stashes) == 0 {
		return Stash{}, errors.New("no stash entries")
	}
	return stashes[0], nil
}

func readStashFile(path string) (Stash, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Stash{}, err
	}
	var stash Stash
	return stash, json.Unmarshal(data, &stash)
}

func stashDir(r Repo) string {
	return filepath.Join(r.Dir, "refs", "stash")
}

func stashPath(r Repo, id string) string {
	return filepath.Join(stashDir(r), id+".json")
}

func stashID(t time.Time, message string) string {
	oid := strings.TrimPrefix(objectID("stash", []byte(t.Format(time.RFC3339Nano)+"\x00"+message)), oidPrefixSHA256)
	return t.Format("20060102150405") + "-" + oid[:12]
}
