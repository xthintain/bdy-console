package lfs

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"baiduyunStorage/internal/repo"
)

const (
	RemoteRoot = repo.AppRoot + "/lfs"
)

type ObjectMeta struct {
	OID        string    `json:"oid"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"created_at"`
	RemotePath string    `json:"remote_path"`
}

func GitRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not a git repository")
		}
		dir = parent
	}
}

func BdyDir(root string) string {
	return filepath.Join(root, repo.DirName)
}

func ObjectPath(root string, p Pointer) string {
	sha := SHA(p)
	return filepath.Join(BdyDir(root), "lfs", "objects", "sha256", sha[:2], sha[2:4], sha)
}

func TransferPath(root string, p Pointer, suffix string) string {
	sha := SHA(p)
	return filepath.Join(BdyDir(root), "lfs", "transfers", sha+"."+suffix+".json")
}

func TempDir(root string) string {
	return filepath.Join(BdyDir(root), "lfs", "tmp")
}

func RemoteObjectPath(p Pointer) string {
	sha := SHA(p)
	return RemoteRoot + "/objects/sha256/" + sha[:2] + "/" + sha[2:4] + "/" + sha
}

func RemoteMetaPath(p Pointer) string {
	return RemoteRoot + "/meta/" + SHA(p) + ".json"
}

func StoreObject(root, src string, p Pointer) error {
	dest := ObjectPath(root, p)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	tmp := dest + ".tmp"
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	got, err := HashFile(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if got.OID != p.OID || got.Size != p.Size {
		_ = os.Remove(tmp)
		return fmt.Errorf("object verification failed for %s", p.OID)
	}
	return os.Rename(tmp, dest)
}

func WritePointerFile(path string, p Pointer) error {
	return os.WriteFile(path, []byte(FormatPointer(p)), 0o644)
}

func ReadPointerFile(path string) (Pointer, error) {
	f, err := os.Open(path)
	if err != nil {
		return Pointer{}, err
	}
	defer f.Close()
	return ParsePointer(f)
}

func IsPointerFile(path string) (Pointer, bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 1024 {
		return Pointer{}, false
	}
	p, err := ParsePointer(strings.NewReader(string(data)))
	return p, err == nil
}

func WriteMeta(path string, meta ObjectMeta) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func ReadMeta(path string) (ObjectMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ObjectMeta{}, err
	}
	var meta ObjectMeta
	return meta, json.Unmarshal(data, &meta)
}
