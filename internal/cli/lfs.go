package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/lfs"
)

func lfsClean(args []string, out io.Writer) error {
	root, err := lfs.GitRoot(".")
	if err != nil {
		return err
	}
	path := filterPath(args)
	if path == "" {
		return errors.New("clean requires a path from git filter")
	}
	p, err := lfs.HashFile(path)
	if err != nil {
		return err
	}
	if err := lfs.StoreObject(root, path, p); err != nil {
		return err
	}
	_, err = io.WriteString(out, lfs.FormatPointer(p))
	return err
}

func lfsSmudge(args []string, out io.Writer) error {
	root, err := lfs.GitRoot(".")
	if err != nil {
		return err
	}
	path := filterPath(args)
	if path == "" {
		return errors.New("smudge requires a path from git filter")
	}
	p, err := lfs.ReadPointerFile(path)
	if err != nil {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return err
		}
		_, writeErr := out.Write(data)
		return writeErr
	}
	objectPath := lfs.ObjectPath(root, p)
	f, err := os.Open(objectPath)
	if err != nil {
		_, writeErr := io.WriteString(out, lfs.FormatPointer(p))
		return writeErr
	}
	defer f.Close()
	_, err = io.Copy(out, f)
	return err
}

func lfsLsFiles(out io.Writer) error {
	root, err := lfs.GitRoot(".")
	if err != nil {
		return err
	}
	files, err := lfs.TrackedPointers(root)
	if err != nil {
		return err
	}
	for _, file := range files {
		fmt.Fprintf(out, "%s %s %d\n", file.Pointer.OID, file.Path, file.Pointer.Size)
	}
	return nil
}

func lfsStatus(out io.Writer) error {
	root, err := lfs.GitRoot(".")
	if err != nil {
		return err
	}
	files, err := lfs.TrackedPointers(root)
	if err != nil {
		return err
	}
	missing := 0
	for _, file := range files {
		if _, err := os.Stat(lfs.ObjectPath(root, file.Pointer)); err != nil {
			missing++
			fmt.Fprintf(out, "missing %s %s\n", file.Pointer.OID, file.Path)
		}
	}
	fmt.Fprintf(out, "tracked: %d, missing: %d\n", len(files), missing)
	return nil
}

func lfsPush(ctx context.Context, out io.Writer) error {
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return err
	}
	root, err := lfs.GitRoot(".")
	if err != nil {
		return err
	}
	client := baidu.NewClient(cfg)
	ensureLFSRemoteDirs(ctx, client)
	files, err := lfs.TrackedPointers(root)
	if err != nil {
		return err
	}
	for _, file := range files {
		p := file.Pointer
		objectPath := lfs.ObjectPath(root, p)
		if _, err := os.Stat(objectPath); err != nil {
			return fmt.Errorf("object missing for %s; run bdy lfs clean/checkout or re-add file", file.Path)
		}
		remotePath := lfs.RemoteObjectPath(p)
		if err := client.UploadFile(ctx, objectPath, remotePath); err != nil {
			return fmt.Errorf("upload %s: %w", p.OID, err)
		}
		meta := lfs.ObjectMeta{OID: p.OID, Size: p.Size, CreatedAt: time.Now().UTC(), RemotePath: remotePath}
		tmp, err := os.CreateTemp("", "bdy-lfs-meta-*.json")
		if err != nil {
			return err
		}
		data, _ := json.MarshalIndent(meta, "", "  ")
		if _, err := tmp.Write(append(data, '\n')); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return err
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(tmp.Name())
			return err
		}
		if err := client.UploadFile(ctx, tmp.Name(), lfs.RemoteMetaPath(p)); err != nil {
			_ = os.Remove(tmp.Name())
			return err
		}
		_ = os.Remove(tmp.Name())
		fmt.Fprintf(out, "uploaded %s %s\n", p.OID, file.Path)
	}
	return nil
}

func lfsFetch(ctx context.Context, out io.Writer) error {
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return err
	}
	root, err := lfs.GitRoot(".")
	if err != nil {
		return err
	}
	client := baidu.NewClient(cfg)
	files, err := lfs.TrackedPointers(root)
	if err != nil {
		return err
	}
	all, err := client.ListAll(ctx, lfs.RemoteRoot)
	if err != nil {
		return err
	}
	byPath := map[string]baidu.FileEntry{}
	for _, item := range all {
		byPath[item.Path] = item
	}
	for _, file := range files {
		p := file.Pointer
		objectPath := lfs.ObjectPath(root, p)
		if _, err := os.Stat(objectPath); err == nil {
			continue
		}
		entry, ok := byPath[lfs.RemoteObjectPath(p)]
		if !ok {
			return fmt.Errorf("remote object missing for %s", p.OID)
		}
		meta, err := client.FileMetas(ctx, []uint64{entry.FSID}, true)
		if err != nil {
			return err
		}
		if len(meta) == 0 || meta[0].DLink == "" {
			return fmt.Errorf("missing dlink for %s", p.OID)
		}
		tmp := filepath.Join(lfs.TempDir(root), lfs.SHA(p)+".part")
		if err := client.Download(ctx, meta[0].DLink, tmp); err != nil {
			return err
		}
		if err := lfs.StoreObject(root, tmp, p); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		_ = os.Remove(tmp)
		fmt.Fprintf(out, "fetched %s %s\n", p.OID, file.Path)
	}
	return nil
}

func lfsCheckout(out io.Writer) error {
	root, err := lfs.GitRoot(".")
	if err != nil {
		return err
	}
	files, err := lfs.PointerFiles(root)
	if err != nil {
		return err
	}
	for _, file := range files {
		path := filepath.Join(root, file)
		p, err := lfs.ReadPointerFile(path)
		if err != nil {
			return err
		}
		objectPath := lfs.ObjectPath(root, p)
		if _, err := os.Stat(objectPath); err != nil {
			return fmt.Errorf("cached object missing for %s; run bdy lfs fetch", file)
		}
		if err := copyFileAtomic(objectPath, path); err != nil {
			return err
		}
		fmt.Fprintf(out, "checked out %s\n", file)
	}
	return nil
}

func ensureLFSRemoteDirs(ctx context.Context, client baidu.Client) {
	_ = client.Mkdir(ctx, lfs.RemoteRoot)
	_ = client.Mkdir(ctx, lfs.RemoteRoot+"/objects")
	_ = client.Mkdir(ctx, lfs.RemoteRoot+"/objects/sha256")
	_ = client.Mkdir(ctx, lfs.RemoteRoot+"/meta")
}

func filterPath(args []string) string {
	if len(args) == 0 {
		return ""
	}
	if args[0] == "--" && len(args) > 1 {
		return args[1]
	}
	return args[len(args)-1]
}

func copyFileAtomic(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dest + ".bdy-lfs"
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
	return os.Rename(tmp, dest)
}
