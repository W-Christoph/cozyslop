package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cozycast/internal/diag"
)

func TestTrackRequests(t *testing.T) {
	d, err := diag.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Server{diagnostics: d}
	inside := make(chan []diag.Request, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/admin/users/{username}", func(w http.ResponseWriter, r *http.Request) {
		inside <- s.Requests()
		time.Sleep(slowRequest + 50*time.Millisecond)
	})
	mux.HandleFunc("POST /api/rooms/{room}/media", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "broken", http.StatusInternalServerError)
	})
	mux.HandleFunc("GET /fast", func(w http.ResponseWriter, r *http.Request) {})
	h := s.track(mux)
	for _, r := range []*http.Request{
		httptest.NewRequest("GET", "/api/admin/users/alice?x=secret", nil),
		httptest.NewRequest("POST", "/api/rooms/default/media", nil),
		httptest.NewRequest("GET", "/fast", nil),
		httptest.NewRequest("GET", "/nothing/here", nil),
	} {
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
	if open := <-inside; len(open) != 1 || open[0].Route != "GET /api/admin/users/{username}" || open[0].Long {
		t.Errorf("under way: %+v", open)
	}
	if open := s.Requests(); len(open) != 0 {
		t.Errorf("still under way: %+v", open)
	}
	d.Sync()
	list, err := d.Entries(context.Background(), diag.Filter{Kinds: []string{diag.KindRequest}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("entries: %+v", list)
	}
	failed, slow := list[0], list[1]
	if failed.Msg != "POST /api/rooms/{room}/media" || failed.Room != "default" || failed.Attrs["status"] != float64(500) {
		t.Errorf("failed: %+v", failed)
	}
	if slow.Msg != "GET /api/admin/users/{username}" || slow.Attrs["status"] != float64(0) && slow.Attrs["status"] != float64(200) ||
		slow.Attrs["duration_ms"].(float64) < float64(slowRequest.Milliseconds()) {
		t.Errorf("slow: %+v", slow)
	}
	if s.reqs.total.Load() != 4 {
		t.Errorf("total %d", s.reqs.total.Load())
	}
}
