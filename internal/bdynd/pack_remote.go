package bdynd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func RemotePackPath(remoteRoot, id string) string {
	return strings.TrimRight(remoteRoot, "/") + "/packs/" + id + ".pack"
}

func RemotePackManifestPath(remoteRoot, id string) string {
	return strings.TrimRight(remoteRoot, "/") + "/packs/" + id + ".json"
}

func PushPacks(ctx context.Context, r Repo, remote RemoteStore, remoteRoot string) error {
	packs, err := ListPacks(r)
	if err != nil {
		return err
	}
	for _, pack := range packs {
		if !ValidPackID(pack.ID) {
			return fmt.Errorf("invalid pack id %q", pack.ID)
		}
		packPath := filepath.Join(packDir(r), pack.ID+".pack")
		manifestPath := filepath.Join(packDir(r), pack.ID+".json")
		if _, err := os.Stat(packPath); err != nil {
			return fmt.Errorf("missing pack %s: %w", pack.ID, err)
		}
		if err := uploadLocalFile(ctx, remote, packPath, RemotePackPath(remoteRoot, pack.ID)); err != nil {
			return err
		}
		if err := uploadLocalFile(ctx, remote, manifestPath, RemotePackManifestPath(remoteRoot, pack.ID)); err != nil {
			return err
		}
	}
	return nil
}

func FetchPacks(ctx context.Context, r Repo, remote RemoteStore, remoteRoot string, ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("fetch pack requires at least one id")
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !ValidPackID(id) {
			return fmt.Errorf("invalid pack id %q", id)
		}
		if err := downloadRemoteFile(ctx, remote, RemotePackManifestPath(remoteRoot, id), filepath.Join(packDir(r), id+".json")); err != nil {
			return err
		}
		if err := downloadRemoteFile(ctx, remote, RemotePackPath(remoteRoot, id), filepath.Join(packDir(r), id+".pack")); err != nil {
			return err
		}
	}
	return nil
}

func ValidPackID(id string) bool {
	if len(id) != len("20060102150405-0123456789ab") || id[14] != '-' {
		return false
	}
	for i, r := range id {
		if i == 14 {
			continue
		}
		if i < 14 {
			if r < '0' || r > '9' {
				return false
			}
			continue
		}
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
