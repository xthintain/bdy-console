package bdynd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const LFSSpecVersion = "https://bdy-lfs/spec/v1"

type Attributes struct {
	Patterns []string `json:"patterns"`
}

type LFSPointer struct {
	OID  string
	Size int64
}

func TrackPattern(r Repo, pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return errors.New("lfs track requires a pattern")
	}
	attrs, err := LoadAttributes(r)
	if err != nil {
		return err
	}
	for _, existing := range attrs.Patterns {
		if existing == pattern {
			return nil
		}
	}
	attrs.Patterns = append(attrs.Patterns, pattern)
	return SaveAttributes(r, attrs)
}

func UntrackPattern(r Repo, pattern string) error {
	attrs, err := LoadAttributes(r)
	if err != nil {
		return err
	}
	var next []string
	for _, existing := range attrs.Patterns {
		if existing != pattern {
			next = append(next, existing)
		}
	}
	attrs.Patterns = next
	return SaveAttributes(r, attrs)
}

func TrackedPatterns(r Repo) ([]string, error) {
	attrs, err := LoadAttributes(r)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), attrs.Patterns...), nil
}

func LoadAttributes(r Repo) (Attributes, error) {
	data, err := os.ReadFile(r.AttributesPath())
	if os.IsNotExist(err) {
		return Attributes{}, nil
	}
	if err != nil {
		return Attributes{}, err
	}
	var attrs Attributes
	if err := json.Unmarshal(data, &attrs); err != nil {
		return Attributes{}, err
	}
	return attrs, nil
}

func SaveAttributes(r Repo, attrs Attributes) error {
	return writeJSON(r.AttributesPath(), attrs)
}

func MatchesLFSPattern(r Repo, rel string) (bool, error) {
	patterns, err := TrackedPatterns(r)
	if err != nil {
		return false, err
	}
	base := filepath.Base(rel)
	for _, pattern := range patterns {
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true, nil
		}
		if ok, _ := filepath.Match(pattern, base); ok {
			return true, nil
		}
	}
	return false, nil
}

func StoreLFSFile(r Repo, path string) (LFSPointer, error) {
	f, err := os.Open(path)
	if err != nil {
		return LFSPointer{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return LFSPointer{}, err
	}
	p := LFSPointer{OID: oidPrefixSHA256 + hex.EncodeToString(h.Sum(nil)), Size: n}
	dest := LFSObjectPath(r, p.OID)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return LFSPointer{}, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return LFSPointer{}, err
	}
	out, err := os.Create(dest)
	if err != nil {
		return LFSPointer{}, err
	}
	defer out.Close()
	if _, err := io.Copy(out, f); err != nil {
		return LFSPointer{}, err
	}
	return p, nil
}

func LFSObjectPath(r Repo, oid string) string {
	hash := strings.TrimPrefix(oid, oidPrefixSHA256)
	prefix := hash
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	return filepath.Join(r.Dir, "lfs", "objects", "sha256", prefix, hash)
}

func FormatLFSPointer(oid string, size int64) string {
	return fmt.Sprintf("version %s\noid %s\nsize %d\n", LFSSpecVersion, oid, size)
}

func ParseLFSPointer(reader io.Reader) (LFSPointer, error) {
	scanner := bufio.NewScanner(reader)
	var p LFSPointer
	var versionOK bool
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
			continue
		case line == "version "+LFSSpecVersion:
			versionOK = true
		case strings.HasPrefix(line, "oid "):
			p.OID = strings.TrimSpace(strings.TrimPrefix(line, "oid "))
		case strings.HasPrefix(line, "size "):
			size, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "size ")), 10, 64)
			if err != nil {
				return LFSPointer{}, err
			}
			p.Size = size
		}
	}
	if err := scanner.Err(); err != nil {
		return LFSPointer{}, err
	}
	if !versionOK || !strings.HasPrefix(p.OID, oidPrefixSHA256) || p.Size < 0 {
		return LFSPointer{}, errors.New("not a bdy nd lfs pointer")
	}
	return p, nil
}

func LFSFiles(r Repo) ([]IndexEntry, error) {
	idx, err := LoadIndex(r)
	if err != nil {
		return nil, err
	}
	var files []IndexEntry
	for _, entry := range sortedIndexEntries(idx) {
		if entry.Kind == KindLFS {
			files = append(files, entry)
		}
	}
	return files, nil
}

func (r Repo) AttributesPath() string {
	return filepath.Join(r.Dir, "attributes.json")
}
