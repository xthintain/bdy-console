package baidu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"baiduyunStorage/internal/config"
)

func TestFileBlockMD5sSmallFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocks, whole, err := FileBlockMD5s(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "900150983cd24fb0d6963f7d28e17f72"
	if len(blocks) != 1 || blocks[0] != want || whole != want {
		t.Fatalf("blocks=%v whole=%s", blocks, whole)
	}
}

func TestReadOnlyClientRejectsUploadAndMkdir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := Client{Config: config.Config{ReadOnly: true}}
	if err := client.UploadFile(context.Background(), path, "/apps/baiduyunStorage/a.txt"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("UploadFile err=%v want ErrReadOnly", err)
	}
	if err := client.Mkdir(context.Background(), "/apps/baiduyunStorage/dir"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Mkdir err=%v want ErrReadOnly", err)
	}
}

func TestUploadFileUploadsPartsConcurrently(t *testing.T) {
	t.Setenv("BDY_UPLOAD_CONCURRENCY", "3")
	t.Setenv("BDY_UPLOAD_CHUNK_SIZE", "4M")
	var mu sync.Mutex
	active := 0
	maxActive := 0
	uploads := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/2.0/xpan/file":
			switch r.URL.Query().Get("method") {
			case "precreate":
				_ = json.NewEncoder(w).Encode(precreateResponse{UploadID: "up", BlockList: []int{0, 1, 2}})
			case "create":
				_ = json.NewEncoder(w).Encode(map[string]int{"errno": 0})
			default:
				t.Fatalf("unexpected xpan method %s", r.URL.Query().Get("method"))
			}
		case "/rest/2.0/pcs/superfile2":
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			active--
			uploads++
			mu.Unlock()
			_ = r.ParseMultipartForm(ChunkSize + 1024)
			_ = json.NewEncoder(w).Encode(uploadPartResponse{Errno: 0, MD5: "ok"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "parts.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(int64(ChunkSize * 3)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	client := Client{
		Config: config.Config{AccessToken: "tok"},
		HTTP:   server.Client(),
		PanURL: server.URL,
		PCSURL: server.URL,
	}
	if err := client.UploadFile(context.Background(), path, "/apps/baiduyunStorage/parts.bin"); err != nil {
		t.Fatal(err)
	}
	if uploads != 3 {
		t.Fatalf("uploads=%d want 3", uploads)
	}
	if maxActive < 2 {
		t.Fatalf("max active uploads=%d, expected concurrent uploads", maxActive)
	}
}

func TestUploadChunkSizeFromEnv(t *testing.T) {
	t.Setenv("BDY_UPLOAD_CHUNK_SIZE", "16M")
	size, ok := uploadChunkSizeFromEnv()
	if !ok || size != memberChunkSize {
		t.Fatalf("size=%d ok=%v want %d true", size, ok, memberChunkSize)
	}
	t.Setenv("BDY_UPLOAD_CHUNK_SIZE", "64M")
	size, ok = uploadChunkSizeFromEnv()
	if !ok || size != superVIPChunkSize {
		t.Fatalf("size=%d ok=%v want capped %d true", size, ok, superVIPChunkSize)
	}
}

func TestUploadChunkSizeUsesVIPType(t *testing.T) {
	t.Setenv("BDY_UPLOAD_CONCURRENCY", "4")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/2.0/xpan/nas" || r.URL.Query().Get("method") != "uinfo" {
			t.Fatalf("unexpected request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(UserInfo{Errno: 0, ErrMsg: "succ", VIPType: VIPSuper})
	}))
	defer server.Close()
	client := Client{Config: config.Config{AccessToken: "tok"}, HTTP: server.Client(), PanURL: server.URL}
	size := client.uploadChunkSize(context.Background(), 512*1024*1024)
	if size != superVIPChunkSize {
		t.Fatalf("chunk size=%d want %d", size, superVIPChunkSize)
	}
}

func TestAdaptiveUploadChunkSizeKeepsEnoughParts(t *testing.T) {
	tests := []struct {
		name     string
		fileSize int64
		maxSize  int64
		want     int64
	}{
		{name: "small super keeps 4m", fileSize: 64 * 1024 * 1024, maxSize: superVIPChunkSize, want: ChunkSize},
		{name: "medium super uses 16m", fileSize: 256 * 1024 * 1024, maxSize: superVIPChunkSize, want: memberChunkSize},
		{name: "large super uses 32m", fileSize: 512 * 1024 * 1024, maxSize: superVIPChunkSize, want: superVIPChunkSize},
		{name: "member caps 16m", fileSize: 512 * 1024 * 1024, maxSize: memberChunkSize, want: memberChunkSize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptiveUploadChunkSize(tt.fileSize, tt.maxSize, 4)
			if got != tt.want {
				t.Fatalf("chunk size=%d want %d", got, tt.want)
			}
		})
	}
}
