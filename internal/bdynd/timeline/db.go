package timeline

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the local SQLite index for the timeline layer. Prepared statements
// are cached on the struct so repeated node/block writes avoid re-parsing SQL.
type DB struct {
	sql  *sql.DB
	path string

	stmtRecordBlock   *sql.Stmt
	stmtSetBlockState *sql.Stmt
	stmtBlockState    *sql.Stmt
	stmtRecordNode    *sql.Stmt
	stmtUpdateRef     *sql.Stmt
	stmtRecordObject  *sql.Stmt
	stmtNodesByBranch *sql.Stmt
}

// OpenDB opens (or creates) the local timeline index database at path.
func OpenDB(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// A few busy connections speed up the low-volume index workload.
	handle.SetMaxOpenConns(1)
	db := &DB{sql: handle, path: path}
	if err := db.migrate(); err != nil {
		handle.Close()
		return nil, err
	}
	if err := db.prepare(); err != nil {
		handle.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.sql.Close()
}

func (db *DB) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS blocks (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			state TEXT NOT NULL,
			size INTEGER NOT NULL DEFAULT 0,
			sha256 TEXT NOT NULL DEFAULT '',
			remote_path TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			node_id TEXT PRIMARY KEY,
			parent_id TEXT NOT NULL DEFAULT '',
			branch TEXT NOT NULL DEFAULT '',
			seq INTEGER NOT NULL DEFAULT 0,
			block_id TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL DEFAULT '',
			author TEXT NOT NULL DEFAULT '',
			timestamp_ms INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS refs (
			name TEXT PRIMARY KEY,
			node_id TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS objects (
			oid TEXT PRIMARY KEY,
			size INTEGER NOT NULL DEFAULT 0,
			sha256 TEXT NOT NULL DEFAULT '',
			block_id TEXT NOT NULL DEFAULT '',
			ref_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_branch_seq ON nodes(branch, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_blocks_state ON blocks(state)`,
		`CREATE INDEX IF NOT EXISTS idx_objects_block ON objects(block_id)`,
	}
	for _, stmt := range statements {
		if _, err := db.sql.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func (db *DB) prepare() error {
	var err error
	db.stmtRecordBlock, err = db.sql.Prepare(
		`INSERT INTO blocks (id, kind, state, size, sha256, remote_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   kind=excluded.kind, state=excluded.state, size=excluded.size,
		   sha256=excluded.sha256, remote_path=excluded.remote_path`)
	if err != nil {
		return fmt.Errorf("prepare RecordBlock: %w", err)
	}
	db.stmtSetBlockState, err = db.sql.Prepare(`UPDATE blocks SET state=? WHERE id=?`)
	if err != nil {
		return fmt.Errorf("prepare SetBlockState: %w", err)
	}
	db.stmtBlockState, err = db.sql.Prepare(`SELECT state FROM blocks WHERE id=?`)
	if err != nil {
		return fmt.Errorf("prepare BlockState: %w", err)
	}
	db.stmtRecordNode, err = db.sql.Prepare(
		`INSERT INTO nodes (node_id, parent_id, branch, seq, block_id, message, author, timestamp_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(node_id) DO UPDATE SET
		   parent_id=excluded.parent_id, branch=excluded.branch, seq=excluded.seq,
		   block_id=excluded.block_id, message=excluded.message, author=excluded.author,
		   timestamp_ms=excluded.timestamp_ms`)
	if err != nil {
		return fmt.Errorf("prepare RecordNode: %w", err)
	}
	db.stmtUpdateRef, err = db.sql.Prepare(
		`INSERT INTO refs (name, node_id, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET node_id=excluded.node_id, updated_at=excluded.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare UpdateRef: %w", err)
	}
	db.stmtRecordObject, err = db.sql.Prepare(
		`INSERT INTO objects (oid, size, sha256, block_id, ref_count)
		 VALUES (?, ?, ?, ?, 1)
		 ON CONFLICT(oid) DO UPDATE SET
		   size=excluded.size, sha256=excluded.sha256, block_id=excluded.block_id,
		   ref_count=ref_count+1`)
	if err != nil {
		return fmt.Errorf("prepare RecordObject: %w", err)
	}
	db.stmtNodesByBranch, err = db.sql.Prepare(
		`SELECT node_id, parent_id, branch, seq, message, author, timestamp_ms
		 FROM nodes WHERE branch=? ORDER BY seq`)
	if err != nil {
		return fmt.Errorf("prepare NodesByBranch: %w", err)
	}
	return nil
}

// RecordBlock upserts a block row.
func (db *DB) RecordBlock(b BlockMeta) error {
	_, err := db.stmtRecordBlock.Exec(
		b.ID, string(b.Kind), string(b.State), b.Size, b.SHA256, b.RemotePath, b.CreatedAt.UnixMilli(),
	)
	return err
}

// SetBlockState transitions a block's lifecycle state.
func (db *DB) SetBlockState(id string, state BlockState) error {
	_, err := db.stmtSetBlockState.Exec(string(state), id)
	return err
}

// BlockState returns the stored state of a block.
func (db *DB) BlockState(id string) (BlockState, error) {
	var state string
	err := db.stmtBlockState.QueryRow(id).Scan(&state)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return BlockState(state), nil
}

// RecordNode upserts a commit node row.
func (db *DB) RecordNode(n NodeMeta, blockID string) error {
	_, err := db.stmtRecordNode.Exec(
		n.CommitID, n.ParentID, n.Branch, n.Seq, blockID, n.Message, n.Author, n.TimestampMs,
	)
	return err
}

// UpdateRef points a named ref at a node.
func (db *DB) UpdateRef(name, nodeID string, nowMs int64) error {
	_, err := db.stmtUpdateRef.Exec(name, nodeID, nowMs)
	return err
}

// RecordObject upserts a content object row.
func (db *DB) RecordObject(o ObjectRef, blockID string) error {
	_, err := db.stmtRecordObject.Exec(o.ObjectID, o.Size, o.SHA256, blockID)
	return err
}

// NodesByBranch returns the nodes of a branch ordered by seq.
func (db *DB) NodesByBranch(branch string) ([]NodeMeta, error) {
	rows, err := db.stmtNodesByBranch.Query(branch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NodeMeta
	for rows.Next() {
		var n NodeMeta
		if err := rows.Scan(&n.CommitID, &n.ParentID, &n.Branch, &n.Seq, &n.Message, &n.Author, &n.TimestampMs); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// LastNodeSeq returns the maximum seq recorded for a branch.
func (db *DB) LastNodeSeq(branch string) (uint64, error) {
	var seq uint64
	err := db.sql.QueryRow(`SELECT COALESCE(MAX(seq), 0) FROM nodes WHERE branch=?`, branch).Scan(&seq)
	if err != nil {
		return 0, err
	}
	return seq, nil
}

// Ping reports database health.
func (db *DB) Ping() error {
	return db.sql.Ping()
}

var _ = time.Now
