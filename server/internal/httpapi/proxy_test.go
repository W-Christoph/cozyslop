package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cozycast/internal/hub"
	"cozycast/internal/neko"
)

func TestNekoProxy(t *testing.T) {
	var gotPath, gotQuery, gotOrigin string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotOrigin = r.URL.Path, r.URL.RawQuery, r.Header.Get("Origin")
	}))
	defer upstream.Close()

	nc, err := neko.NewClient(upstream.URL, "secret")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(Deps{Hub: hub.New(nil, "", []hub.RoomConfig{{Name: "default", Neko: nc}})}).Handler())
	defer srv.Close()

	const get, post = http.MethodGet, http.MethodPost
	tests := []struct {
		method     string
		path       string
		wantStatus int
		wantPath   string
	}{
		{get, "/neko/default/api/ws?token=abc", http.StatusOK, "/api/ws"},
		{post, "/neko/default/api/filetransfer?token=abc", http.StatusOK, "/api/filetransfer"},
		// Downloads from the desktop are not offered.
		{get, "/neko/default/api/filetransfer?filename=x", http.StatusNotFound, ""},
		{get, "/neko/default/api/filetransfer/x", http.StatusNotFound, ""},
		{post, "/neko/default/api/filetransfer/x", http.StatusNotFound, ""},
		{get, "/neko/default/api/members", http.StatusNotFound, ""},
		{post, "/neko/default/api/members", http.StatusNotFound, ""},
		{get, "/neko/default/api/room/upload/drop", http.StatusNotFound, ""},
		{get, "/neko/default/api/wsx", http.StatusNotFound, ""},
		{get, "/neko/other/api/ws", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		gotPath, gotQuery, gotOrigin = "", "", ""
		req, _ := http.NewRequest(tt.method, srv.URL+tt.path, nil)
		// Other sites may only read; neko never sees the Origin either way.
		req.Header.Set("Origin", "https://evil.example")
		if tt.method == post {
			req.Header.Set("Origin", srv.URL)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tt.wantStatus {
			t.Errorf("%s %s: status %d, want %d", tt.method, tt.path, res.StatusCode, tt.wantStatus)
		}
		if gotPath != tt.wantPath {
			t.Errorf("%s %s: upstream path %q, want %q", tt.method, tt.path, gotPath, tt.wantPath)
		}
		if tt.path == "/neko/default/api/ws?token=abc" && gotQuery != "token=abc" {
			t.Errorf("query not forwarded: %q", gotQuery)
		}
		if tt.wantStatus == http.StatusOK && gotOrigin != "" {
			t.Errorf("%s: Origin forwarded: %q", tt.path, gotOrigin)
		}
	}
}
