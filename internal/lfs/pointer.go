package lfs

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	SpecVersion = "https://bdy-lfs/spec/v1"
)

type Pointer struct {
	OID  string
	Size int64
}

func HashFile(path string) (Pointer, error) {
	f, err := os.Open(path)
	if err != nil {
		return Pointer{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return Pointer{}, err
	}
	return Pointer{OID: "sha256:" + hex.EncodeToString(h.Sum(nil)), Size: n}, nil
}

func ParsePointer(r io.Reader) (Pointer, error) {
	scanner := bufio.NewScanner(r)
	var p Pointer
	var versionOK bool
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
			continue
		case line == "version "+SpecVersion:
			versionOK = true
		case strings.HasPrefix(line, "oid "):
			p.OID = strings.TrimSpace(strings.TrimPrefix(line, "oid "))
		case strings.HasPrefix(line, "size "):
			size, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "size ")), 10, 64)
			if err != nil {
				return Pointer{}, err
			}
			p.Size = size
		}
	}
	if err := scanner.Err(); err != nil {
		return Pointer{}, err
	}
	if !versionOK || !strings.HasPrefix(p.OID, "sha256:") || p.Size < 0 {
		return Pointer{}, errors.New("not a bdy lfs pointer")
	}
	return p, nil
}

func FormatPointer(p Pointer) string {
	return fmt.Sprintf("version %s\noid %s\nsize %d\n", SpecVersion, p.OID, p.Size)
}

func IsPointerBytes(data []byte) bool {
	_, err := ParsePointer(strings.NewReader(string(data)))
	return err == nil
}

func SHA(p Pointer) string {
	return strings.TrimPrefix(p.OID, "sha256:")
}
