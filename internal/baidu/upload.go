package baidu

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

const (
	ChunkSize                = 4 * 1024 * 1024
	memberChunkSize          = 16 * 1024 * 1024
	superVIPChunkSize        = 32 * 1024 * 1024
	defaultUploadConcurrency = 8
)

func FileBlockMD5s(path string) ([]string, string, error) {
	return fileBlockMD5s(path, ChunkSize)
}

func fileBlockMD5s(path string, chunkSize int64) ([]string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	whole := md5.New()
	buf := make([]byte, chunkSize)
	var blocks []string
	for {
		n, readErr := io.ReadFull(f, buf)
		if readErr == io.ErrUnexpectedEOF || readErr == io.EOF {
			if n > 0 {
				sum := md5.Sum(buf[:n])
				blocks = append(blocks, hex.EncodeToString(sum[:]))
				_, _ = whole.Write(buf[:n])
			}
			break
		}
		if readErr != nil {
			return nil, "", readErr
		}
		sum := md5.Sum(buf[:n])
		blocks = append(blocks, hex.EncodeToString(sum[:]))
		_, _ = whole.Write(buf[:n])
	}
	if len(blocks) == 0 {
		sum := md5.Sum(nil)
		blocks = append(blocks, hex.EncodeToString(sum[:]))
	}
	return blocks, hex.EncodeToString(whole.Sum(nil)), nil
}

type precreateResponse struct {
	Errno     int    `json:"errno"`
	Path      string `json:"path"`
	UploadID  string `json:"uploadid"`
	BlockList []int  `json:"block_list"`
}

type uploadPartResponse struct {
	Errno int    `json:"errno"`
	MD5   string `json:"md5"`
}

func (c Client) UploadFile(ctx context.Context, localPath, remotePath string) error {
	if err := c.ensureWritable(); err != nil {
		return err
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", localPath)
	}
	chunkSize := c.uploadChunkSize(ctx, info.Size())
	blocks, wholeMD5, err := fileBlockMD5s(localPath, chunkSize)
	if err != nil {
		return err
	}
	pre, err := c.precreate(ctx, remotePath, info.Size(), blocks, wholeMD5)
	if err != nil {
		return err
	}
	server := strings.TrimRight(c.pcs(), "/")
	parts := pre.BlockList
	if len(parts) == 0 {
		parts = make([]int, len(blocks))
		for i := range blocks {
			parts[i] = i
		}
	}
	if err := c.uploadParts(ctx, server, localPath, remotePath, pre.UploadID, parts, chunkSize); err != nil {
		return err
	}
	return c.create(ctx, remotePath, info.Size(), blocks, pre.UploadID)
}

func (c Client) uploadParts(ctx context.Context, server, localPath, remotePath, uploadID string, parts []int, chunkSize int64) error {
	workers := uploadConcurrency()
	if workers > len(parts) {
		workers = len(parts)
	}
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan int)
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for part := range jobs {
				err := c.uploadPartFromFile(ctx, server, localPath, remotePath, uploadID, part, chunkSize)
				if err != nil {
					select {
					case errCh <- err:
						cancel()
					default:
					}
					return
				}
			}
		}()
	}
sendJobs:
	for _, part := range parts {
		select {
		case <-ctx.Done():
			break sendJobs
		case jobs <- part:
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return ctx.Err()
	}
}

func (c Client) uploadChunkSize(ctx context.Context, fileSize int64) int64 {
	if size, ok := uploadChunkSizeFromEnv(); ok {
		return size
	}
	if fileSize <= ChunkSize {
		return ChunkSize
	}
	maxSize := int64(ChunkSize)
	info, err := c.UserInfo(ctx)
	if err != nil {
		return ChunkSize
	}
	switch info.VIPType {
	case VIPSuper:
		maxSize = superVIPChunkSize
	case VIPMember:
		maxSize = memberChunkSize
	}
	return adaptiveUploadChunkSize(fileSize, maxSize, uploadConcurrency())
}

func adaptiveUploadChunkSize(fileSize, maxSize int64, concurrency int) int64 {
	if concurrency < 1 {
		concurrency = 1
	}
	targetParts := int64(concurrency * 4)
	size := (fileSize + targetParts - 1) / targetParts
	if rem := size % ChunkSize; rem != 0 {
		size += ChunkSize - rem
	}
	if size < ChunkSize {
		return ChunkSize
	}
	if size > maxSize {
		return maxSize
	}
	return size
}

func uploadChunkSizeFromEnv() (int64, bool) {
	raw := os.Getenv("BDY_UPLOAD_CHUNK_SIZE")
	if raw == "" || raw == "auto" {
		return 0, false
	}
	size, err := parseByteSize(raw)
	if err != nil || size < ChunkSize {
		return ChunkSize, true
	}
	if size > superVIPChunkSize {
		return superVIPChunkSize, true
	}
	return size, true
}

func parseByteSize(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, strconv.ErrSyntax
	}
	i := 0
	for ; i < len(raw); i++ {
		if !unicode.IsDigit(rune(raw[i])) {
			break
		}
	}
	n, err := strconv.ParseInt(strings.TrimSpace(raw[:i]), 10, 64)
	if err != nil {
		return 0, err
	}
	unit := strings.ToLower(strings.TrimSpace(raw[i:]))
	switch unit {
	case "", "b":
		return n, nil
	case "k", "kb", "kib":
		return n * 1024, nil
	case "m", "mb", "mib":
		return n * 1024 * 1024, nil
	case "g", "gb", "gib":
		return n * 1024 * 1024 * 1024, nil
	default:
		return 0, strconv.ErrSyntax
	}
}

func uploadConcurrency() int {
	raw := os.Getenv("BDY_UPLOAD_CONCURRENCY")
	if raw == "" {
		return defaultUploadConcurrency
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return defaultUploadConcurrency
	}
	if n > 16 {
		return 16
	}
	return n
}

func (c Client) Mkdir(ctx context.Context, remotePath string) error {
	if err := c.ensureWritable(); err != nil {
		return err
	}
	q := url.Values{}
	q.Set("method", "create")
	form := url.Values{}
	form.Set("path", remotePath)
	form.Set("isdir", "1")
	form.Set("rtype", "0")
	var out struct {
		Errno int `json:"errno"`
	}
	return c.postForm(ctx, c.pan()+"/rest/2.0/xpan/file", q, form, &out)
}

func (c Client) precreate(ctx context.Context, remotePath string, size int64, blocks []string, contentMD5 string) (precreateResponse, error) {
	raw, _ := json.Marshal(blocks)
	q := url.Values{}
	q.Set("method", "precreate")
	form := url.Values{}
	form.Set("path", remotePath)
	form.Set("size", fmt.Sprintf("%d", size))
	form.Set("isdir", "0")
	form.Set("autoinit", "1")
	form.Set("rtype", "3")
	form.Set("block_list", string(raw))
	form.Set("content-md5", contentMD5)
	if len(blocks) > 0 {
		form.Set("slice-md5", blocks[0])
	}
	var out precreateResponse
	err := c.postForm(ctx, c.pan()+"/rest/2.0/xpan/file", q, form, &out)
	return out, err
}

func (c Client) uploadPart(ctx context.Context, server, chunkPath, remotePath, uploadID string, partseq int) error {
	f, err := os.Open(chunkPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return c.uploadPartReader(ctx, server, f, info.Size(), chunkPath, remotePath, uploadID, partseq)
}

func (c Client) uploadPartFromFile(ctx context.Context, server, localPath, remotePath, uploadID string, partseq int, chunkSize int64) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	offset := int64(partseq) * chunkSize
	info, err := f.Stat()
	if err != nil {
		return err
	}
	partSize := info.Size() - offset
	if partSize > chunkSize {
		partSize = chunkSize
	}
	if partSize < 0 {
		partSize = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	src := io.LimitReader(f, partSize)
	return c.uploadPartReader(ctx, server, src, partSize, localPath, remotePath, uploadID, partseq)
}

func (c Client) uploadPartReader(ctx context.Context, server string, src io.Reader, size int64, fileName, remotePath, uploadID string, partseq int) error {
	q := url.Values{}
	q.Set("method", "upload")
	q.Set("type", "tmpfile")
	q.Set("path", remotePath)
	q.Set("uploadid", uploadID)
	q.Set("partseq", fmt.Sprintf("%d", partseq))
	var out uploadPartResponse
	return c.postMultipartReader(ctx, server+"/rest/2.0/pcs/superfile2", q, "file", filepathBase(fileName), src, size, &out)
}

func (c Client) create(ctx context.Context, remotePath string, size int64, blocks []string, uploadID string) error {
	raw, _ := json.Marshal(blocks)
	q := url.Values{}
	q.Set("method", "create")
	form := url.Values{}
	form.Set("path", remotePath)
	form.Set("size", fmt.Sprintf("%d", size))
	form.Set("isdir", "0")
	form.Set("rtype", "3")
	form.Set("uploadid", uploadID)
	form.Set("block_list", string(raw))
	var out struct {
		Errno int `json:"errno"`
	}
	return c.postForm(ctx, c.pan()+"/rest/2.0/xpan/file", q, form, &out)
}

func filepathBase(path string) string {
	path = strings.TrimRight(path, "/")
	idx := strings.LastIndex(path, "/")
	if idx >= 0 {
		return path[idx+1:]
	}
	return path
}
