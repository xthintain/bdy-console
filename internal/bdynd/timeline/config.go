package timeline

import "time"

const (
	DefaultSegmentSize        = 5
	DefaultArchiveSize        = 100
	DefaultCheckpointInterval = 100
	DefaultLargeFileThreshold = 4 * 1024 * 1024
	DefaultChunkSize          = 4 * 1024 * 1024
	DefaultCompression        = "zstd"
)

var DefaultGCGracePeriod = 7 * 24 * time.Hour

type Kind string

const (
	KindNode       Kind = "node"
	KindSegment    Kind = "segment"
	KindArchive    Kind = "archive"
	KindCheckpoint Kind = "checkpoint"
)

type Config struct {
	SegmentSize        int           `json:"segment_size"`
	ArchiveSize        int           `json:"archive_size"`
	CheckpointInterval int           `json:"checkpoint_interval"`
	LargeFileThreshold int64         `json:"large_file_threshold"`
	ChunkSize          int64         `json:"chunk_size"`
	Compression        string        `json:"compression"`
	GCGracePeriod      time.Duration `json:"gc_grace_period"`
}

func DefaultConfig() Config {
	return Config{
		SegmentSize:        DefaultSegmentSize,
		ArchiveSize:        DefaultArchiveSize,
		CheckpointInterval: DefaultCheckpointInterval,
		LargeFileThreshold: DefaultLargeFileThreshold,
		ChunkSize:          DefaultChunkSize,
		Compression:        DefaultCompression,
		GCGracePeriod:      DefaultGCGracePeriod,
	}
}

func (c Config) WithDefaults() Config {
	if c.SegmentSize <= 0 {
		c.SegmentSize = DefaultSegmentSize
	}
	if c.ArchiveSize <= 0 {
		c.ArchiveSize = DefaultArchiveSize
	}
	if c.CheckpointInterval <= 0 {
		c.CheckpointInterval = DefaultCheckpointInterval
	}
	if c.LargeFileThreshold <= 0 {
		c.LargeFileThreshold = DefaultLargeFileThreshold
	}
	if c.ChunkSize <= 0 {
		c.ChunkSize = DefaultChunkSize
	}
	if c.Compression == "" {
		c.Compression = DefaultCompression
	}
	if c.GCGracePeriod <= 0 {
		c.GCGracePeriod = DefaultGCGracePeriod
	}
	return c
}
