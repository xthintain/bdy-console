package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"baiduyunStorage/internal/lfs"
	"baiduyunStorage/internal/repo"
)

func cmdLFS(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy lfs install|track|untrack|status|ls-files|push|fetch|pull|checkout|clean|smudge")
	}
	switch args[0] {
	case "install":
		root, err := lfs.GitRoot(".")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(root, repo.DirName, "lfs", "objects"), 0o755); err != nil {
			return err
		}
		exe, _ := os.Executable()
		if err := lfs.InstallGitFilters(root, exe); err != nil {
			return err
		}
		fmt.Fprintln(out, "bdy lfs filters installed")
		return nil
	case "track":
		root, err := lfs.GitRoot(".")
		if err != nil {
			return err
		}
		if err := lfs.Track(root, args[1:]); err != nil {
			return err
		}
		fmt.Fprintf(out, "tracking %d pattern(s)\n", len(args)-1)
		return nil
	case "untrack":
		root, err := lfs.GitRoot(".")
		if err != nil {
			return err
		}
		if err := lfs.Untrack(root, args[1:]); err != nil {
			return err
		}
		fmt.Fprintf(out, "untracked %d pattern(s)\n", len(args)-1)
		return nil
	case "clean":
		return lfsClean(args[1:], out)
	case "smudge":
		return lfsSmudge(args[1:], out)
	case "ls-files":
		return lfsLsFiles(out)
	case "status":
		return lfsStatus(out)
	case "push":
		return lfsPush(ctx, out)
	case "fetch":
		return lfsFetch(ctx, out)
	case "checkout":
		return lfsCheckout(out)
	case "pull":
		if err := lfsFetch(ctx, out); err != nil {
			return err
		}
		return lfsCheckout(out)
	default:
		return errors.New("usage: bdy lfs install|track|untrack|status|ls-files|push|fetch|pull|checkout|clean|smudge")
	}
}
