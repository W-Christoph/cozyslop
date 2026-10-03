package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/coder/websocket"

	"cozycast/internal/hub"
	"cozycast/internal/neko"
)

// nekoRequestAllowed lists the neko endpoints browsers may reach. Everything else
// (member management, room settings, neko's own UI) stays private, even
// though neko would also reject non-admin sessions on its own.
// Uploads go through the file transfer plugin, which neko gates by the
// member's upload right; neko's drop/dialog uploads only check remote
// control, so they are not exposed. The same plugin serves the desktop's
// Downloads folder on GET, to anyone with the upload right: only POST (an
// upload) is let through.
func nekoRequestAllowed(method, p string) bool {
	switch {
	case p == "api/ws":
		return method == http.MethodGet
	case p == "api/filetransfer":
		return method == http.MethodPost
	}
	return false
}

func (s *Server) nekoProxy(w http.ResponseWriter, r *http.Request) {
	rm := s.hub.Room(r.PathValue("room"))
	path := r.PathValue("path")
	if rm == nil || !nekoRequestAllowed(r.Method, path) {
		http.NotFound(w, r)
		return
	}
	if path == "api/ws" {
		s.nekoSocket(w, r, rm)
		return
	}
	// neko takes the token of any of its members; only the ones the hub
	// handed out are let through (see hub.Room.NekoTokenIssued).
	if !rm.NekoTokenIssued(r.URL.Query().Get("token")) {
		writeError(w, http.StatusForbidden, "Your connection to the room's desktop has expired.")
		return
	}

	target := rm.Neko().BaseURL()
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = strings.TrimSuffix(target.Path, "/") + "/" + path
			pr.Out.URL.RawPath = ""
			// neko would reject the browser's Origin (it does not know our
			// domain). Dropping it is safe: neko authenticates with the
			// per-member token in the URL, not a cookie, so a cross-site page
			// cannot ride on someone else's session.
			pr.Out.Header.Del("Origin")
		},
	}
	proxy.ServeHTTP(w, r)
}

const (
	nekoDialTimeout = 10 * time.Second
	// neko's largest messages: the room state on connect, and session
	// descriptions and clipboard text from the browser.
	nekoReadLimit    = 4 << 20
	browserReadLimit = 1 << 20
)

// nekoSocket carries a tab's WebSocket to neko. It is not a plain pipe:
// neko lets every session choose its own capture pipeline, and each
// pipeline someone watches is one more encoder running. A room has one
// stream, so the messages that choose one are rewritten on the way (see
// neko.PinStream), and the hub moves the connection along when the room's
// stream changes.
func (s *Server) nekoSocket(w http.ResponseWriter, r *http.Request, rm *hub.Room) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	token := r.URL.Query().Get("token")
	fromHub := make(chan []byte, 8)
	detach, ok := rm.AttachNeko(token, &hub.NekoConn{
		Send: func(msg []byte) {
			select {
			case fromHub <- msg:
			default: // a connection this far behind is about to fail anyway
			}
		},
		Close: cancel,
	})
	if !ok {
		writeError(w, http.StatusForbidden, "Your connection to the room's desktop has expired.")
		return
	}
	defer detach()

	dialCtx, stop := context.WithTimeout(ctx, nekoDialTimeout)
	up, _, err := websocket.Dial(dialCtx, rm.Neko().SocketURL(token), nil)
	stop()
	if err != nil {
		writeError(w, http.StatusBadGateway, "The room's desktop is not reachable right now.")
		return
	}
	defer up.CloseNow()
	down, err := websocket.Accept(w, r, nil) // same-origin only
	if err != nil {
		return
	}
	defer down.CloseNow()
	up.SetReadLimit(nekoReadLimit)
	down.SetReadLimit(browserReadLimit)

	write := func(to *websocket.Conn, typ websocket.MessageType, data []byte) error {
		wctx, cancel := context.WithTimeout(ctx, writeTimeout)
		defer cancel()
		return to.Write(wctx, typ, data)
	}
	// Each direction ends with the error that stopped it; the other side is
	// then closed the way this one was.
	ended := make(chan error, 2)
	go func() {
		for {
			typ, data, err := up.Read(ctx)
			if err == nil {
				err = write(down, typ, data)
			}
			if err != nil {
				ended <- err
				return
			}
		}
	}()
	go func() {
		for {
			_, data, err := down.Read(ctx)
			if err != nil {
				ended <- err
				return
			}
			// neko reads text and binary frames alike.
			if data, ok := neko.PinStream(data, rm.Stream()); ok {
				if err := write(up, websocket.MessageText, data); err != nil {
					ended <- err
					return
				}
			}
		}
	}()
	for {
		select {
		case msg := <-fromHub:
			if err := write(up, websocket.MessageText, msg); err != nil {
				return
			}
		case err := <-ended:
			code, reason := websocket.StatusGoingAway, ""
			var closed websocket.CloseError
			if errors.As(err, &closed) && closed.Code != websocket.StatusNoStatusRcvd {
				code, reason = closed.Code, closed.Reason
			}
			go up.Close(code, reason)
			down.Close(code, reason)
			return
		case <-ctx.Done(): // the tab left the room or was kicked
			return
		}
	}
}
