package httpapi

import (
	"net/http"
	"net/http/httputil"
	"strings"
)

// nekoPaths are the neko endpoints browsers may reach. Everything else
// (member management, room settings, neko's own UI) stays private, even
// though neko would also reject non-admin sessions on its own.
// Uploads go through the file transfer plugin, which neko gates by the
// member's upload right; neko's drop/dialog uploads only check remote
// control, so they are not exposed.
var nekoPaths = []string{
	"api/ws",
	"api/filetransfer",
}

func nekoPathAllowed(p string) bool {
	for _, allowed := range nekoPaths {
		if p == allowed || strings.HasPrefix(p, allowed+"/") {
			return true
		}
	}
	return false
}

func (s *Server) nekoProxy(w http.ResponseWriter, r *http.Request) {
	rm := s.hub.Room(r.PathValue("room"))
	path := r.PathValue("path")
	if rm == nil || !nekoPathAllowed(path) {
		http.NotFound(w, r)
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
