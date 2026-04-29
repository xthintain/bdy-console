package bdynd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CommitOptions struct {
	Message string
}

type LogOptions struct {
	Limit int
}

type CommitObject struct {
	OID       string       `json:"oid"`
	Tree      string       `json:"tree"`
	Parent    string       `json:"parent,omitempty"`
	Message   string       `json:"message"`
	Time      time.Time    `json:"time"`
	Entries   []IndexEntry `json:"entries"`
	Committer string       `json:"committer,omitempty"`
}

func CommitChanges(r Repo, opts CommitOptions) (CommitObject, error) {
	return Commit(r, opts)
}

func Commit(r Repo, opts CommitOptions) (CommitObject, error) {
	msg := strings.TrimSpace(opts.Message)
	if msg == "" {
		return CommitObject{}, errors.New("commit message is required")
	}
	idx, err := LoadIndex(r)
	if err != nil {
		return CommitObject{}, err
	}
	entries := sortedIndexEntries(idx)
	tree, err := WriteTree(r, entries)
	if err != nil {
		return CommitObject{}, err
	}
	parent, _ := HeadCommit(r)
	c := CommitObject{Tree: tree, Parent: parent, Message: msg, Time: time.Now().UTC(), Entries: entries}
	data, err := json.Marshal(c)
	if err != nil {
		return CommitObject{}, err
	}
	c.OID = objectID("commit", data)
	if err := WriteCommitObject(r, c); err != nil {
		return CommitObject{}, err
	}
	if err := UpdateHead(r, c.OID); err != nil {
		return CommitObject{}, err
	}
	if err := appendHeadLog(r, parent, c.OID, msg); err != nil {
		return CommitObject{}, err
	}
	return c, nil
}

func HeadCommit(r Repo) (string, error) {
	ref, err := headRef(r)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(r.Dir, ref))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func UpdateHead(r Repo, oid string) error {
	ref, err := headRef(r)
	if err != nil {
		return err
	}
	path := filepath.Join(r.Dir, ref)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(oid+"\n"), 0o644)
}

func Log(r Repo, opts LogOptions) ([]CommitObject, error) {
	var out []CommitObject
	oid, err := HeadCommit(r)
	if err != nil || oid == "" {
		return out, nil
	}
	for oid != "" {
		c, err := ReadCommit(r, oid)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
		if opts.Limit > 0 && len(out) >= opts.Limit {
			break
		}
		oid = c.Parent
	}
	return out, nil
}

func headRef(r Repo) (string, error) {
	data, err := os.ReadFile(r.HeadPath())
	if err != nil {
		return "", err
	}
	raw := strings.TrimSpace(string(data))
	if !strings.HasPrefix(raw, "ref: ") {
		return "", errors.New("detached HEAD is not implemented")
	}
	return strings.TrimSpace(strings.TrimPrefix(raw, "ref: ")), nil
}

func appendHeadLog(r Repo, oldOID, newOID, message string) error {
	if err := os.MkdirAll(filepath.Join(r.Dir, "logs"), 0o755); err != nil {
		return err
	}
	line := time.Now().UTC().Format(time.RFC3339Nano) + " " + oldOID + " " + newOID + " " + message + "\n"
	f, err := os.OpenFile(filepath.Join(r.Dir, "logs", "HEAD"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}
