package timeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// Default and cap for concurrent remote operations.
const (
	// DefaultUploadConcurrency is the number of blocks uploaded in parallel.
	DefaultUploadConcurrency = 4
	// DefaultDownloadConcurrency is the number of blocks downloaded in parallel.
	DefaultDownloadConcurrency = 4
	// MaxTransferConcurrency bounds how high either value may be set.
	MaxTransferConcurrency = 8
)

// BoundedTransfer wraps a RemoteTransfer and adds a concurrency limit plus
// bounded retries with backoff for remote operation failures. It is safe for
// concurrent use by multiple goroutines.
type BoundedTransfer struct {
	base   RemoteTransfer
	upload chan struct{}
	down   chan struct{}
	// Retries is the number of times a failed operation is retried (default 3).
	Retries int
	// BackoffBase is the initial backoff between retries (default 200ms).
	BackoffBase time.Duration
	// Logger receives retry/backoff events; nil means silent.
	Log Logger
}

// NewBoundedTransfer wraps base with concurrency limits. uploadN/downloadN are
// clamped to [1, MaxTransferConcurrency]. A nil base is allowed so callers can
// defer wiring; operations will fail with a clear error.
func NewBoundedTransfer(base RemoteTransfer, uploadN, downloadN int) *BoundedTransfer {
	if uploadN < 1 {
		uploadN = DefaultUploadConcurrency
	}
	if uploadN > MaxTransferConcurrency {
		uploadN = MaxTransferConcurrency
	}
	if downloadN < 1 {
		downloadN = DefaultDownloadConcurrency
	}
	if downloadN > MaxTransferConcurrency {
		downloadN = MaxTransferConcurrency
	}
	return &BoundedTransfer{
		base:        base,
		upload:      make(chan struct{}, uploadN),
		down:        make(chan struct{}, downloadN),
		Retries:     3,
		BackoffBase: 200 * time.Millisecond,
		Log:         NopLogger{},
	}
}

func (t *BoundedTransfer) log(ctx context.Context, level LogLevel, op, id, msg string) {
	if t.Log != nil {
		t.Log.Log(ctx, LogEvent{Level: level, Op: op, BlockID: id, Message: msg})
	}
}

// UploadFile acquires the upload slot then uploads, with retry+backoff.
func (t *BoundedTransfer) UploadFile(ctx context.Context, localPath, remotePath string) error {
	if t.base == nil {
		return errors.New("BoundedTransfer: no underlying transfer configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case t.upload <- struct{}{}:
	}
	defer func() { <-t.upload }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return retry(ctx, t.Retries, t.BackoffBase, t.Log, "upload", filepathBaseForLog(remotePath),
		func() error { return t.base.UploadFile(ctx, localPath, remotePath) })
}

// DownloadFile acquires the download slot then downloads, with retry+backoff.
func (t *BoundedTransfer) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	if t.base == nil {
		return errors.New("BoundedTransfer: no underlying transfer configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case t.down <- struct{}{}:
	}
	defer func() { <-t.down }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return retry(ctx, t.Retries, t.BackoffBase, t.Log, "download", filepathBaseForLog(remotePath),
		func() error { return t.base.DownloadFile(ctx, remotePath, localPath) })
}

// Exists passes through without a slot (cheap metadata call).
func (t *BoundedTransfer) Exists(ctx context.Context, remotePath string) (bool, error) {
	if t.base == nil {
		return false, errors.New("BoundedTransfer: no underlying transfer configured")
	}
	return t.base.Exists(ctx, remotePath)
}

// ListFiles passes through without a slot (metadata call).
func (t *BoundedTransfer) ListFiles(ctx context.Context, remoteRoot string) ([]string, error) {
	if t.base == nil {
		return nil, errors.New("BoundedTransfer: no underlying transfer configured")
	}
	return t.base.ListFiles(ctx, remoteRoot)
}

// DeleteFiles passes through without a slot.
func (t *BoundedTransfer) DeleteFiles(ctx context.Context, remotePaths []string) error {
	if t.base == nil {
		return errors.New("BoundedTransfer: no underlying transfer configured")
	}
	return t.base.DeleteFiles(ctx, remotePaths)
}

// retry runs fn, retrying up to n times with exponential backoff. Non-idempotent
// operations must ensure their underlying implementation is safe to retry at the
// granularity of fn. Context cancellation aborts immediately.
func retry(ctx context.Context, n int, base time.Duration, log Logger, op, id string, fn func() error) error {
	var err error
	for attempt := 0; ; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if n <= 0 || attempt >= n || ctx.Err() != nil {
			return err
		}
		delay := base << uint(attempt)
		if delay < base {
			delay = base
		}
		if delay > 5*time.Second {
			delay = 5 * time.Second
		}
		if log != nil {
			log.Log(ctx, LogEvent{Level: LevelWarn, Op: op, BlockID: id,
				Message: fmt.Sprintf("retry %d after error: %v", attempt+1, err)})
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// UploadBatch uploads multiple blocks concurrently, bounded by the upload slot.
// Each item returns its error independently; a nil error means success. Results
// are ordered to match ids.
func (t *BoundedTransfer) UploadBatch(ctx context.Context, ids []string, fn func(ctx context.Context, id string) error) []error {
	return runConcurrent(ctx, ids, t.upload, fn)
}

// DownloadBatch downloads multiple blocks concurrently, bounded by the download slot.
func (t *BoundedTransfer) DownloadBatch(ctx context.Context, ids []string, fn func(ctx context.Context, id string) error) []error {
	return runConcurrent(ctx, ids, t.down, fn)
}

// runConcurrent fans ids out to workers, each holding a slot from the semaphore,
// collecting one result per id in input order. Context cancellation stops new
// work; already-started workers finish and report.
func runConcurrent(ctx context.Context, ids []string, sem chan struct{}, fn func(ctx context.Context, id string) error) []error {
	out := make([]error, len(ids))
	if len(ids) == 0 {
		return out
	}
	var wg sync.WaitGroup
	for i, id := range ids {
		i, id := i, id
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				out[i] = ctx.Err()
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			out[i] = fn(ctx, id)
		}()
	}
	wg.Wait()
	return out
}

func filepathBaseForLog(path string) string {
	base := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			base = path[i+1:]
			break
		}
	}
	if base == "" {
		return path
	}
	return base
}

var _ = os.ErrNotExist
