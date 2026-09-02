package timeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RemoteTransfer is the narrow remote interface the timeline layer depends on.
type RemoteTransfer interface {
	UploadFile(ctx context.Context, localPath, remotePath string) error
	DownloadFile(ctx context.Context, remotePath, localPath string) error
	Exists(ctx context.Context, remotePath string) (bool, error)
	ListFiles(ctx context.Context, remoteRoot string) ([]string, error)
	DeleteFiles(ctx context.Context, remotePaths []string) error
}

// Store coordinates the local index, the local block store, and the remote
// transfer layer for one timeline project.
type Store struct {
	DB       *DB
	Layout   Layout
	Remote   RemoteLayout
	Transfer RemoteTransfer
	Cfg      Config
	Log      Logger
}

// NewStore creates a Store, opening (or creating) the local index database.
func NewStore(dbPath string, layout Layout, remote RemoteLayout, transfer RemoteTransfer, cfg Config) (*Store, error) {
	db, err := OpenDB(dbPath)
	if err != nil {
		return nil, err
	}
	cfg = cfg.WithDefaults()
	return &Store{
		DB:       db,
		Layout:   layout,
		Remote:   remote,
		Transfer: transfer,
		Cfg:      cfg,
		Log:      NopLogger{},
	}, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

// Init creates all local timeline directories.
func (s *Store) Init() error {
	for _, dir := range s.Layout.Dirs() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// AppendNode records a commit node into the local pending index.
func (s *Store) AppendNode(ctx context.Context, n NodeMeta) error {
	blockID := "node-" + n.CommitID
	if err := s.DB.RecordNode(n, blockID); err != nil {
		return err
	}
	meta := NewBlockMeta(blockID, KindNode)
	meta.State = StatePending
	meta.CreatedAt = time.UnixMilli(int64(n.TimestampMs))
	return s.DB.RecordBlock(meta)
}

// FlushNode writes a single node as a standalone NodeBlock file in the local
// pending directory.
func (s *Store) FlushNode(ctx context.Context, n NodeMeta, ops []DeltaOp, refs []ObjectRef) (string, error) {
	h := NodeBlockHeader{
		NodeID:       n.CommitID,
		ParentNodeID: n.ParentID,
		ProjectID:    n.Branch,
		Author:       n.Author,
		TimestampMs:  n.TimestampMs,
		Seq:          n.Seq,
	}
	encoded, err := encodeNodeBlockForTest(h, ops, refs)
	if err != nil {
		return "", err
	}
	blockID := "node-" + n.CommitID
	localPath := filepath.Join(s.Layout.PendingDir(), blockID+".block")
	if err := os.MkdirAll(s.Layout.PendingDir(), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(localPath, encoded, 0o644); err != nil {
		return "", err
	}
	meta := NewBlockMeta(blockID, KindNode)
	meta.State = StateActive
	meta.Size = int64(len(encoded))
	meta.SHA256 = HashBytes(encoded)
	meta.RemotePath = s.Remote.ArchivesDir() + "/" + blockID + ".block"
	meta.CreatedAt = time.UnixMilli(int64(n.TimestampMs))
	if err := s.DB.RecordBlock(meta); err != nil {
		return "", err
	}
	s.log(ctx, LevelInfo, "flush", blockID, "node flushed")
	return blockID, nil
}

// BuildSegment merges nodes into a SegmentBlock, writes it locally, and
// uploads it to the remote archive area.
func (s *Store) BuildSegment(ctx context.Context, project string, nodes []NodeMeta, blocks [][]byte) (string, error) {
	if len(nodes) == 0 || len(nodes) != len(blocks) {
		return "", fmt.Errorf("BuildSegment: node/block count mismatch")
	}
	from := nodes[0].Seq
	to := nodes[len(nodes)-1].Seq
	segID := BlockID(KindSegment, project, from, to)
	if segID == "" {
		return "", fmt.Errorf("BuildSegment: bad segment id")
	}
	sh := SegmentHeader{
		SegmentID:   segID,
		ProjectID:   project,
		BeginSeq:    from,
		EndSeq:      to,
		NodeCount:   uint64(len(nodes)),
		Compression: 1,
	}
	compressed := make([][]byte, 0, len(blocks))
	for _, b := range blocks {
		cb, _, err := compressIfNeeded(b, 1)
		if err != nil {
			return "", err
		}
		compressed = append(compressed, cb)
	}
	encoded, err := encodeSegmentBlock(sh, compressed)
	if err != nil {
		return "", err
	}
	localPath := filepath.Join(s.Layout.ArchivesDir(), segID+".pack.zst")
	if err := os.MkdirAll(s.Layout.ArchivesDir(), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(localPath, encoded, 0o644); err != nil {
		return "", err
	}
	meta := NewBlockMeta(segID, KindSegment)
	meta.State = StateSealed
	meta.Size = int64(len(encoded))
	meta.SHA256 = HashBytes(encoded)
	meta.RemotePath = s.Remote.ArchivePackPath(segID)
	if err := s.DB.RecordBlock(meta); err != nil {
		return "", err
	}
	for _, n := range nodes {
		if err := s.DB.RecordNode(n, segID); err != nil {
			return "", err
		}
	}
	s.log(ctx, LevelInfo, "repack", segID, "segment built")
	return segID, nil
}

// UploadBlock uploads a locally built block to its recorded remote path.
func (s *Store) UploadBlock(ctx context.Context, id string) error {
	state, err := s.DB.BlockState(id)
	if err != nil {
		return err
	}
	if state == "" {
		return fmt.Errorf("UploadBlock: unknown block %s", id)
	}
	localPath := s.findLocalBlock(id)
	if localPath == "" {
		return fmt.Errorf("UploadBlock: local file for %s not found", id)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	expected, err := s.blockSHA(id)
	if err != nil || expected == "" {
		expected = HashBytes(data)
	}
	if HashBytes(data) != expected {
		return fmt.Errorf("UploadBlock: %s local hash mismatch before upload", id)
	}
	remotePath := s.remotePathFor(id, state)
	if remotePath == "" {
		return fmt.Errorf("UploadBlock: no remote path for %s", id)
	}
	if s.Transfer == nil {
		return fmt.Errorf("UploadBlock: no remote transfer configured")
	}
	if err := s.Transfer.UploadFile(ctx, localPath, remotePath); err != nil {
		return err
	}
	if err := s.DB.SetBlockState(id, StateActive); err != nil {
		return err
	}
	s.log(ctx, LevelInfo, "upload", id, "uploaded "+remotePath)
	return nil
}

// DownloadBlock downloads a block from the remote to a local path and verifies
// its SHA-256 against the recorded hash. On mismatch the temporary file is
// removed and an error is returned (caller may retry).
func (s *Store) DownloadBlock(ctx context.Context, id string) (string, error) {
	state, err := s.DB.BlockState(id)
	if err != nil {
		return "", err
	}
	if state == "" {
		return "", fmt.Errorf("DownloadBlock: unknown block %s", id)
	}
	remotePath := s.remotePathFor(id, state)
	if remotePath == "" {
		return "", fmt.Errorf("DownloadBlock: no remote path for %s", id)
	}
	localPath := filepath.Join(s.Layout.CacheDir(), id+".pack")
	if err := os.MkdirAll(s.Layout.CacheDir(), 0o755); err != nil {
		return "", err
	}
	tmp := localPath + ".bdy-download"
	if err := s.Transfer.DownloadFile(ctx, remotePath, tmp); err != nil {
		return "", err
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		return "", err
	}
	expected, err := s.blockSHA(id)
	if err != nil || expected == "" {
		expected = HashBytes(data)
	}
	if HashBytes(data) != expected {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("DownloadBlock: %s hash mismatch; corrupt file removed", id)
	}
	if err := os.Rename(tmp, localPath); err != nil {
		return "", err
	}
	return localPath, nil
}

func (s *Store) findLocalBlock(id string) string {
	candidates := []string{
		filepath.Join(s.Layout.ArchivesDir(), id+".pack.zst"),
		filepath.Join(s.Layout.PendingDir(), id+".block"),
		filepath.Join(s.Layout.CacheDir(), id+".pack"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (s *Store) blockSHA(id string) (string, error) {
	var sha string
	err := s.DB.sql.QueryRow(`SELECT sha256 FROM blocks WHERE id=?`, id).Scan(&sha)
	if err != nil {
		return "", err
	}
	return sha, nil
}

func (s *Store) remotePathFor(id string, state BlockState) string {
	switch {
	case strings.HasPrefix(id, "archive-"):
		return s.Remote.ArchivePackPath(id)
	case strings.HasPrefix(id, "segment-"):
		return s.Remote.ArchivePackPath(id)
	case strings.HasPrefix(id, "checkpoint-"):
		return s.Remote.CheckpointPackPath(id)
	case strings.HasPrefix(id, "node-"):
		return s.Remote.ArchivesDir() + "/" + id + ".block"
	}
	return ""
}

// UpdateRef points the remote branch ref file at a node, uploading it last.
func (s *Store) UpdateRef(ctx context.Context, branch, nodeID string) error {
	if err := s.DB.UpdateRef(branch, nodeID, time.Now().UnixMilli()); err != nil {
		return err
	}
	if s.Transfer == nil {
		return nil
	}
	refJSON := fmt.Sprintf("{\"node_id\":%q,\"updated_at_ms\":%d}\n", nodeID, time.Now().UnixMilli())
	localPath := filepath.Join(s.Layout.RefsDir(), branch+".json")
	if err := os.MkdirAll(s.Layout.RefsDir(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(localPath, []byte(refJSON), 0o644); err != nil {
		return err
	}
	remotePath := s.Remote.RefPath(branch)
	if err := s.Transfer.UploadFile(ctx, localPath, remotePath); err != nil {
		return err
	}
	s.log(ctx, LevelInfo, "ref", branch, "ref updated")
	return nil
}

// ListNodesByBranch returns the nodes of a branch ordered by seq.
func (s *Store) ListNodesByBranch(branch string) ([]NodeMeta, error) {
	return s.DB.NodesByBranch(branch)
}

// LastNodeSeq returns the highest node seq recorded locally for a branch.
func (s *Store) LastNodeSeq(branch string) (uint64, error) {
	return s.DB.LastNodeSeq(branch)
}

func (s *Store) log(ctx context.Context, level LogLevel, op, blockID, msg string) {
	if s.Log != nil {
		s.Log.Log(ctx, LogEvent{Level: level, Op: op, BlockID: blockID, Message: msg})
	}
}
