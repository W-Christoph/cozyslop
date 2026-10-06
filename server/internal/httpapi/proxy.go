package httpapi

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"

	"cozycast/internal/hub"
	"cozycast/internal/neko"
)

// nekoRequestAllowed lists the neko endpoints browsers may reach. Everything else
// (member management, room settings, neko's own UI) stays private, even
// though neko would also reject non-admin sessions on its own.
// Files go through the file transfer plugin, which neko gates by the
// member's upload right: POST uploads into the desktop's Downloads folder,
// GET downloads a file from it (see asDownload). Its DELETE is not exposed.
// neko's drop/dialog uploads only check remote control, so they are not
// exposed either.
func nekoRequestAllowed(method, p string) bool {
	switch {
	case p == "api/ws":
		return method == http.MethodGet
	case p == "api/filetransfer":
		return method == http.MethodPost || method == http.MethodGet
	}
	return false
}

// asDownload makes the browser save a file from the room's desktop, never
// show it. The file arrives from this site's address: an HTML or SVG file
// displayed as a page would run its scripts with the visitor's login.
func asDownload(name string) func(*http.Response) error {
	return func(res *http.Response) error {
		h := res.Header
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "sandbox")
		h.Set("Cache-Control", "private, no-store")
		if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusPartialContent {
			disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
			if disposition == "" {
				disposition = "attachment"
			}
			h.Set("Content-Disposition", disposition)
			h.Set("Content-Type", "application/octet-stream")
		}
		return nil
	}
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
	token := r.URL.Query().Get("token")
	if !rm.NekoTokenIssued(token) {
		writeError(w, http.StatusForbidden, "Your connection to the room's desktop has expired.")
		return
	}
	download := r.Method == http.MethodGet
	if download && !rm.NekoFilesAllowed(token) {
		writeError(w, http.StatusForbidden, "You are not allowed to download files from the room.")
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	detach, ok := rm.AttachNeko(token, &hub.NekoConn{Send: func([]byte) {}, Close: cancel})
	if !ok {
		writeError(w, http.StatusForbidden, "Your connection to the room's desktop has expired.")
		return
	}
	defer detach()
	r = r.WithContext(ctx)
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
	if download {
		// neko serves the file of this name in the Downloads folder itself.
		proxy.ModifyResponse = asDownload(filepath.Base(filepath.Clean(r.URL.Query().Get("filename"))))
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
// stream changes. neko also sends every session the list of the files in
// the desktop's Downloads folder; only tabs with the upload right get it.
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
			if err == nil && (!neko.IsFileList(data) || rm.NekoFilesAllowed(token)) {
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
			if neko.IsFileList(data) && !rm.NekoFilesAllowed(token) {
				continue
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
