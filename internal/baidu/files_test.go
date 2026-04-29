package baidu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
