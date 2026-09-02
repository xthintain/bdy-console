package timeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// failingTransfer fails the first n calls to UploadFile, then succeeds.
type failingTransfer struct {
	mu      sync.Mutex
	failUp  int
	failDn  int
	callsUp atomic.Int64
	callsDn atomic.Int64
}

func (f *failingTransfer) UploadFile(_ context.Context, localPath, remotePath string) error {
	f.callsUp.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failUp > 0 {
		f.failUp--
		return fmt.Errorf("injected upload failure")
	}
	return os.WriteFile(remotePath, []byte("x"), 0o644)
}

func (f *failingTransfer) DownloadFile(_ context.Context, remotePath, localPath string) error {
	f.callsDn.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDn > 0 {
		f.failDn--
		return fmt.Errorf("injected download failure")
	}
	return os.WriteFile(localPath, []byte("y"), 0o644)
}

func (f *failingTransfer) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *failingTransfer) ListFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (f *failingTransfer) DeleteFiles(_ context.Context, _ []string) error { return nil }

func TestBoundedTransferRetryOnUpload(t *testing.T) {
	base := &failingTransfer{failUp: 2}
	bt := NewBoundedTransfer(base, 2, 2)
	bt.Retries = 5
	bt.BackoffBase = time.Millisecond

	local := filepath.Join(t.TempDir(), "a.bin")
	if err := os.WriteFile(local, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(t.TempDir(), "a.bin")

	if err := bt.UploadFile(context.Background(), local, remote); err != nil {
		t.Fatalf("upload should succeed after retries: %v", err)
	}
	if base.callsUp.Load() != 3 {
		t.Fatalf("expected 3 upload calls (1+2 retries), got %d", base.callsUp.Load())
	}
}

func TestBoundedTransferFailsAfterExhaustingRetries(t *testing.T) {
	base := &failingTransfer{failUp: 100}
	bt := NewBoundedTransfer(base, 2, 2)
	bt.Retries = 2
	bt.BackoffBase = time.Millisecond

	local := filepath.Join(t.TempDir(), "a.bin")
	if err := os.WriteFile(local, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(t.TempDir(), "a.bin")
	if err := bt.UploadFile(context.Background(), local, remote); err == nil {
		t.Fatal("expected failure after exhausting retries")
	}
	if base.callsUp.Load() != 3 { // 1 + 2 retries
		t.Fatalf("expected 3 calls, got %d", base.callsUp.Load())
	}
}

func TestBoundedTransferCapsConcurrency(t *testing.T) {
	bt := NewBoundedTransfer(nil, 99, 0)
	if cap(bt.upload) != MaxTransferConcurrency {
		t.Fatalf("upload cap=%d, want %d", cap(bt.upload), MaxTransferConcurrency)
	}
	if cap(bt.down) != DefaultDownloadConcurrency {
		t.Fatalf("download cap=%d, want default %d", cap(bt.down), DefaultDownloadConcurrency)
	}
}

func TestBoundedTransferConcurrentUploadsBounded(t *testing.T) {
	var inflight atomic.Int64
	var maxInflight atomic.Int64
	base := &trackingTransfer{onUpload: func() {
		cur := inflight.Add(1)
		for {
			old := maxInflight.Load()
			if cur <= old || maxInflight.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		inflight.Add(-1)
	}}
	bt := NewBoundedTransfer(base, 3, 3)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			local := filepath.Join(t.TempDir(), fmt.Sprintf("f%d", i))
			remote := filepath.Join(t.TempDir(), fmt.Sprintf("r%d", i))
			_ = os.WriteFile(local, []byte("d"), 0o644)
			if err := bt.UploadFile(ctx, local, remote); err != nil {
				t.Errorf("upload %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if max := maxInflight.Load(); max != 3 {
		t.Fatalf("max concurrent uploads=%d, want 3", max)
	}
}

func TestBoundedTransferBatchConcurrency(t *testing.T) {
	var inflight atomic.Int64
	var maxInflight atomic.Int64
	base := &trackingTransfer{onUpload: func() {
		cur := inflight.Add(1)
		for {
			old := maxInflight.Load()
			if cur <= old || maxInflight.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		inflight.Add(-1)
	}}
	bt := NewBoundedTransfer(base, 2, 2)
	ids := make([]string, 10)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	errs := bt.UploadBatch(context.Background(), ids, func(_ context.Context, id string) error {
		return bt.base.UploadFile(context.Background(), filepath.Join(t.TempDir(), id), filepath.Join(t.TempDir(), id+".r"))
	})
	for i, e := range errs {
		if e != nil {
			t.Fatalf("batch item %d error: %v", i, e)
		}
	}
	if max := maxInflight.Load(); max != 2 {
		t.Fatalf("max concurrent uploads=%d, want 2", max)
	}
}

func TestBoundedTransferNilBaseErrors(t *testing.T) {
	bt := NewBoundedTransfer(nil, 2, 2)
	if err := bt.UploadFile(context.Background(), "a", "b"); err == nil {
		t.Fatal("expected error for nil base")
	}
	if err := bt.DownloadFile(context.Background(), "a", "b"); err == nil {
		t.Fatal("expected error for nil base")
	}
	if _, err := bt.Exists(context.Background(), "a"); err == nil {
		t.Fatal("expected error for nil base")
	}
}

func TestBoundedTransferContextCancellation(t *testing.T) {
	base := &trackingTransfer{onUpload: func() { time.Sleep(10 * time.Millisecond) }}
	bt := NewBoundedTransfer(base, 1, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	local := filepath.Join(t.TempDir(), "a")
	remote := filepath.Join(t.TempDir(), "r")
	_ = os.WriteFile(local, []byte("d"), 0o644)
	if err := bt.UploadFile(ctx, local, remote); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

// trackingTransfer records concurrent inflight count via a callback.
type trackingTransfer struct {
	onUpload   func()
	onDownload func()
}

func (t *trackingTransfer) UploadFile(_ context.Context, localPath, remotePath string) error {
	if t.onUpload != nil {
		t.onUpload()
	}
	return os.WriteFile(remotePath, []byte("x"), 0o644)
}

func (t *trackingTransfer) DownloadFile(_ context.Context, remotePath, localPath string) error {
	if t.onDownload != nil {
		t.onDownload()
	}
	return os.WriteFile(localPath, []byte("y"), 0o644)
}

func (t *trackingTransfer) Exists(_ context.Context, _ string) (bool, error) { return true, nil }
func (t *trackingTransfer) ListFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (t *trackingTransfer) DeleteFiles(_ context.Context, _ []string) error { return nil }

var _ = errors.New
