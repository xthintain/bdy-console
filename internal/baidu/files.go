package baidu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type FileEntry struct {
	FSID           uint64 `json:"fs_id"`
	Path           string `json:"path"`
	ServerFilename string `json:"server_filename"`
	Size           int64  `json:"size"`
	ServerMTime    int64  `json:"server_mtime"`
	IsDir          int    `json:"isdir"`
	MD5            string `json:"md5"`
	DLink          string `json:"dlink"`
}

type listResponse struct {
	Errno int         `json:"errno"`
	List  []FileEntry `json:"list"`
}

func (c Client) List(ctx context.Context, dir string) ([]FileEntry, error) {
	q := url.Values{}
	q.Set("method", "list")
	q.Set("dir", dir)
	q.Set("order", "name")
	q.Set("start", "0")
	q.Set("limit", "1000")
	var out listResponse
	if err := c.getJSON(ctx, c.pan()+"/rest/2.0/xpan/file", q, &out); err != nil {
		return nil, err
	}
	return out.List, nil
}

func (c Client) ListAll(ctx context.Context, path string) ([]FileEntry, error) {
	q := url.Values{}
	q.Set("method", "listall")
	q.Set("path", path)
	q.Set("recursion", "1")
	q.Set("start", "0")
	q.Set("limit", "10000")
	var out listResponse
	if err := c.getJSON(ctx, c.pan()+"/rest/2.0/xpan/multimedia", q, &out); err != nil {
		return nil, err
	}
	return out.List, nil
}

type metasResponse struct {
	Errno int         `json:"errno"`
	List  []FileEntry `json:"list"`
}

func (c Client) FileMetas(ctx context.Context, fsids []uint64, dlink bool) ([]FileEntry, error) {
	raw, err := json.Marshal(fsids)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("method", "filemetas")
	q.Set("fsids", string(raw))
	if dlink {
		q.Set("dlink", "1")
	}
	var out metasResponse
	if err := c.getJSON(ctx, c.pan()+"/rest/2.0/xpan/multimedia", q, &out); err != nil {
		return nil, err
	}
	return out.List, nil
}

func (c Client) Download(ctx context.Context, dlink, dest string) error {
	sep := "?"
	if strings.Contains(dlink, "?") {
		sep = "&"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlink+sep+"access_token="+url.QueryEscape(c.Config.AccessToken), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download http status %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".bdy-download"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, dest)
}

func (c Client) FileManager(ctx context.Context, opera string, filelist any) error {
	raw, err := json.Marshal(filelist)
	if err != nil {
		return err
	}
	q := url.Values{}
	q.Set("method", "filemanager")
	q.Set("opera", opera)
	form := url.Values{}
	form.Set("async", "1")
	form.Set("filelist", string(raw))
	form.Set("ondup", "overwrite")
	var out struct {
		Errno int `json:"errno"`
	}
	return c.postForm(ctx, c.pan()+"/rest/2.0/xpan/file", q, form, &out)
}

func (c Client) pan() string {
	if c.PanURL != "" {
		return c.PanURL
	}
	return PanBaseURL
}

func (c Client) pcs() string {
	if c.PCSURL != "" {
		return c.PCSURL
	}
	return PCSBaseURL
}
