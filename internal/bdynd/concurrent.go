package bdynd

import (
	"context"
	"os"
	"path/filepath"
	"sync"
)

// defaultConcurrency is the number of parallel object operations during
// push/fetch of reachable objects.
const defaultConcurrency = 4

// workItem is one remote object transfer decided upfront so the same object is
// never scheduled twice within a run.
type workItem struct {
	kind  string
	oid   string
	local string
	path  string
}

// scheduleReachableCollects walks the commit chain from oid and appends one
// work item per missing object. Presence checks against remote are batched and
// executed concurrently via remote.Exists; local presence is checked directly.
func (s *ConcurrentRunner) collect(r Repo, remote RemoteStore, remoteRoot, oid string) ([]workItem, error) {
	seen := map[string]bool{}
	var items []workItem
	add := func(kind, oid, local, path string) {
		key := kind + ":" + oid
		if seen[key] {
			return
		}
		seen[key] = true
		items = append(items, workItem{kind: kind, oid: oid, local: local, path: path})
	}
	for oid != "" {
		c, err := ReadCommit(r, oid)
		if err != nil {
			return nil, err
		}
		add("commits", oid, localObjectPath(r, "commits", oid), RemoteCommitPath(remoteRoot, oid))
		add("trees", c.Tree, localObjectPath(r, "trees", c.Tree), RemoteTreePath(remoteRoot, c.Tree))
		for _, entry := range c.Entries {
			if entry.Kind == KindBlob {
				add("blobs", entry.OID, localObjectPath(r, "blobs", entry.OID), RemoteBlobPath(remoteRoot, entry.OID))
			}
		}
		oid = c.Parent
	}
	return items, nil
}

// filterRemoteMissing keeps only work items whose remote path does not exist,
// so push uploads only missing objects. Exists calls run concurrently; an
// error on a probe is treated as "missing" so the object is retried by upload.
func (s *ConcurrentRunner) filterRemoteMissing(ctx context.Context, remote RemoteStore, items []workItem) ([]workItem, error) {
	keep := make([]workItem, 0, len(items))
	var wg sync.WaitGroup
	missing := make([]bool, len(items))
	for i := range items {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := remote.Exists(ctx, items[i].path)
			if err != nil {
				missing[i] = true
				return
			}
			missing[i] = !ok
		}()
	}
	wg.Wait()
	for i, it := range items {
		if missing[i] {
			keep = append(keep, it)
		}
	}
	return keep, nil
}

// run executes fn for each work item with bounded concurrency, stopping early
// on the first error. Context cancellation also stops dispatch.
func (s *ConcurrentRunner) run(ctx context.Context, items []workItem, fn func(item workItem) error) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, s.n)
	errCh := make(chan error, 1)
	for _, it := range items {
		it := it
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			if err := fn(it); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return ctx.Err()
	}
}

// ConcurrentRunner executes a batch of independent remote object transfers with
// bounded concurrency. Uploads/downloads of different objects are independent,
// so they can proceed in parallel; presence checks happen first.
type ConcurrentRunner struct {
	n int
}

// NewConcurrentRunner returns a runner with a concurrency of at most n
// (clamped to >=1). Use n=0 to pick the package default.
func NewConcurrentRunner(n int) *ConcurrentRunner {
	if n < 1 {
		n = defaultConcurrency
	}
	return &ConcurrentRunner{n: n}
}

// Push uploads all reachable objects from oid that are missing remotely.
func (s *ConcurrentRunner) Push(ctx context.Context, r Repo, remote RemoteStore, remoteRoot, oid string) error {
	items, err := s.collect(r, remote, remoteRoot, oid)
	if err != nil {
		return err
	}
	items, err = s.filterRemoteMissing(ctx, remote, items)
	if err != nil {
		return err
	}
	return s.run(ctx, items, func(it workItem) error {
		return remote.UploadFile(ctx, it.local, it.path)
	})
}

// Fetch downloads all reachable objects from oid that are missing locally.
func (s *ConcurrentRunner) Fetch(ctx context.Context, r Repo, remote RemoteStore, remoteRoot, oid string) error {
	items, err := s.collect(r, remote, remoteRoot, oid)
	if err != nil {
		return err
	}
	return s.run(ctx, items, func(it workItem) error {
		if _, err := os.Stat(it.local); err == nil {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(it.local), 0o755); err != nil {
			return err
		}
		return remote.DownloadFile(ctx, it.path, it.local)
	})
}
