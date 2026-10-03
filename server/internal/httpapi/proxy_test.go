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

	tests := []struct {
		path       string
		wantStatus int
		wantPath   string
	}{
		{"/neko/default/api/ws?token=abc", http.StatusOK, "/api/ws"},
		{"/neko/default/api/filetransfer/x", http.StatusOK, "/api/filetransfer/x"},
		{"/neko/default/api/members", http.StatusNotFound, ""},
		{"/neko/default/api/room/upload/drop", http.StatusNotFound, ""},
		{"/neko/default/api/wsx", http.StatusNotFound, ""},
		{"/neko/other/api/ws", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		gotPath, gotQuery, gotOrigin = "", "", ""
		req, _ := http.NewRequest(http.MethodGet, srv.URL+tt.path, nil)
		req.Header.Set("Origin", "https://evil.example")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tt.wantStatus {
			t.Errorf("%s: status %d, want %d", tt.path, res.StatusCode, tt.wantStatus)
		}
		if gotPath != tt.wantPath {
			t.Errorf("%s: upstream path %q, want %q", tt.path, gotPath, tt.wantPath)
		}
		if tt.path == "/neko/default/api/ws?token=abc" && gotQuery != "token=abc" {
			t.Errorf("query not forwarded: %q", gotQuery)
		}
		if tt.wantStatus == http.StatusOK && gotOrigin != "" {
			t.Errorf("%s: Origin forwarded: %q", tt.path, gotOrigin)
		}
	}
}
