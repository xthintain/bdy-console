package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/bdynd"
	"baiduyunStorage/internal/bdynd/timeline"
)

// ndTimelineOpen opens the bdynd repo and constructs the timeline Store wired
// to the Baidu remote. The local layout lives under <repo>.bdynd/timeline and
// the remote layout reuses the configured remote root.
func ndTimelineOpen(ctx context.Context) (*timeline.Store, error) {
	r, err := bdynd.Open(".")
	if err != nil {
		return nil, err
	}
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return nil, err
	}
	remoteRoot := r.Config.Remotes[bdynd.DefaultRemote]
	if remoteRoot == "" {
		remoteRoot = "/apps/baiduyunStorage/nd/repos/" + filepath.Base(r.Root)
	}
	layout := timeline.NewLayout(filepath.Join(r.Dir, "timeline"))
	remote := timeline.NewRemoteLayout(remoteRoot)
	transfer := timeline.BaiduTransfer{Client: baidu.NewClient(cfg)}
	store, err := timeline.NewStore(
		filepath.Join(r.Dir, "timeline", "index.sqlite"),
		layout, remote, transfer, timeline.DefaultConfig(),
	)
	if err != nil {
		return nil, err
	}
	return store, nil
}

func cmdNDTimeline(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy nd timeline init|status|verify|flush|repack|compact|split|checkpoint|prune|restore")
	}
	switch args[0] {
	case "init":
		store, err := ndTimelineOpen(ctx)
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.Init(); err != nil {
			return err
		}
		fmt.Fprintln(out, "timeline initialized")
		return nil
	case "status":
		store, err := ndTimelineOpen(ctx)
		if err != nil {
			return err
		}
		defer store.Close()
		st, err := store.Status(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "nodes: %v\n", st["nodes"])
		fmt.Fprintf(out, "objects: %v\n", st["objects"])
		if bs, ok := st["blocks_by_state"].(map[string]int); ok {
			for _, state := range []string{"pending", "sealed", "active", "superseded", "garbage", "deleting", "deleted"} {
				if n := bs[state]; n > 0 {
					fmt.Fprintf(out, "blocks %s: %d\n", state, n)
				}
			}
		}
		if branches, ok := st["branches"].([]string); ok {
			fmt.Fprintf(out, "branches: %s\n", strings.Join(branches, ", "))
		}
		return nil
	case "verify":
		if len(args) < 2 {
			return errors.New("usage: bdy nd timeline verify <block-id>")
		}
		store, err := ndTimelineOpen(ctx)
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.VerifyBlock(ctx, args[1]); err != nil {
			return err
		}
		fmt.Fprintf(out, "verified %s\n", args[1])
		return nil
	case "flush":
		// First cut: flush reports pending nodes that need explicit segment
		// building. Store.FlushNode is called per commit via the sink; here we
		// surface how many pending blocks exist.
		store, err := ndTimelineOpen(ctx)
		if err != nil {
			return err
		}
		defer store.Close()
		pending, err := store.DB.BlocksByState(timeline.StatePending)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "pending blocks: %d\n", len(pending))
		for _, id := range pending {
			fmt.Fprintf(out, "  %s\n", id)
		}
		return nil
	case "repack":
		fs := flag.NewFlagSet("timeline repack", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		branch := fs.String("branch", "main", "branch to repack")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		store, err := ndTimelineOpen(ctx)
		if err != nil {
			return err
		}
		defer store.Close()
		active, err := store.DB.BlocksByState(timeline.StateActive)
		if err != nil {
			return err
		}
		var segs []string
		for _, id := range active {
			kind, _ := store.DB.BlockKind(id)
			if kind == timeline.KindSegment {
				segs = append(segs, id)
			}
		}
		if len(segs) == 0 {
			fmt.Fprintln(out, "no active segments to repack")
			return nil
		}
		archID, err := store.RepackSegments(ctx, *branch, segs)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "repacked %d segments into %s\n", len(segs), archID)
		return nil
	case "checkpoint":
		fs := flag.NewFlagSet("timeline checkpoint", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		branch := fs.String("branch", "main", "branch to checkpoint")
		upload := fs.Bool("upload", false, "upload checkpoint after building")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		store, err := ndTimelineOpen(ctx)
		if err != nil {
			return err
		}
		defer store.Close()
		cpID, err := store.BuildCheckpoint(ctx, *branch)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "built checkpoint %s\n", cpID)
		if *upload {
			if err := store.UploadCheckpoint(ctx, cpID); err != nil {
				return err
			}
			fmt.Fprintf(out, "uploaded checkpoint %s\n", cpID)
		}
		return nil
	case "restore":
		if len(args) != 2 {
			return errors.New("usage: bdy nd timeline restore <commit>")
		}
		store, err := ndTimelineOpen(ctx)
		if err != nil {
			return err
		}
		defer store.Close()
		tree, err := store.RestoreNode(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "restored tree for %s (%d entries)\n", args[1], len(tree))
		for path := range tree {
			fmt.Fprintf(out, "  %s -> %s\n", path, tree[path])
		}
		return nil
	case "prune":
		fs := flag.NewFlagSet("timeline prune", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		older := fs.String("older-than", "7d", "only prune blocks older than this duration")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		d, err := time.ParseDuration(*older)
		if err != nil {
			return fmt.Errorf("bad --older-than %q: %w", *older, err)
		}
		store, err := ndTimelineOpen(ctx)
		if err != nil {
			return err
		}
		defer store.Close()
		pruned, err := store.Prune(ctx, d)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "pruned %d block(s)\n", len(pruned))
		for _, id := range pruned {
			fmt.Fprintf(out, "  %s\n", id)
		}
		return nil
	case "compact", "split":
		return errors.New("timeline compact/split are not implemented yet")
	default:
		return errors.New("usage: bdy nd timeline init|status|verify|flush|repack|compact|split|checkpoint|prune|restore")
	}
}

// ndCommitTimeline records a commit node into the timeline store: it appends
// the node metadata and flushes the node delta so the block is available for
// segment/checkpoint building. It is called automatically after each bdy nd
// commit. If the timeline store cannot be opened (e.g. no remote configured),
// the error is returned but commit still succeeded.
func ndCommitTimeline(ctx context.Context, r bdynd.Repo, c bdynd.CommitObject) error {
	// Build the timeline store; failure to init the local dirs is tolerated by
	// falling back to the default remote root.
	remoteRoot := r.Config.Remotes[bdynd.DefaultRemote]
	if remoteRoot == "" {
		remoteRoot = "/apps/baiduyunStorage/nd/repos/" + filepath.Base(r.Root)
	}
	layout := timeline.NewLayout(filepath.Join(r.Dir, "timeline"))
	remote := timeline.NewRemoteLayout(remoteRoot)
	transfer := timeline.BaiduTransfer{}
	store, err := timeline.NewStore(
		filepath.Join(r.Dir, "timeline", "index.sqlite"),
		layout, remote, transfer, timeline.DefaultConfig(),
	)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		return err
	}
	parent := ""
	if c.Parent != "" {
		parent = c.Parent
	}
	seq, err := store.LastNodeSeq(branchForCommit(r, c))
	if err != nil {
		return err
	}
	n := timeline.NodeMeta{
		CommitID:    c.OID,
		ParentID:    parent,
		Branch:      branchForCommit(r, c),
		Seq:         seq + 1,
		TimestampMs: uint64(c.Time.UnixMilli()),
		Message:     c.Message,
	}
	if err := store.AppendNode(ctx, n); err != nil {
		return err
	}
	// Flush the node delta: upsert all entries in this commit.
	ops := make([]timeline.DeltaOp, 0, len(c.Entries))
	for _, e := range c.Entries {
		if e.Kind == bdynd.KindBlob {
			ops = append(ops, timeline.DeltaOp{Op: timeline.OpUpsert, Path: e.Path, ObjectID: e.OID})
		}
	}
	if _, err := store.FlushNode(ctx, n, ops, nil); err != nil {
		return err
	}
	return nil
}

// branchForCommit returns the default branch name for a commit's timeline sink.
func branchForCommit(r bdynd.Repo, c bdynd.CommitObject) string {
	branch := r.Config.DefaultBranch
	if branch == "" {
		branch = bdynd.DefaultBranch
	}
	return branch
}

var _ = os.Stdout
