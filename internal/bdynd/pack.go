package bdynd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type PackOptions struct {
	Ref  string
	Name string
}

type PackManifest struct {
	ID        string      `json:"id"`
	Name      string      `json:"name,omitempty"`
	Ref       string      `json:"ref"`
	CreatedAt time.Time   `json:"created_at"`
	Entries   []PackEntry `json:"entries"`
}

type PackEntry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	OID    string `json:"oid"`
	Size   int64  `json:"size"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
}

func Pack(r Repo, opts PackOptions) (PackManifest, error) {
	ref := strings.TrimSpace(opts.Ref)
	if ref == "" {
		ref = "HEAD"
	}
	oid, err := ResolveRef(r, ref)
	if err != nil {
		return PackManifest{}, err
	}
	c, err := ReadCommit(r, oid)
	if err != nil {
		return PackManifest{}, err
	}
	if len(c.Entries) == 0 {
		return PackManifest{}, errors.New("pack requires at least one file entry")
	}
	now := time.Now().UTC()
	id := packID(oid, opts.Name, now)
	if err := os.MkdirAll(packDir(r), 0o755); err != nil {
		return PackManifest{}, err
	}
	packPath := filepath.Join(packDir(r), id+".pack")
	f, err := os.Create(packPath)
	if err != nil {
		return PackManifest{}, err
	}
	var writeErr error
	defer func() {
		if writeErr != nil {
			_ = os.Remove(packPath)
		}
	}()
	defer f.Close()

	manifest := PackManifest{ID: id, Name: strings.TrimSpace(opts.Name), Ref: oid, CreatedAt: now}
	var offset int64
	for _, entry := range c.Entries {
		data, err := packEntryData(r, entry)
		if err != nil {
			writeErr = err
			return PackManifest{}, err
		}
		n, err := f.Write(data)
		if err != nil {
			writeErr = err
			return PackManifest{}, err
		}
		objectOID := entry.OID
		if entry.Kind == KindLFS {
			objectOID = entry.LFSOID
		}
		manifest.Entries = append(manifest.Entries, PackEntry{
			Path:   entry.Path,
			Kind:   entry.Kind,
			OID:    objectOID,
			Size:   entry.Size,
			Offset: offset,
			Length: int64(n),
		})
		offset += int64(n)
	}
	if err := f.Close(); err != nil {
		writeErr = err
		return PackManifest{}, err
	}
	if err := writeJSON(filepath.Join(packDir(r), id+".json"), manifest); err != nil {
		writeErr = err
		return PackManifest{}, err
	}
	return manifest, nil
}

func ListPacks(r Repo) ([]PackManifest, error) {
	items, err := os.ReadDir(packDir(r))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var packs []PackManifest
	for _, item := range items {
		if item.IsDir() || !strings.HasSuffix(item.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(packDir(r), item.Name()))
		if err != nil {
			return nil, err
		}
		var manifest PackManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, err
		}
		packs = append(packs, manifest)
	}
	sort.Slice(packs, func(i, j int) bool {
		return packs[i].CreatedAt.After(packs[j].CreatedAt)
	})
	return packs, nil
}

func packEntryData(r Repo, entry IndexEntry) ([]byte, error) {
	switch entry.Kind {
	case KindBlob:
		return ReadBlob(r, entry.OID)
	case KindLFS:
		return os.ReadFile(LFSObjectPath(r, entry.LFSOID))
	default:
		return nil, fmt.Errorf("unsupported entry kind %q", entry.Kind)
	}
}

func packDir(r Repo) string {
	return filepath.Join(r.Dir, "packs")
}

func packID(ref, name string, t time.Time) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\x00%s\x00%s", ref, strings.TrimSpace(name), t.Format(time.RFC3339Nano))
	return t.Format("20060102150405") + "-" + hex.EncodeToString(h.Sum(nil))[:12]
}
