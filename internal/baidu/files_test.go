package baidu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"baiduyunStorage/internal/config"
)

func TestListUsesUserAgentAndEncodesDir(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/2.0/xpan/file" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("User-Agent") != UserAgent {
			t.Fatalf("missing user agent")
		}
		q := r.URL.Query()
		if q.Get("method") != "list" || q.Get("dir") != "/apps/baiduyunStorage/测试" || q.Get("access_token") != "tok" {
			t.Fatalf("bad query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(listResponse{List: []FileEntry{{Path: "/apps/baiduyunStorage/测试/a.txt", Size: 3}}})
	}))
	defer server.Close()

	client := Client{Config: config.Config{AccessToken: "tok"}, HTTP: server.Client(), PanURL: server.URL}
	items, err := client.List(context.Background(), "/apps/baiduyunStorage/测试")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Size != 3 {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestSearchAllUsesRemoteSearchPagination(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/rest/2.0/xpan/file" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("method") != "search" || q.Get("dir") != "/" || q.Get("key") != "mp4" || q.Get("recursion") != "1" || q.Get("num") != "2" {
			t.Fatalf("bad query: %s", r.URL.RawQuery)
		}
		switch q.Get("page") {
		case "1":
			_ = json.NewEncoder(w).Encode(listResponse{List: []FileEntry{
				{Path: "/a.mp4", ServerFilename: "a.mp4"},
				{Path: "/b.mp4", ServerFilename: "b.mp4"},
			}})
		case "2":
			_ = json.NewEncoder(w).Encode(listResponse{List: []FileEntry{{Path: "/c.mp4", ServerFilename: "c.mp4"}}})
		default:
			t.Fatalf("unexpected page %s", q.Get("page"))
		}
	}))
	defer server.Close()

	client := Client{Config: config.Config{AccessToken: "tok"}, HTTP: server.Client(), PanURL: server.URL}
	items, err := client.SearchAll(context.Background(), SearchOptions{Dir: "/", Key: "mp4", PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || requests != 2 {
		t.Fatalf("items=%+v requests=%d", items, requests)
	}
}

func TestDownloadWithProgressReportsBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("access_token") != "tok" {
			t.Fatalf("missing token: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "hello.txt")
	var progress bytes.Buffer
	client := Client{Config: config.Config{AccessToken: "tok"}, HTTP: server.Client()}
	if err := client.DownloadWithProgress(context.Background(), server.URL+"/file", dest, &progress); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("data=%q", data)
	}
	if got := progress.String(); !strings.Contains(got, "downloading hello.txt:") || !strings.Contains(got, "5 B") {
		t.Fatalf("progress=%q", got)
	}
}

func TestProgressReaderThrottlesKnownSizeOutput(t *testing.T) {
	data := strings.Repeat("x", 1024*1024)
	var progress bytes.Buffer
	reader := newProgressReader(strings.NewReader(data), int64(len(data)), "large.bin", &progress)
	buf := make([]byte, 16*1024)
	for {
		_, err := reader.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Count(strings.TrimSpace(progress.String()), "\n") + 1
	if lines > 105 {
		t.Fatalf("progress emitted %d lines, want at most 105", lines)
	}
	if !strings.Contains(progress.String(), "100%") {
		t.Fatalf("progress missing completion: %q", progress.String())
	}
}

func TestReadOnlyClientRejectsFileManager(t *testing.T) {
	client := Client{Config: config.Config{ReadOnly: true}}
	err := client.FileManager(context.Background(), "delete", []map[string]string{{"path": "/apps/baiduyunStorage/a.txt"}})
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("FileManager err=%v want ErrReadOnly", err)
	}
}
