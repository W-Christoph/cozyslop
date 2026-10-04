package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"cozycast/internal/hub"
)

const (
	sendBuffer   = 256
	writeTimeout = 10 * time.Second
	pingInterval = 20 * time.Second

	// Close code for "the server ended this on purpose"; the browser does
	// not reconnect.
	statusKicked websocket.StatusCode = 4000
	// Close code for a browser that fell behind; it reconnects.
	statusOverflow = websocket.StatusTryAgainLater
)

// Shutdown closes room sockets and waits for their handlers, including Leave.
// HTTP listeners must be shut down first: http.Server does not wait for
// hijacked connections.
func (s *Server) Shutdown(ctx context.Context) error {
	s.socketMu.Lock()
	s.stopSockets()
	s.socketMu.Unlock()
	done := make(chan struct{})
	go func() {
		s.socketWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) roomSocket(w http.ResponseWriter, r *http.Request) {
	s.socketMu.Lock()
	if s.socketCtx.Err() != nil {
		s.socketMu.Unlock()
		writeError(w, http.StatusServiceUnavailable, "Server is shutting down.")
		return
	}
	s.socketWG.Add(1)
	s.socketMu.Unlock()
	defer s.socketWG.Done()

	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(s.socketCtx, cancel)
	defer stop()
	defer cancel()
	r = r.WithContext(ctx)

	rm := s.hub.Room(r.PathValue("room"))
	if rm == nil {
		http.NotFound(w, r)
		return
	}
	// Identify before the upgrade so a fresh anonymous cookie is set on the
	// handshake response.
	id, err := s.auth.Identify(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	conn, err := websocket.Accept(w, r, nil) // same-origin only
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(64 << 10)

	sock := newSocket(conn)
	go sock.writeLoop(ctx, cancel)

	client, err := rm.Join(ctx, hub.JoinRequest{
		Identity:   id,
		AccessCode: r.URL.Query().Get("access"),
		Send:       sock.send,
		Kill:       sock.kill,
	})
	var denied *hub.DeniedError
	if errors.As(err, &denied) {
		sock.send(map[string]any{"type": "kicked", "reason": denied.Denial.Reason, "bannedUntil": denied.Denial.BannedUntil})
		sock.kill()
		<-ctx.Done()
		return
	}
	if err != nil {
		s.log.Error("join room", "room", rm.Name, "err", err)
		conn.Close(websocket.StatusInternalError, "")
		return
	}
	defer rm.Leave(context.WithoutCancel(ctx), client)

	go rm.SendNekoToken(ctx, client)

	for {
		var msg hub.ClientMsg
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return
		}
		rm.Handle(ctx, client, msg)
	}
}

// socket queues outgoing messages so the hub never blocks on a slow browser.
type socket struct {
	conn    *websocket.Conn
	out     chan any
	closing chan struct{}
	once    sync.Once
	code    websocket.StatusCode // set before closing is closed
}

func newSocket(conn *websocket.Conn) *socket {
	return &socket{conn: conn, out: make(chan any, sendBuffer), closing: make(chan struct{})}
}

func (s *socket) send(msg any) {
	select {
	case <-s.closing:
		return // nothing is queued after the reason for closing
	default:
	}
	select {
	case s.out <- msg:
	default:
		// A browser that cannot keep up is dropped rather than allowed to
		// stall the room; it reconnects and gets a fresh state.
		s.close(statusOverflow)
	}
}

// kill closes the connection for good, after the messages queued so far
// (the reason, usually) are written.
func (s *socket) kill() { s.close(statusKicked) }

func (s *socket) close(code websocket.StatusCode) {
	s.once.Do(func() {
		s.code = code
		close(s.closing)
	})
}

func (s *socket) writeLoop(ctx context.Context, cancel context.CancelFunc) {
	defer cancel()
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.out:
			if s.write(ctx, msg) != nil {
				return
			}
		case <-s.closing:
			if s.code == statusKicked {
				s.drain(ctx)
			}
			s.conn.Close(s.code, "")
			return
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, writeTimeout)
			err := s.conn.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		}
	}
}

// drain writes what is queued, within one write timeout in total: a browser
// that reads slowly cannot keep a closing connection open.
func (s *socket) drain(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	for {
		select {
		case msg := <-s.out:
			if wsjson.Write(ctx, s.conn, msg) != nil {
				return
			}
		default:
			return
		}
	}
}

func (s *socket) write(ctx context.Context, msg any) error {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return wsjson.Write(wctx, s.conn, msg)
}
