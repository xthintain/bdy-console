package repo

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Commit struct {
	ID           string    `json:"id"`
	Time         time.Time `json:"time"`
	Message      string    `json:"message"`
	Parent       string    `json:"parent,omitempty"`
	ManifestHash string    `json:"manifest_hash"`
	Entries      []Entry   `json:"entries"`
}

func CreateCommit(r Repo, message string, m Manifest) (Commit, error) {
	hash, err := m.Hash()
	if err != nil {
		return Commit{}, err
	}
	now := time.Now().UTC()
	id := commitID(now, message, hash)
	c := Commit{ID: id, Time: now, Message: message, ManifestHash: hash, Entries: m.Entries}
	if err := writeJSON(filepath.Join(r.CommitsDir(), id+".json"), c); err != nil {
		return Commit{}, err
	}
	return c, nil
}

func LatestCommitID(r Repo) string {
	items, err := os.ReadDir(r.CommitsDir())
	if err != nil || len(items) == 0 {
		return ""
	}
	var latest string
	for _, item := range items {
		name := item.Name()
		if filepath.Ext(name) == ".json" && name > latest {
			latest = name
		}
	}
	if latest == "" {
		return ""
	}
	return latest[:len(latest)-len(".json")]
}

func commitID(t time.Time, message, manifestHash string) string {
	raw, _ := json.Marshal([]string{t.Format(time.RFC3339Nano), message, manifestHash})
	sum := md5.Sum(raw)
	return t.Format("20060102150405") + "-" + hex.EncodeToString(sum[:])[:12]
}
