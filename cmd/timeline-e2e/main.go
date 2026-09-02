// Command timeline-e2e exercises the layered timeline Store against the real
// Baidu Netdisk using the currently configured token. It is a manual smoke
// test: it uploads a segment pack and a ref under a test-only remote root, then
// cleans them up.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/bdynd/timeline"
)

const testRemoteRoot = "/apps/baiduyunStorage/timeline-e2e"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return err
	}
	client := baidu.NewClient(cfg)

	root := os.TempDir()
	layout := timeline.NewLayout(filepath.Join(root, ".bdynd-timeline-e2e", "timeline"))
	remote := timeline.NewRemoteLayout(testRemoteRoot)
	transfer := timeline.BaiduTransfer{Client: client}

	store, err := timeline.NewStore(
		filepath.Join(root, ".bdynd-timeline-e2e", "index.sqlite"),
		layout, remote, transfer, timeline.DefaultConfig(),
	)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		return err
	}

	// Build two nodes and a segment containing both.
	n1 := timeline.NodeMeta{CommitID: "sha256:e2e-c1", Branch: "main", Seq: 1, Message: "first", TimestampMs: 1756800000000}
	n2 := timeline.NodeMeta{CommitID: "sha256:e2e-c2", ParentID: "sha256:e2e-c1", Branch: "main", Seq: 2, Message: "second", TimestampMs: 1756800001000}
	nb1, err := timeline.EncodeNodeBlockForTest(timeline.NodeBlockHeader{NodeID: n1.CommitID, ProjectID: "main", Seq: 1},
		[]timeline.DeltaOp{{Op: timeline.OpUpsert, Path: "readme.txt", ObjectID: "sha256:o-readme"}}, nil)
	if err != nil {
		return err
	}
	nb2, err := timeline.EncodeNodeBlockForTest(timeline.NodeBlockHeader{NodeID: n2.CommitID, ParentNodeID: n1.CommitID, ProjectID: "main", Seq: 2},
		[]timeline.DeltaOp{{Op: timeline.OpDelete, Path: "readme.txt"}}, nil)
	if err != nil {
		return err
	}

	if err := store.AppendNode(ctx, n1); err != nil {
		return err
	}
	if err := store.AppendNode(ctx, n2); err != nil {
		return err
	}

	segID, err := store.BuildSegment(ctx, "main", []timeline.NodeMeta{n1, n2}, [][]byte{nb1, nb2})
	if err != nil {
		return err
	}
	fmt.Printf("built segment %s\n", segID)

	if err := store.UploadBlock(ctx, segID); err != nil {
		return err
	}
	fmt.Printf("uploaded segment to %s\n", store.Remote.ArchivePackPath(segID))

	// Update the remote ref last.
	if err := store.UpdateRef(ctx, "main", n2.CommitID); err != nil {
		return err
	}
	fmt.Printf("updated remote ref %s\n", store.Remote.RefPath("main"))

	// Verify remote presence.
	ok, err := transfer.Exists(ctx, store.Remote.ArchivePackPath(segID))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("segment not present remotely")
	}
	fmt.Println("segment present remotely")

	// Cleanup: delete the test remote root files.
	paths := []string{store.Remote.ArchivePackPath(segID), store.Remote.RefPath("main")}
	if err := transfer.DeleteFiles(ctx, paths); err != nil {
		return fmt.Errorf("cleanup delete: %w", err)
	}
	fmt.Println("cleanup done")
	return nil
}
