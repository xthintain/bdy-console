package timeline

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// benchPayload is a realistic small source file payload.
func benchPayload(n int) []byte {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "line %d: package main; func main(){ println(\"hello world %d\"); }\n", i, i)
	}
	return b.Bytes()
}

func TestBenchEncodeDecodeThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	payload := benchPayload(50)
	t.Logf("payload size: %d bytes", len(payload))

	const nodes = 5
	nodeBlocks := make([][]byte, 0, nodes)
	for i := 0; i < nodes; i++ {
		h := NodeBlockHeader{NodeID: fmt.Sprintf("sha256:c%d", i), ProjectID: "main", Seq: uint64(i + 1)}
		if i == 0 {
			h.ParentNodeID = ""
		} else {
			h.ParentNodeID = fmt.Sprintf("sha256:c%d", i-1)
		}
		ops := []DeltaOp{{Op: OpUpsert, Path: fmt.Sprintf("src/file%d.go", i), ObjectID: fmt.Sprintf("sha256:o%d", i)}}
		nb, err := encodeNodeBlockForTest(h, ops, nil)
		if err != nil {
			t.Fatal(err)
		}
		nodeBlocks = append(nodeBlocks, nb)
	}

	sh := SegmentHeader{SegmentID: "segment-bench", ProjectID: "main", BeginSeq: 1, EndSeq: uint64(nodes), NodeCount: nodes, Compression: 1}
	start := time.Now()
	seg, err := encodeSegmentBlock(sh, nodeBlocks)
	encElapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("segment encode: %s (raw=%d bytes compressed-to=%d bytes)", encElapsed, len(nodeBlocks[0])*nodes, len(seg))

	start = time.Now()
	_, gotNodes, idx, err := decodeSegmentBlock(seg)
	decElapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotNodes) != nodes {
		t.Fatalf("decoded nodes=%d", len(gotNodes))
	}
	for i, nb := range gotNodes {
		if HashBytes(nb) != idx[i].SHA256 {
			t.Fatalf("node %d hash mismatch", i)
		}
	}
	t.Logf("segment decode: %s for %d nodes", decElapsed, nodes)

	totalRaw := 0
	for _, nb := range nodeBlocks {
		totalRaw += len(nb)
	}
	t.Logf("throughput: encode %d bytes in %s (%s/s)", totalRaw, encElapsed, formatRate(totalRaw, encElapsed))
	t.Logf("throughput: decode %d bytes in %s (%s/s)", totalRaw, decElapsed, formatRate(totalRaw, decElapsed))
}

func TestBenchRestoreSpeed(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	const files = 200
	var totalBytes int
	start := time.Now()
	for i := 0; i < files; i++ {
		content := benchPayload(20)
		path := filepath.Join(t.TempDir(), fmt.Sprintf("file%d.go", i))
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
		totalBytes += len(content)
	}
	elapsed := time.Since(start)
	t.Logf("restore materialize %d files (%d bytes): %s", files, totalBytes, elapsed)
}

func formatRate(bytes int, d time.Duration) string {
	perSec := float64(bytes) / d.Seconds()
	switch {
	case perSec > 1<<20:
		return fmt.Sprintf("%.1f MiB/s", perSec/(1<<20))
	case perSec > 1<<10:
		return fmt.Sprintf("%.1f KiB/s", perSec/(1<<10))
	}
	return fmt.Sprintf("%.0f B/s", perSec)
}

func TestBenchDBWriteThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	path := filepath.Join(t.TempDir(), "bench.sqlite")
	db, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const nodes = 2000
	start := time.Now()
	for i := 0; i < nodes; i++ {
		n := NodeMeta{CommitID: fmt.Sprintf("sha256:bench-%d", i), Branch: "main", Seq: uint64(i + 1), TimestampMs: uint64(i)}
		if err := db.RecordNode(n, "segment-bench"); err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	t.Logf("db insert %d nodes: %s (%.0f nodes/s)", nodes, elapsed, float64(nodes)/elapsed.Seconds())
}

func TestBenchEncodeBatchUnderBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	const nodes = 100
	nodeBlocks := make([][]byte, 0, nodes)
	for i := 0; i < nodes; i++ {
		nb, _ := encodeNodeBlockForTest(NodeBlockHeader{NodeID: fmt.Sprintf("sha256:n%d", i), Seq: uint64(i + 1)}, nil, nil)
		nodeBlocks = append(nodeBlocks, nb)
	}
	sh := SegmentHeader{SegmentID: "s-budget", ProjectID: "main", BeginSeq: 1, EndSeq: uint64(nodes), NodeCount: uint64(nodes)}
	start := time.Now()
	seg, err := encodeSegmentBlock(sh, nodeBlocks)
	if err != nil {
		t.Fatal(err)
	}
	encTime := time.Since(start)
	start = time.Now()
	_, got, _, err := decodeSegmentBlock(seg)
	decTime := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != nodes {
		t.Fatalf("nodes=%d", len(got))
	}
	t.Logf("batch %d nodes: encode %s decode %s", nodes, encTime, decTime)
	if encTime+decTime > 2*time.Second {
		t.Fatalf("batch too slow: %s", encTime+decTime)
	}
}

var _ = runtime.NumCPU
var _ = strings.TrimSpace
