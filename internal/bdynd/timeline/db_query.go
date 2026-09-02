package timeline

import (
	"database/sql"
	"fmt"
	"time"
)

// This file adds the query surface the lifecycle operations (verify / restore /
// repack / checkpoint / prune) need on top of the core DB statements defined in
// db.go. It deliberately does not change the schema: every query here works
// against the four core tables (blocks / nodes / refs / objects).

// BlocksByState returns block ids of a given lifecycle state, ordered by id.
func (db *DB) BlocksByState(state BlockState) ([]string, error) {
	rows, err := db.sql.Query(`SELECT id FROM blocks WHERE state=? ORDER BY id`, string(state))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Block returns the full metadata row for a block id.
func (db *DB) Block(id string) (BlockMeta, error) {
	var b BlockMeta
	var kind, state string
	var createdMs int64
	err := db.sql.QueryRow(
		`SELECT id, kind, state, size, sha256, remote_path, created_at FROM blocks WHERE id=?`, id,
	).Scan(&b.ID, &kind, &state, &b.Size, &b.SHA256, &b.RemotePath, &createdMs)
	if err == nil {
		b.CreatedAt = time.UnixMilli(createdMs)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			return BlockMeta{}, fmt.Errorf("block %s not found", id)
		}
		return BlockMeta{}, err
	}
	b.Kind = Kind(kind)
	b.State = BlockState(state)
	return b, nil
}

// BlockKind returns the kind of a block, or "" if unknown.
func (db *DB) BlockKind(id string) (Kind, error) {
	var kind string
	err := db.sql.QueryRow(`SELECT kind FROM blocks WHERE id=?`, id).Scan(&kind)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return Kind(kind), nil
}

// ObjectsForBlock returns all object rows referencing the given block.
func (db *DB) ObjectsForBlock(blockID string) ([]ObjectRef, error) {
	rows, err := db.sql.Query(
		`SELECT oid, size, sha256 FROM objects WHERE block_id=?`, blockID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ObjectRef
	for rows.Next() {
		var r ObjectRef
		if err := rows.Scan(&r.ObjectID, &r.Size, &r.SHA256); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// NodeBlockID returns the block id recorded for a node, or "" if none.
func (db *DB) NodeBlockID(nodeID string) (string, error) {
	var blockID string
	err := db.sql.QueryRow(`SELECT block_id FROM nodes WHERE node_id=?`, nodeID).Scan(&blockID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return blockID, nil
}

// Ref returns the node id a named ref points at.
func (db *DB) Ref(name string) (string, error) {
	var nodeID string
	err := db.sql.QueryRow(`SELECT node_id FROM refs WHERE name=?`, name).Scan(&nodeID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return nodeID, nil
}

// ObjectCount returns how many distinct objects are indexed locally.
func (db *DB) ObjectCount() (int, error) {
	var n int
	err := db.sql.QueryRow(`SELECT COUNT(*) FROM objects`).Scan(&n)
	return n, err
}

// NodeCount returns how many nodes are indexed locally.
func (db *DB) NodeCount() (int, error) {
	var n int
	err := db.sql.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&n)
	return n, err
}
