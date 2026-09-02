package timeline

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfigFreezesPackSizing(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SegmentSize != 5 {
		t.Fatalf("segment size=%d", cfg.SegmentSize)
	}
	if cfg.ArchiveSize != 100 {
		t.Fatalf("archive size=%d", cfg.ArchiveSize)
	}
	if cfg.CheckpointInterval != 100 {
		t.Fatalf("checkpoint interval=%d", cfg.CheckpointInterval)
	}
	if cfg.ChunkSize != 4*1024*1024 || cfg.LargeFileThreshold != 4*1024*1024 {
		t.Fatalf("chunk=%d threshold=%d", cfg.ChunkSize, cfg.LargeFileThreshold)
	}
	if cfg.Compression != "zstd" {
		t.Fatalf("compression=%q", cfg.Compression)
	}
	if cfg.GCGracePeriod != 7*24*time.Hour {
		t.Fatalf("gc grace=%s", cfg.GCGracePeriod)
	}
}

func TestConfigWithDefaultsPreservesExplicitValues(t *testing.T) {
	cfg := (Config{SegmentSize: 10, Compression: "none"}).WithDefaults()
	if cfg.SegmentSize != 10 {
		t.Fatalf("segment size=%d", cfg.SegmentSize)
	}
	if cfg.Compression != "none" {
		t.Fatalf("compression=%q", cfg.Compression)
	}
	if cfg.ArchiveSize != DefaultArchiveSize || cfg.GCGracePeriod != DefaultGCGracePeriod {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
}

func TestLayoutDirsAndRemotePaths(t *testing.T) {
	layout := NewLayout(filepath.Join("repo", ".bdynd", "timeline"))
	if got := layout.ArchivesDir(); got != filepath.Join("repo", ".bdynd", "timeline", "archives") {
		t.Fatalf("archives dir=%q", got)
	}
	if got := layout.ObjectPath("sha256-abc"); got != filepath.Join("repo", ".bdynd", "timeline", "objects", "sha256-abc") {
		t.Fatalf("object path=%q", got)
	}
	if len(layout.Dirs()) != 7 {
		t.Fatalf("dir count=%d", len(layout.Dirs()))
	}

	remote := NewRemoteLayout("/apps/baiduyunStorage/nd/repos/demo/")
	if got := remote.ArchivePackPath("archive-main-000001-000100"); got != "/apps/baiduyunStorage/nd/repos/demo/archives/archive-main-000001-000100.pack.zst" {
		t.Fatalf("remote archive path=%q", got)
	}
	if got := remote.RefPath("main"); got != "/apps/baiduyunStorage/nd/repos/demo/refs/heads/main.json" {
		t.Fatalf("remote ref path=%q", got)
	}
	if got := remote.TimelinePath("main"); got != "/apps/baiduyunStorage/nd/repos/demo/timelines/main.index.json" {
		t.Fatalf("remote timeline path=%q", got)
	}
}

func TestBlockIDFormats(t *testing.T) {
	if got := BlockID(KindArchive, "main", 1, 100); got != "archive-main-000001-000100" {
		t.Fatalf("archive id=%q", got)
	}
	if got := BlockID(KindSegment, "main", 6, 10); got != "segment-main-000006-000010" {
		t.Fatalf("segment id=%q", got)
	}
	if got := BlockID(KindCheckpoint, "main", 100, 100); got != "checkpoint-main-000100" {
		t.Fatalf("checkpoint id=%q", got)
	}
}

func TestHashBytesStableSHA256(t *testing.T) {
	a := HashBytes([]byte("hello"))
	b := HashBytes([]byte("hello"))
	if a != b {
		t.Fatalf("hash not stable")
	}
	if len(a) != 64 {
		t.Fatalf("hash length=%d", len(a))
	}
}
