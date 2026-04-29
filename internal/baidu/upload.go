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
	"path/filepath"
)

const ChunkSize = 4 * 1024 * 1024

func FileBlockMD5s(path string) ([]string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	whole := md5.New()
	buf := make([]byte, ChunkSize)
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

type locateResponse struct {
	Servers []struct {
		Server string `json:"server"`
	} `json:"servers"`
}

type uploadPartResponse struct {
	Errno int    `json:"errno"`
	MD5   string `json:"md5"`
}

func (c Client) UploadFile(ctx context.Context, localPath, remotePath string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", localPath)
	}
	blocks, wholeMD5, err := FileBlockMD5s(localPath)
	if err != nil {
		return err
	}
	pre, err := c.precreate(ctx, remotePath, info.Size(), blocks, wholeMD5)
	if err != nil {
		return err
	}
	server, err := c.locateUpload(ctx, remotePath, pre.UploadID)
	if err != nil {
		return err
	}
	parts := pre.BlockList
	if len(parts) == 0 {
		parts = make([]int, len(blocks))
		for i := range blocks {
			parts[i] = i
		}
	}
	for _, part := range parts {
		tmp, err := chunkToTemp(localPath, part)
		if err != nil {
			return err
		}
		err = c.uploadPart(ctx, server, tmp, remotePath, pre.UploadID, part)
		_ = os.Remove(tmp)
		if err != nil {
			return err
		}
	}
	return c.create(ctx, remotePath, info.Size(), blocks, pre.UploadID)
}

func (c Client) Mkdir(ctx context.Context, remotePath string) error {
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

func (c Client) locateUpload(ctx context.Context, remotePath, uploadID string) (string, error) {
	q := url.Values{}
	q.Set("method", "locateupload")
	q.Set("appid", "250528")
	q.Set("path", remotePath)
	q.Set("uploadid", uploadID)
	q.Set("upload_version", "2.0")
	var out locateResponse
	if err := c.getJSON(ctx, c.pcs()+"/rest/2.0/pcs/file", q, &out); err != nil {
		return "", err
	}
	for _, srv := range out.Servers {
		if srv.Server != "" && len(srv.Server) >= 8 && srv.Server[:8] == "https://" {
			return srv.Server, nil
		}
	}
	return "", fmt.Errorf("no https upload server returned")
}

func (c Client) uploadPart(ctx context.Context, server, chunkPath, remotePath, uploadID string, partseq int) error {
	q := url.Values{}
	q.Set("method", "upload")
	q.Set("type", "tmpfile")
	q.Set("path", remotePath)
	q.Set("uploadid", uploadID)
	q.Set("partseq", fmt.Sprintf("%d", partseq))
	var out uploadPartResponse
	return c.postMultipartFile(ctx, server+"/rest/2.0/pcs/superfile2", q, "file", chunkPath, &out)
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

func chunkToTemp(path string, part int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Seek(int64(part)*ChunkSize, io.SeekStart); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", "bdy-part-*")
	if err != nil {
		return "", err
	}
	_, copyErr := io.CopyN(tmp, f, ChunkSize)
	if copyErr == io.EOF || copyErr == io.ErrUnexpectedEOF {
		copyErr = nil
	}
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmp.Name())
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp.Name())
		return "", closeErr
	}
	return filepath.Abs(tmp.Name())
}
