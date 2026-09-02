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

type SearchOptions struct {
	Dir      string
	Key      string
	PageSize int
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

func (c Client) Search(ctx context.Context, opts SearchOptions, page int) ([]FileEntry, error) {
	if opts.Dir == "" {
		opts.Dir = "/"
	}
	if opts.PageSize <= 0 {
		opts.PageSize = 500
	}
	if page <= 0 {
		page = 1
	}
	q := url.Values{}
	q.Set("method", "search")
	q.Set("dir", opts.Dir)
	q.Set("key", opts.Key)
	q.Set("recursion", "1")
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("num", fmt.Sprintf("%d", opts.PageSize))
	var out listResponse
	if err := c.getJSON(ctx, c.pan()+"/rest/2.0/xpan/file", q, &out); err != nil {
		return nil, err
	}
	return out.List, nil
}

func (c Client) SearchAll(ctx context.Context, opts SearchOptions) ([]FileEntry, error) {
	if opts.PageSize <= 0 {
		opts.PageSize = 500
	}
	var all []FileEntry
	for page := 1; ; page++ {
		items, err := c.Search(ctx, opts, page)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < opts.PageSize {
			return all, nil
		}
	}
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
	return c.DownloadWithProgress(ctx, dlink, dest, nil)
}

func (c Client) DownloadWithProgress(ctx context.Context, dlink, dest string, progress io.Writer) error {
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
	body := io.Reader(resp.Body)
	if progress != nil {
		body = newProgressReader(resp.Body, resp.ContentLength, filepath.Base(dest), progress)
	}
	_, copyErr := io.Copy(f, body)
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

type progressReader struct {
	src         io.Reader
	total       int64
	name        string
	out         io.Writer
	read        int64
	lastPercent int64
	nextUnknown int64
}

func newProgressReader(src io.Reader, total int64, name string, out io.Writer) *progressReader {
	fmt.Fprintf(out, "downloading %s: 0/%s\n", name, formatBytes(total))
	return &progressReader{src: src, total: total, name: name, out: out, lastPercent: -1, nextUnknown: 1024 * 1024}
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.read += int64(n)
		if r.total > 0 {
			pct := r.read * 100 / r.total
			if pct != r.lastPercent || r.read >= r.total {
				r.lastPercent = pct
				fmt.Fprintf(r.out, "downloading %s: %s/%s %d%%\n", r.name, formatBytes(r.read), formatBytes(r.total), pct)
			}
		} else if r.read >= r.nextUnknown {
			fmt.Fprintf(r.out, "downloading %s: %s\n", r.name, formatBytes(r.read))
			r.nextUnknown += 1024 * 1024
		}
	}
	return n, err
}

func formatBytes(n int64) string {
	if n < 0 {
		return "unknown"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PB", value/unit)
}

func (c Client) FileManager(ctx context.Context, opera string, filelist any) error {
	if err := c.ensureWritable(); err != nil {
		return err
	}
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
