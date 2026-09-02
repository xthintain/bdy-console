package timeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// BlockMeta is the data-layer metadata for one timeline block. The byte-level
// layout of the block is defined by the block format contract, not here.
type BlockMeta struct {
	ID         string
	Kind       Kind
	State      BlockState
	Size       int64
	SHA256     string
	RemotePath string
	CreatedAt  time.Time
}

func NewBlockMeta(id string, kind Kind) BlockMeta {
	return BlockMeta{ID: id, Kind: kind, State: StatePending, CreatedAt: time.Now().UTC()}
}

// BlockID builds a stable human-readable block id for archive/segment/checkpoint
// blocks. Node ids are content-addressed by the commit hash instead.
func BlockID(kind Kind, project string, from, to uint64) string {
	switch kind {
	case KindArchive:
		return fmt.Sprintf("archive-%s-%06d-%06d", project, from, to)
	case KindSegment:
		return fmt.Sprintf("segment-%s-%06d-%06d", project, from, to)
	case KindCheckpoint:
		return fmt.Sprintf("checkpoint-%s-%06d", project, from)
	default:
		return ""
	}
}

// HashBytes returns the lowercase hex SHA-256 of data, used as the strong
// block/object content identifier.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
