package timeline

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	DirRefs        = "refs"
	DirTimelines   = "timelines"
	DirCheckpoints = "checkpoints"
	DirArchives    = "archives"
	DirPending     = "pending"
	DirObjects     = "objects"
	DirCache       = "cache"
)

// Layout describes local working directories under the timeline root. The
// timeline root lives inside the repository, for example
// <repo>/.bdynd/timeline.
type Layout struct {
	Root string
}

func NewLayout(root string) Layout {
	return Layout{Root: root}
}

func (l Layout) RefsDir() string        { return filepath.Join(l.Root, DirRefs) }
func (l Layout) TimelinesDir() string   { return filepath.Join(l.Root, DirTimelines) }
func (l Layout) CheckpointsDir() string { return filepath.Join(l.Root, DirCheckpoints) }
func (l Layout) ArchivesDir() string    { return filepath.Join(l.Root, DirArchives) }
func (l Layout) PendingDir() string     { return filepath.Join(l.Root, DirPending) }
func (l Layout) ObjectsDir() string     { return filepath.Join(l.Root, DirObjects) }
func (l Layout) CacheDir() string       { return filepath.Join(l.Root, DirCache) }

func (l Layout) Dirs() []string {
	return []string{
		l.RefsDir(),
		l.TimelinesDir(),
		l.CheckpointsDir(),
		l.ArchivesDir(),
		l.PendingDir(),
		l.ObjectsDir(),
		l.CacheDir(),
	}
}

// ObjectPath returns the local payload path for a content-addressed object id.
func (l Layout) ObjectPath(oid string) string {
	return filepath.Join(l.ObjectsDir(), oid)
}

// RemoteLayout describes the remote (Baidu Netdisk) layout for one project.
// Remote paths are slash-separated cloud paths.
type RemoteLayout struct {
	Root string
}

func NewRemoteLayout(root string) RemoteLayout {
	return RemoteLayout{Root: strings.TrimRight(root, "/")}
}

func (r RemoteLayout) RefsDir() string      { return r.Root + "/refs" }
func (r RemoteLayout) TimelinesDir() string { return r.Root + "/timelines" }
func (r RemoteLayout) CheckpointsDir() string {
	return r.Root + "/checkpoints"
}
func (r RemoteLayout) ArchivesDir() string { return r.Root + "/archives" }

func (r RemoteLayout) RefPath(branch string) string {
	return fmt.Sprintf("%s/heads/%s.json", r.RefsDir(), branch)
}

func (r RemoteLayout) TimelinePath(branch string) string {
	return fmt.Sprintf("%s/%s.index.json", r.TimelinesDir(), branch)
}

func (r RemoteLayout) ArchivePackPath(id string) string {
	return fmt.Sprintf("%s/%s.pack.zst", r.ArchivesDir(), id)
}

func (r RemoteLayout) ArchiveIndexPath(id string) string {
	return fmt.Sprintf("%s/%s.index.json", r.ArchivesDir(), id)
}

func (r RemoteLayout) CheckpointPackPath(id string) string {
	return fmt.Sprintf("%s/%s.pack.zst", r.CheckpointsDir(), id)
}
