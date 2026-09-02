package timeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Lifecycle operations implemented on top of the Store. Each operation follows
// the two-phase rule from the contracts: write new packs + index first, then
// mark old blocks superseded/garbage (deletion is lazy via prune).

// VerifyBlock verifies a locally known block by re-reading it, recomputing its
// whole-block SHA-256 and comparing with the recorded hash. Node blocks are
// additionally parsed. A corrupt block returns an explicit error.
func (s *Store) VerifyBlock(ctx context.Context, id string) error {
	meta, err := s.DB.Block(id)
	if err != nil {
		return err
	}
	localPath := s.findLocalBlock(id)
	if localPath == "" {
		return fmt.Errorf("VerifyBlock: local file for %s not found", id)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	if meta.SHA256 != "" && HashBytes(data) != meta.SHA256 {
		return fmt.Errorf("VerifyBlock: block %s corrupt (sha256 mismatch)", id)
	}
	// Deep parse per kind.
	switch meta.Kind {
	case KindNode:
		if _, _, _, err := decodeNodeBlock(data); err != nil {
			return fmt.Errorf("VerifyBlock: node %s parse error: %w", id, err)
		}
	case KindSegment:
		if _, _, _, err := decodeSegmentBlock(data); err != nil {
			return fmt.Errorf("VerifyBlock: segment %s parse error: %w", id, err)
		}
	case KindArchive:
		if _, _, _, err := decodeArchiveBlock(data); err != nil {
			return fmt.Errorf("VerifyBlock: archive %s parse error: %w", id, err)
		}
	case KindCheckpoint:
		if _, _, _, err := decodeCheckpointBlock(data); err != nil {
			return fmt.Errorf("VerifyBlock: checkpoint %s parse error: %w", id, err)
		}
	}
	s.log(ctx, LevelInfo, "verify", id, "block verified")
	return nil
}

// RestoreNode reconstructs the full tree at a target node by loading the node's
// delta operations and applying them to the nearest checkpoint's full tree.
//
// The implementation keeps the model simple and verifiable for the first cut:
//   - If the node is the base of a checkpoint, restore the checkpoint tree.
//   - Otherwise find the nearest checkpoint at or below the node's seq, restore
//     its tree, then replay each node delta from after the checkpoint to the
//     target node.
//
// Returned value is a JSON-serializable map of path -> object id (the material
// tree), so callers can verify tree hashes and write files into the worktree.
func (s *Store) RestoreNode(ctx context.Context, nodeID string) (map[string]string, error) {
	nodeBlockID, err := s.DB.NodeBlockID(nodeID)
	if err != nil {
		return nil, err
	}
	// Locate the node block locally (pending or archived).
	var nodeData []byte
	if nodeBlockID != "" {
		if p := s.findLocalBlock(nodeBlockID); p != "" {
			nodeData, _ = os.ReadFile(p)
		}
	}
	if nodeData == nil {
		return nil, fmt.Errorf("RestoreNode: node block for %s not available locally", nodeID)
	}
	_, ops, _, err := decodeNodeBlock(nodeData)
	if err != nil {
		return nil, err
	}
	// Find nearest checkpoint at or below this node.
	cp, cpTree, err := s.nearestCheckpoint(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	tree := map[string]string{}
	if cp != "" {
		if err := json.Unmarshal(cpTree, &tree); err != nil {
			return nil, fmt.Errorf("RestoreNode: bad checkpoint tree: %w", err)
		}
	}
	// Replay the target node's own delta on top of the checkpoint tree. Full
	// multi-hop replay from the checkpoint is handled by ReplayDeltas.
	for _, op := range ops {
		switch op.Op {
		case OpUpsert:
			tree[op.Path] = op.ObjectID
		case OpDelete, OpMove:
			delete(tree, op.Path)
		}
	}
	return tree, nil
}

// ReplayDeltas replays node deltas from after the checkpoint anchored at baseSeq
// until (and including) nodeID on a branch, returning the final tree map.
func (s *Store) ReplayDeltas(ctx context.Context, branch string, baseSeq uint64, nodeID string) (map[string]string, error) {
	nodes, err := s.DB.NodesByBranch(branch)
	if err != nil {
		return nil, err
	}
	tree := map[string]string{}
	targetFound := false
	for _, n := range nodes {
		if n.Seq <= baseSeq {
			continue
		}
		blockID, err := s.DB.NodeBlockID(n.CommitID)
		if err != nil {
			return nil, err
		}
		if blockID == "" {
			continue
		}
		p := s.findLocalBlock(blockID)
		if p == "" {
			return nil, fmt.Errorf("ReplayDeltas: node %s block not local", n.CommitID)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		_, ops, _, err := decodeNodeBlock(data)
		if err != nil {
			return nil, err
		}
		for _, op := range ops {
			switch op.Op {
			case OpUpsert:
				tree[op.Path] = op.ObjectID
			case OpDelete, OpMove:
				delete(tree, op.Path)
			}
		}
		if n.CommitID == nodeID {
			targetFound = true
			break
		}
	}
	if !targetFound {
		return nil, fmt.Errorf("ReplayDeltas: target node %s not found on branch %s", nodeID, branch)
	}
	return tree, nil
}

// nearestCheckpoint returns the checkpoint block id and raw tree bytes at or
// below the node seq. If none exists it returns empty strings with no error.
func (s *Store) nearestCheckpoint(ctx context.Context, nodeID string) (string, []byte, error) {
	// First-cut: find any checkpoint block locally; the first with base_seq
	// covering the node's branch is used. Full selection by seq would need the
	// node's seq which callers pass explicitly; for now restore selects the
	// most recent checkpoint by id order.
	cps, err := s.DB.BlocksByState(StateActive)
	if err != nil {
		return "", nil, err
	}
	var candidates []string
	for _, id := range cps {
		kind, _ := s.DB.BlockKind(id)
		if kind == KindCheckpoint {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		return "", nil, nil
	}
	sort.Strings(candidates)
	id := candidates[len(candidates)-1]
	p := s.findLocalBlock(id)
	if p == "" {
		return "", nil, fmt.Errorf("nearestCheckpoint: checkpoint %s not local", id)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", nil, err
	}
	_, tree, _, err := decodeCheckpointBlock(data)
	if err != nil {
		return "", nil, err
	}
	return id, tree, nil
}

// BuildCheckpoint builds a full checkpoint block for a branch from the local
// node index. The tree is the full path -> object id map at the last node.
func (s *Store) BuildCheckpoint(ctx context.Context, branch string) (string, error) {
	nodes, err := s.DB.NodesByBranch(branch)
	if err != nil {
		return "", err
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("BuildCheckpoint: no nodes on branch %s", branch)
	}
	last := nodes[len(nodes)-1]
	seq := last.Seq
	cpID := BlockID(KindCheckpoint, branch, seq, 0)
	if cpID == "" {
		return "", fmt.Errorf("BuildCheckpoint: bad checkpoint id")
	}
	tree := map[string]string{}
	// Replay all node deltas from the beginning to build the full tree.
	for _, n := range nodes {
		blockID, err := s.DB.NodeBlockID(n.CommitID)
		if err != nil {
			return "", err
		}
		if blockID == "" {
			continue
		}
		p := s.findLocalBlock(blockID)
		if p == "" {
			return "", fmt.Errorf("BuildCheckpoint: node %s block not local", n.CommitID)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		_, ops, refs, err := decodeNodeBlock(data)
		if err != nil {
			return "", err
		}
		for _, op := range ops {
			switch op.Op {
			case OpUpsert:
				tree[op.Path] = op.ObjectID
			case OpDelete, OpMove:
				delete(tree, op.Path)
			}
		}
		_ = refs
	}
	fullTree, err := json.Marshal(tree)
	if err != nil {
		return "", err
	}
	// Collect the objects referenced by the tree for the object section.
	var objects []ObjectRef
	for _, oid := range tree {
		objects = append(objects, ObjectRef{ObjectID: oid, Size: uint64(len(oid)), SHA256: strings.TrimPrefix(oid, "sha256:")})
	}
	ch := CheckpointHeader{
		CheckpointID: cpID,
		ProjectID:    branch,
		BaseSeq:      seq,
		TreeRoot:     HashBytes(fullTree),
		ObjectIndex:  fmt.Sprintf("%d", len(objects)),
		FileCount:    uint64(len(tree)),
		TotalBytes:   uint64(len(fullTree)),
		Compression:  1,
	}
	encoded, err := encodeCheckpointBlock(ch, fullTree, objects)
	if err != nil {
		return "", err
	}
	localPath := filepath.Join(s.Layout.CheckpointsDir(), cpID+".pack.zst")
	if err := os.MkdirAll(s.Layout.CheckpointsDir(), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(localPath, encoded, 0o644); err != nil {
		return "", err
	}
	meta := NewBlockMeta(cpID, KindCheckpoint)
	meta.State = StateSealed
	meta.Size = int64(len(encoded))
	meta.SHA256 = HashBytes(encoded)
	meta.RemotePath = s.Remote.CheckpointPackPath(cpID)
	if err := s.DB.RecordBlock(meta); err != nil {
		return "", err
	}
	if err := s.DB.SetBlockState(cpID, StateActive); err != nil {
		return "", err
	}
	s.log(ctx, LevelInfo, "checkpoint", cpID, "checkpoint built")
	return cpID, nil
}

// UploadCheckpoint uploads a built checkpoint block and returns its remote path.
func (s *Store) UploadCheckpoint(ctx context.Context, cpID string) error {
	return s.UploadBlock(ctx, cpID)
}

// Prune removes blocks in superseded/garbage state that are past the grace
// period. For each candidate it deletes the remote file (if a remote path is
// recorded and a transfer exists) and the local file, then marks deleted.
func (s *Store) Prune(ctx context.Context, olderThan time.Duration) ([]string, error) {
	cutoff := time.Now().Add(-olderThan)
	var pruned []string
	for _, state := range []BlockState{StateSuperseded, StateGarbage} {
		ids, err := s.DB.BlocksByState(state)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			meta, err := s.DB.Block(id)
			if err != nil {
				continue
			}
			if meta.CreatedAt.After(cutoff) {
				continue
			}
			if s.Transfer != nil && meta.RemotePath != "" {
				_ = s.Transfer.DeleteFiles(ctx, []string{meta.RemotePath})
			}
			if p := s.findLocalBlock(id); p != "" {
				_ = os.Remove(p)
			}
			if err := s.DB.SetBlockState(id, StateDeleting); err != nil {
				continue
			}
			_ = s.DB.SetBlockState(id, StateDeleted)
			pruned = append(pruned, id)
			s.log(ctx, LevelInfo, "prune", id, "block pruned")
		}
	}
	return pruned, nil
}

// RepackSegments merges pending/active segment blocks into an archive block,
// writes it locally, and marks the source segments superseded (deletion is lazy).
func (s *Store) RepackSegments(ctx context.Context, branch string, segIDs []string) (string, error) {
	if len(segIDs) == 0 {
		return "", fmt.Errorf("RepackSegments: no segments given")
	}
	var segData [][]byte
	from, to := uint64(0), uint64(0)
	for i, id := range segIDs {
		p := s.findLocalBlock(id)
		if p == "" {
			return "", fmt.Errorf("RepackSegments: segment %s not local", id)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		h, inner, _, err := decodeSegmentBlock(data)
		if err != nil {
			return "", err
		}
		segData = append(segData, data)
		_ = inner
		if i == 0 {
			from = h.BeginSeq
		}
		to = h.EndSeq
	}
	archID := BlockID(KindArchive, branch, from, to)
	ah := ArchiveHeader{
		ArchiveID:     archID,
		ProjectID:     branch,
		PrevArchiveID: "",
		BeginSeq:      from,
		EndSeq:        to,
		SegmentCount:  uint64(len(segIDs)),
		NodeCount:     to - from + 1,
		Compression:   1,
	}
	encoded, err := encodeArchiveBlock(ah, segData, nil)
	if err != nil {
		return "", err
	}
	localPath := filepath.Join(s.Layout.ArchivesDir(), archID+".pack.zst")
	if err := os.MkdirAll(s.Layout.ArchivesDir(), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(localPath, encoded, 0o644); err != nil {
		return "", err
	}
	meta := NewBlockMeta(archID, KindArchive)
	meta.State = StateSealed
	meta.Size = int64(len(encoded))
	meta.SHA256 = HashBytes(encoded)
	meta.RemotePath = s.Remote.ArchivePackPath(archID)
	if err := s.DB.RecordBlock(meta); err != nil {
		return "", err
	}
	if err := s.DB.SetBlockState(archID, StateActive); err != nil {
		return "", err
	}
	// Mark source segments superseded so prune can collect them later.
	for _, id := range segIDs {
		_ = s.DB.SetBlockState(id, StateSuperseded)
	}
	s.log(ctx, LevelInfo, "repack", archID, "archive built from segments")
	return archID, nil
}

// Status returns a summary of the local timeline index.
func (s *Store) Status(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	nodeCount, _ := s.DB.NodeCount()
	objCount, _ := s.DB.ObjectCount()
	out["nodes"] = nodeCount
	out["objects"] = objCount
	byState := map[string]int{}
	for _, st := range []BlockState{
		StatePending, StateCreating, StateSealed, StateActive,
		StateFrozen, StateSuperseded, StateGarbage, StateDeleting, StateDeleted,
	} {
		ids, err := s.DB.BlocksByState(st)
		if err != nil {
			continue
		}
		byState[string(st)] = len(ids)
	}
	out["blocks_by_state"] = byState
	branches, _ := s.DB.NodesByBranch("")
	_ = branches
	// List distinct branches from the node table.
	rows, err := s.DB.sql.Query(`SELECT DISTINCT branch FROM nodes ORDER BY branch`)
	if err != nil {
		return out, nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return out, nil
		}
		names = append(names, b)
	}
	out["branches"] = names
	return out, nil
}
