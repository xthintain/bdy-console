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
		if err := downloadRemoteFile(ctx, remote, RemotePackManifestPath(remoteRoot, id), filepath.Join(packDir(r), id+".json")); err != nil {
			return err
		}
		if err := downloadRemoteFile(ctx, remote, RemotePackPath(remoteRoot, id), filepath.Join(packDir(r), id+".pack")); err != nil {
			return err
		}
	}
	return nil
}
