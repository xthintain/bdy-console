package bdynd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const oidPrefixSHA256 = "sha256:"

func WriteBlob(r Repo, data []byte) (string, error) {
	oid := objectID("blob", data)
	path, err := blobPath(r, oid)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return oid, nil
	}
	return oid, os.WriteFile(path, data, 0o644)
}

func ReadBlob(r Repo, oid string) ([]byte, error) {
	path, err := blobPath(r, oid)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func objectID(kind string, data []byte) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s %d\x00", kind, len(data))
	_, _ = h.Write(data)
	return oidPrefixSHA256 + hex.EncodeToString(h.Sum(nil))
}

func blobPath(r Repo, oid string) (string, error) {
	hash, err := oidHash(oid)
	if err != nil {
		return "", err
	}
	return filepath.Join(r.Dir, "objects", "blobs", "sha256", hash[:2], hash), nil
}

func oidHash(oid string) (string, error) {
	if !strings.HasPrefix(oid, oidPrefixSHA256) {
		return "", fmt.Errorf("unsupported oid %q", oid)
	}
	hash := strings.TrimPrefix(oid, oidPrefixSHA256)
	if len(hash) != sha256.Size*2 {
		return "", fmt.Errorf("invalid sha256 oid %q", oid)
	}
	return hash, nil
}
