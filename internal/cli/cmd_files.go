package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/repo"
)

func cmdCat(ctx context.Context, client baidu.Client, out io.Writer, remotePath string, number bool) error {
	entry, err := findRemoteEntry(ctx, client, remotePath)
	if err != nil {
		return err
	}
	if entry.IsDir == 1 {
		return fmt.Errorf("%s is a directory", remotePath)
	}
	meta, err := client.FileMetas(ctx, []uint64{entry.FSID}, true)
	if err != nil {
		return err
	}
	if len(meta) == 0 || meta[0].DLink == "" {
		return fmt.Errorf("missing dlink for %s", remotePath)
	}
	tmp, err := os.CreateTemp("", "bdy-cmd-cat-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if err := client.Download(ctx, meta[0].DLink, tmpPath); err != nil {
		return err
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if number {
		return writeNumbered(out, f)
	}
	_, err = io.Copy(out, f)
	return err
}

func cmdDownload(ctx context.Context, client baidu.Client, out io.Writer, remotePath, localPath string) error {
	entry, err := findRemoteEntry(ctx, client, remotePath)
	if err != nil {
		return err
	}
	if entry.IsDir == 1 {
		return fmt.Errorf("%s is a directory", remotePath)
	}
	meta, err := client.FileMetas(ctx, []uint64{entry.FSID}, true)
	if err != nil {
		return err
	}
	if len(meta) == 0 || meta[0].DLink == "" {
		return fmt.Errorf("missing dlink for %s", remotePath)
	}
	if os.Getenv(quietEnv) == "1" {
		return client.DownloadWithProgress(ctx, meta[0].DLink, localPath, nil)
	}
	return client.DownloadWithProgress(ctx, meta[0].DLink, localPath, out)
}

func downloadDestination(remotePath, localPath string) (string, error) {
	localPath = strings.TrimSpace(localPath)
	if localPath == "" {
		localPath = "."
	}
	if info, err := os.Stat(localPath); err == nil && info.IsDir() {
		return filepath.Join(localPath, filepath.Base(remotePath)), nil
	}
	if strings.HasSuffix(localPath, string(os.PathSeparator)) || localPath == "." {
		return filepath.Join(localPath, filepath.Base(remotePath)), nil
	}
	return localPath, nil
}

func writeNumbered(out io.Writer, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(data), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		fmt.Fprintf(out, "%6d\t%s", i+1, line)
	}
	return nil
}

func cmdTouch(ctx context.Context, client baidu.Client, root, remotePath string) error {
	if err := ensureRemoteDir(ctx, client, root, filepath.ToSlash(filepath.Dir(remotePath))); err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "bdy-cmd-touch-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)
	return client.UploadFile(ctx, tmpPath, remotePath)
}

func cmdVim(ctx context.Context, client baidu.Client, out io.Writer, root, remotePath string) error {
	if err := ensureRemoteDir(ctx, client, root, filepath.ToSlash(filepath.Dir(remotePath))); err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "bdy-cmd-vim-"+filepath.Base(remotePath)+"-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)

	if entry, err := findRemoteEntry(ctx, client, remotePath); err == nil && entry.IsDir == 0 {
		meta, err := client.FileMetas(ctx, []uint64{entry.FSID}, true)
		if err != nil {
			return err
		}
		if len(meta) == 0 || meta[0].DLink == "" {
			return fmt.Errorf("missing dlink for %s", remotePath)
		}
		if err := client.Download(ctx, meta[0].DLink, tmpPath); err != nil {
			return err
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if err := client.UploadFile(ctx, tmpPath, remotePath); err != nil {
		return err
	}
	fmt.Fprintf(out, "saved %s\n", remotePath)
	return nil
}

func ensureRemoteDir(ctx context.Context, client baidu.Client, root, remotePath string) error {
	remotePath = strings.TrimRight(remotePath, "/")
	if remotePath == "" || remotePath == "." || remotePath == "/" {
		return nil
	}
	root = strings.TrimRight(root, "/")
	if root == "" {
		root = "/"
	}
	if root != "/" && remotePath != root && !strings.HasPrefix(remotePath, root+"/") {
		return fmt.Errorf("path must be under %s", root)
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(remotePath, root), "/")
	current := root
	if current != "/" {
		_ = client.Mkdir(ctx, current)
	}
	if rel == "" {
		return nil
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" {
			continue
		}
		if current == "/" {
			current = "/" + part
		} else {
			current = strings.TrimRight(current, "/") + "/" + part
		}
		_ = client.Mkdir(ctx, current)
	}
	return nil
}

func findRemoteEntry(ctx context.Context, client baidu.Client, remotePath string) (baidu.FileEntry, error) {
	parent := filepath.ToSlash(filepath.Dir(remotePath))
	if parent == "." {
		parent = repo.CmdRoot
	}
	name := filepath.Base(remotePath)
	items, err := client.List(ctx, parent)
	if err != nil {
		return baidu.FileEntry{}, err
	}
	for _, item := range items {
		if item.Path == remotePath || item.ServerFilename == name {
			return item, nil
		}
	}
	return baidu.FileEntry{}, fmt.Errorf("not found: %s", remotePath)
}
