package neko_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cozycast/internal/neko"
)

// After Reconnect, a request stuck on a dead connection gives up at once
// instead of waiting for its timeout.
func TestReconnectEndsRequestsInFlight(t *testing.T) {
	stuck := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(stuck)
		<-r.Context().Done()
	}))
	defer srv.Close()
	var d net.Dialer
	c, err := neko.NewClient(srv.URL, "secret", d.DialContext)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan bool, 1)
	start := time.Now()
	go func() { done <- c.Healthy(context.Background()) }()
	<-stuck
	c.Reconnect()
	select {
	case healthy := <-done:
		if healthy || time.Since(start) > 5*time.Second {
			t.Fatalf("healthy=%v after %v", healthy, time.Since(start))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request not ended by Reconnect")
	}
	// Later requests work again.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	if !c.Healthy(context.Background()) {
		t.Fatal("client unusable after Reconnect")
	}
}
