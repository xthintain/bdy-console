package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/repo"
)

func cmdInit(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	remoteRoot := fs.String("remote-root", repo.DefaultRemoteRoot, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validateIsolatedRemoteRoot(*remoteRoot); err != nil {
		return err
	}
	r, err := repo.Init(".", *remoteRoot)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "initialized bdy repo in %s\n", r.Dir)
	return nil
}

func cmdSync(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy sync init|status|add|commit|push|pull|ls|rm|mv|remote")
	}
	switch args[0] {
	case "init":
		return cmdInit(args[1:], out)
	case "status":
		return cmdStatus(out)
	case "add":
		return cmdAdd(args[1:], out)
	case "commit":
		return cmdCommit(args[1:], out)
	case "push":
		return cmdPush(ctx, out)
	case "pull":
		return cmdPull(ctx, out)
	case "ls":
		return cmdLS(ctx, args[1:], out)
	case "rm":
		return cmdRemove(args[1:], out)
	case "mv":
		return cmdMove(args[1:], out)
	case "remote":
		return cmdRemote(ctx, out)
	default:
		return errors.New("usage: bdy sync init|status|add|commit|push|pull|ls|rm|mv|remote")
	}
}

func cmdHome(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy home ls|find|grep|search|rm|delete|cat|download|get|mkdir|touch|vim [args...]")
	}
	if args[0] == "cmd" {
		args = args[1:]
		if len(args) == 0 {
			return runCloudFileREPL(ctx, cmdInput, out, cloudFileSpace{
				Name:      "home",
				Root:      "/",
				Resolve:   homePath,
				AllowCD:   false,
				UseLongLS: true,
			})
		}
	}
	return runCloudFileCommand(ctx, args, out, cloudFileSpace{
		Name:      "home",
		Root:      "/",
		Resolve:   homePath,
		AllowCD:   false,
		UseLongLS: true,
	})
}

func cmdStatus(out io.Writer) error {
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	base, err := repo.LoadManifest(r.ManifestPath())
	if err != nil {
		return err
	}
	current, err := repo.Scan(r.Root)
	if err != nil {
		return err
	}
	st := repo.Diff(base, current)
	printStatus(out, st)
	return nil
}

func cmdAdd(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy add <path...>")
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	idx, err := repo.LoadIndex(r.IndexPath())
	if err != nil {
		return err
	}
	for _, p := range args {
		idx.Add(p)
	}
	if err := repo.SaveIndex(r.IndexPath(), idx); err != nil {
		return err
	}
	fmt.Fprintf(out, "staged %d path(s)\n", len(args))
	return nil
}

func cmdRemove(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy rm <path...>")
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	idx, err := repo.LoadIndex(r.IndexPath())
	if err != nil {
		return err
	}
	for _, p := range args {
		idx.Remove(p)
		_ = os.RemoveAll(filepath.Join(r.Root, repo.CleanPath(p)))
	}
	if err := repo.SaveIndex(r.IndexPath(), idx); err != nil {
		return err
	}
	fmt.Fprintf(out, "removed %d path(s)\n", len(args))
	return nil
}

func cmdMove(args []string, out io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: bdy mv <old> <new>")
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	oldPath := filepath.Join(r.Root, repo.CleanPath(args[0]))
	newPath := filepath.Join(r.Root, repo.CleanPath(args[1]))
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	idx, err := repo.LoadIndex(r.IndexPath())
	if err != nil {
		return err
	}
	idx.Move(args[0], args[1])
	if err := repo.SaveIndex(r.IndexPath(), idx); err != nil {
		return err
	}
	fmt.Fprintf(out, "moved %s -> %s\n", args[0], args[1])
	return nil
}

func cmdCommit(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("commit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	msg := fs.String("m", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *msg == "" {
		return errors.New("usage: bdy commit -m <message>")
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	idx, err := repo.LoadIndex(r.IndexPath())
	if err != nil {
		return err
	}
	if len(idx.Added) == 0 && len(idx.Removed) == 0 && len(idx.Moved) == 0 {
		return errors.New("nothing staged")
	}
	current, err := repo.Scan(r.Root)
	if err != nil {
		return err
	}
	base, err := repo.LoadManifest(r.ManifestPath())
	if err != nil {
		return err
	}
	next := repo.ApplyIndex(base, current, idx)
	c, err := repo.CreateCommit(r, *msg, next)
	if err != nil {
		return err
	}
	if err := repo.SaveManifest(r.ManifestPath(), next); err != nil {
		return err
	}
	if err := repo.SaveIndex(r.IndexPath(), repo.Index{}); err != nil {
		return err
	}
	fmt.Fprintf(out, "[%s] %s\n", c.ID, c.Message)
	return nil
}

func cmdLS(ctx context.Context, args []string, out io.Writer) error {
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return err
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	if err := validateIsolatedRemoteRoot(r.Config.RemoteRoot); err != nil {
		return err
	}
	dir := r.Config.RemoteRoot
	if len(args) > 0 {
		dir = remoteJoin(r.Config.RemoteRoot, args[0])
	}
	items, err := baidu.NewClient(cfg).List(ctx, dir)
	if err != nil {
		return err
	}
	printRemoteEntries(out, items)
	return nil
}

func cmdPush(ctx context.Context, out io.Writer) error {
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return err
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	if err := validateIsolatedRemoteRoot(r.Config.RemoteRoot); err != nil {
		return err
	}
	local, err := repo.LoadManifest(r.ManifestPath())
	if err != nil {
		return err
	}
	client := baidu.NewClient(cfg)
	_ = client.Mkdir(ctx, r.Config.RemoteRoot)
	_ = client.Mkdir(ctx, remoteJoin(r.Config.RemoteRoot, ".bdy"))
	_ = client.Mkdir(ctx, remoteJoin(r.Config.RemoteRoot, ".bdy/commits"))
	remote, _ := loadRemoteManifest(ctx, client, r)
	diff := repo.Diff(remote, local)
	for _, p := range append(diff.Added, diff.Modified...) {
		entry := local.Map()[p]
		if entry.IsDir {
			_ = client.Mkdir(ctx, remoteJoin(r.Config.RemoteRoot, p))
			continue
		}
		if err := client.UploadFile(ctx, filepath.Join(r.Root, p), remoteJoin(r.Config.RemoteRoot, p)); err != nil {
			return fmt.Errorf("upload %s: %w", p, err)
		}
		fmt.Fprintf(out, "uploaded %s\n", p)
	}
	for _, p := range diff.Deleted {
		_ = client.FileManager(ctx, "delete", []string{remoteJoin(r.Config.RemoteRoot, p)})
		fmt.Fprintf(out, "deleted remote %s\n", p)
	}
	if err := client.UploadFile(ctx, r.ManifestPath(), remoteJoin(r.Config.RemoteRoot, ".bdy/manifest.json")); err != nil {
		return err
	}
	for _, id := range commitFiles(r) {
		localPath := filepath.Join(r.CommitsDir(), id)
		if err := client.UploadFile(ctx, localPath, remoteJoin(r.Config.RemoteRoot, ".bdy/commits/"+id)); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "push complete")
	return nil
}

func cmdPull(ctx context.Context, out io.Writer) error {
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return err
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	if err := validateIsolatedRemoteRoot(r.Config.RemoteRoot); err != nil {
		return err
	}
	base, err := repo.LoadManifest(r.ManifestPath())
	if err != nil {
		return err
	}
	local, err := repo.Scan(r.Root)
	if err != nil {
		return err
	}
	client := baidu.NewClient(cfg)
	remote, err := loadRemoteManifest(ctx, client, r)
	if err != nil {
		return err
	}
	conflicts := repo.Conflicts(base, local, remote)
	if len(conflicts) > 0 {
		return fmt.Errorf("conflicts: %s", strings.Join(conflicts, ", "))
	}
	allRemote, err := client.ListAll(ctx, r.Config.RemoteRoot)
	if err != nil {
		return err
	}
	fsids := map[string]uint64{}
	for _, item := range allRemote {
		rel := strings.TrimPrefix(item.Path, strings.TrimRight(r.Config.RemoteRoot, "/")+"/")
		if strings.HasPrefix(rel, ".bdy/") {
			continue
		}
		fsids[rel] = item.FSID
	}
	diff := repo.Diff(base, remote)
	remoteMap := remote.Map()
	for _, p := range append(diff.Added, diff.Modified...) {
		entry := remoteMap[p]
		dest := filepath.Join(r.Root, p)
		if entry.IsDir {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			continue
		}
		meta, err := client.FileMetas(ctx, []uint64{fsids[p]}, true)
		if err != nil {
			return err
		}
		if len(meta) == 0 || meta[0].DLink == "" {
			return fmt.Errorf("missing dlink for %s", p)
		}
		if err := client.Download(ctx, meta[0].DLink, dest); err != nil {
			return err
		}
		fmt.Fprintf(out, "downloaded %s\n", p)
	}
	for _, p := range diff.Deleted {
		_ = os.RemoveAll(filepath.Join(r.Root, p))
		fmt.Fprintf(out, "deleted local %s\n", p)
	}
	if err := repo.SaveManifest(r.ManifestPath(), remote); err != nil {
		return err
	}
	fmt.Fprintln(out, "pull complete")
	return nil
}

func cmdRemote(ctx context.Context, out io.Writer) error {
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "remote root: %s\n", r.Config.RemoteRoot)
	return nil
}

func printStatus(out io.Writer, st repo.Status) {
	for _, p := range st.Added {
		fmt.Fprintf(out, "A  %s\n", p)
	}
	for _, p := range st.Modified {
		fmt.Fprintf(out, "M  %s\n", p)
	}
	for _, p := range st.Deleted {
		fmt.Fprintf(out, "D  %s\n", p)
	}
	if len(st.Added)+len(st.Modified)+len(st.Deleted) == 0 {
		fmt.Fprintln(out, "clean")
	}
}

func printRemoteEntries(out io.Writer, items []baidu.FileEntry) {
	for _, item := range items {
		kind := "file"
		if item.IsDir == 1 {
			kind = "dir "
		}
		fmt.Fprintf(out, "%s %10d %s\n", kind, item.Size, item.Path)
	}
}

func validateIsolatedRemoteRoot(root string) error {
	root = strings.TrimRight(root, "/")
	if root == "" || root == "/" {
		return errors.New("ordinary bdy commands cannot target /; use bdy home ls [path] for whole-netdisk access")
	}
	appRoot := strings.TrimRight(repo.AppRoot, "/")
	if root == appRoot || strings.HasPrefix(root, appRoot+"/") {
		return nil
	}
	return fmt.Errorf("ordinary bdy commands are isolated under %s; use bdy home ls [path] for whole-netdisk access", repo.AppRoot)
}

func homePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "." || p == "/" {
		return "/"
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(p, "/"))
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func loadRemoteManifest(ctx context.Context, client baidu.Client, r repo.Repo) (repo.Manifest, error) {
	items, err := client.List(ctx, remoteJoin(r.Config.RemoteRoot, ".bdy"))
	if err != nil {
		return repo.Manifest{}, err
	}
	for _, item := range items {
		if filepath.Base(item.Path) != "manifest.json" {
			continue
		}
		meta, err := client.FileMetas(ctx, []uint64{item.FSID}, true)
		if err != nil {
			return repo.Manifest{}, err
		}
		if len(meta) == 0 || meta[0].DLink == "" {
			return repo.Manifest{}, errors.New("remote manifest has no dlink")
		}
		tmp, err := os.CreateTemp("", "bdy-remote-manifest-*")
		if err != nil {
			return repo.Manifest{}, err
		}
		tmpPath := tmp.Name()
		_ = tmp.Close()
		defer os.Remove(tmpPath)
		if err := client.Download(ctx, meta[0].DLink, tmpPath); err != nil {
			return repo.Manifest{}, err
		}
		return repo.LoadManifest(tmpPath)
	}
	return repo.Manifest{}, errors.New("remote manifest not found")
}

func remoteJoin(root, p string) string {
	p = repo.CleanPath(p)
	if p == "" {
		return root
	}
	return strings.TrimRight(root, "/") + "/" + p
}

func commitFiles(r repo.Repo) []string {
	items, err := os.ReadDir(r.CommitsDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, item := range items {
		if !item.IsDir() && strings.HasSuffix(item.Name(), ".json") {
			out = append(out, item.Name())
		}
	}
	return out
}
