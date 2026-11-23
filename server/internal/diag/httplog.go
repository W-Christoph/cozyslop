package diag

import (
	"log"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
)

// OpenConns counts the HTTP servers' open connections (ConnState); a
// connection taken over by a WebSocket no longer counts.
var OpenConns atomic.Int64

// ConnState is an http.Server's ConnState, for OpenConns.
func ConnState(_ net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		OpenConns.Add(1)
	case http.StateHijacked, http.StateClosed:
		OpenConns.Add(-1)
	}
}

// TLSHandshakeErrors counts failed TLS handshakes since the start. Scanners
// and old clients cause plenty; each would be a log line with their address.
var TLSHandshakeErrors atomic.Int64

// ServerErrorLog is an http.Server's ErrorLog: its messages go to l (whose
// handler scrubs them), except failed TLS handshakes, which are counted.
func ServerErrorLog(l *slog.Logger) *log.Logger {
	return log.New(serverErrors{l}, "", 0)
}

type serverErrors struct{ log *slog.Logger }

func (w serverErrors) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if strings.Contains(msg, "TLS handshake error") {
		TLSHandshakeErrors.Add(1)
	} else {
		w.log.Warn("http server: " + msg)
	}
	return len(p), nil
}
