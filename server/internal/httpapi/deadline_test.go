package httpapi

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBodyDeadline(t *testing.T) {
	old := bodyTimeout
	bodyTimeout = 200 * time.Millisecond
	t.Cleanup(func() { bodyTimeout = old })
	srv := httptest.NewServer(bodyDeadline(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "body: "+err.Error(), http.StatusRequestTimeout)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	t.Cleanup(srv.Close)

	request := func(head, body string) int {
		t.Helper()
		conn, err := net.Dial("tcp", srv.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if _, err := io.WriteString(conn, head+"\r\nHost: x\r\n\r\n"+body); err != nil {
			t.Fatal(err)
		}
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		res, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatal("no answer to an unfinished body:", err)
		}
		return res.StatusCode
	}
	// Half a body, then silence: the handler's read fails at the deadline.
	if got := request("POST /api/auth/login HTTP/1.1\r\nContent-Length: 10", "12345"); got != http.StatusRequestTimeout {
		t.Fatal("unfinished body:", got)
	}
	if got := request("POST /api/auth/login HTTP/1.1\r\nContent-Length: 5", "12345"); got != http.StatusNoContent {
		t.Fatal("complete body:", got)
	}
	// No body, no deadline: this is what a WebSocket upgrade looks like.
	if got := request("GET /api/rooms/default/ws HTTP/1.1", ""); got != http.StatusNoContent {
		t.Fatal("no body:", got)
	}
}
