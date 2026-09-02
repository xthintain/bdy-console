package timeline

// Header types for the four block kinds. These mirror the field sets defined
// in the block format contract (docs/timeline-agents/block-format.md).

type NodeBlockHeader struct {
	NodeID       string
	ParentNodeID string
	ProjectID    string
	Author       string
	TimestampMs  uint64
	Seq          uint64
	Compression  byte
}

type SegmentHeader struct {
	SegmentID   string
	ArchiveID   string
	ProjectID   string
	BeginSeq    uint64
	EndSeq      uint64
	NodeCount   uint64
	Compression byte
}

type ArchiveHeader struct {
	ArchiveID     string
	ProjectID     string
	PrevArchiveID string
	BeginSeq      uint64
	EndSeq        uint64
	SegmentCount  uint64
	NodeCount     uint64
	Compression   byte
}

type CheckpointHeader struct {
	CheckpointID string
	ProjectID    string
	BaseSeq      uint64
	TreeRoot     string
	ObjectIndex  string
	FileCount    uint64
	TotalBytes   uint64
	TreeLen      uint64
	Compression  byte
}

// OffEntry is one index entry locating a sub-block inside a parent block.
type OffEntry struct {
	Seq    uint64
	Offset uint64
	Length uint64
	SHA256 string
}

// ObjectRef references a content-addressed object (whole file or chunk).
type ObjectRef struct {
	ObjectID     string
	Size         uint64
	SHA256       string
	RemoteOffset uint64
	RemoteLength uint64
}

// NodeMeta is the commit-node metadata handed to the timeline layer by the
// bdynd main package (see TimelineSink in the integration contract).
type NodeMeta struct {
	CommitID    string
	ParentID    string
	Branch      string
	Seq         uint64
	Author      string
	TimestampMs uint64
	Message     string
}

// DeltaOp is one change operation in a node delta.
type DeltaOp struct {
	Op       byte // 1=upsert 2=delete 3=move 4=attr
	Path     string
	ObjectID string // for upsert
}

const (
	OpUpsert byte = 1
	OpDelete byte = 2
	OpMove   byte = 3
	OpAttr   byte = 4
)
