package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestDeviceCodeUsesDocumentedParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/2.0/device/code" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("User-Agent") != "pan.baidu.com" {
			t.Fatalf("missing user-agent")
		}
		q := r.URL.Query()
		if q.Get("response_type") != "device_code" || q.Get("client_id") != "app-key" || q.Get("scope") != "basic,netdisk" {
			t.Fatalf("bad query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(DeviceCode{DeviceCode: "dev", UserCode: "user", VerificationURL: "https://openapi.baidu.com/device", Interval: 5})
	}))
	defer server.Close()

	got, err := (OAuth{BaseURL: server.URL, HTTP: server.Client()}).RequestDeviceCode(context.Background(), "app-key")
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceCode != "dev" || got.UserCode != "user" {
		t.Fatalf("unexpected response: %+v", got)
	}
}
