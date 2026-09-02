package timeline

import (
	"context"
	"os"
	"path/filepath"

	"baiduyunStorage/internal/baidu"
)

// BaiduTransfer adapts the Baidu Netdisk client to the RemoteTransfer
// interface used by the timeline layer. It lives in the timeline package so
// the Store can be wired to a real Baidu client without reaching into cli.
type BaiduTransfer struct {
	Client baidu.Client
}

// UploadFile uploads a local file to a remote netdisk path.
func (t BaiduTransfer) UploadFile(ctx context.Context, localPath, remotePath string) error {
	return t.Client.UploadFile(ctx, localPath, remotePath)
}

// DownloadFile downloads a remote netdisk file to a local path. It resolves
// the file by listing its parent and fetching its dlink, then writing bytes.
func (t BaiduTransfer) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	parent := filepath.ToSlash(filepath.Dir(remotePath))
	name := filepath.Base(remotePath)
	items, err := t.Client.List(ctx, parent)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Path != remotePath && item.ServerFilename != name {
			continue
		}
		meta, err := t.Client.FileMetas(ctx, []uint64{item.FSID}, true)
		if err != nil {
			return err
		}
		if len(meta) == 0 || meta[0].DLink == "" {
			return os.ErrNotExist
		}
		return t.Client.Download(ctx, meta[0].DLink, localPath)
	}
	return os.ErrNotExist
}

// Exists reports whether a remote path exists.
func (t BaiduTransfer) Exists(ctx context.Context, remotePath string) (bool, error) {
	parent := filepath.ToSlash(filepath.Dir(remotePath))
	name := filepath.Base(remotePath)
	items, err := t.Client.List(ctx, parent)
	if err != nil {
		return false, nil
	}
	for _, item := range items {
		if item.Path == remotePath || item.ServerFilename == name {
			return true, nil
		}
	}
	return false, nil
}

// ListFiles lists all file paths under a remote root.
func (t BaiduTransfer) ListFiles(ctx context.Context, remoteRoot string) ([]string, error) {
	items, err := t.Client.ListAll(ctx, remoteRoot)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, item := range items {
		if item.IsDir == 0 {
			paths = append(paths, item.Path)
		}
	}
	return paths, nil
}

// DeleteFiles deletes remote paths through the file manager API.
func (t BaiduTransfer) DeleteFiles(ctx context.Context, remotePaths []string) error {
	if len(remotePaths) == 0 {
		return nil
	}
	return t.Client.FileManager(ctx, "delete", remotePaths)
}
