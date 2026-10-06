// Package egress is the way out to the internet for rooms on other
// computers (docs/home-hosting.md). Such a room has no network but the one
// to its agent; its apps use an HTTP proxy there, which the agent carries
// through the tunnel to this proxy on the server. Websites therefore see the
// server's address, host names are looked up here, and nothing reaches
// private networks: neither the home's nor the server's.
package egress

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

// Port is where the proxy listens on the server's tunnel address.
const Port = 3128

var errDenied = errors.New("egress: destination not allowed")

// Not public, though Go does not flag them: carrier-grade NAT, the IETF
// protocol block, benchmarking, documentation and reserved ranges.
var special = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// Public reports whether ip is an ordinary internet address: not loopback,
// private, link-local (cloud metadata lives there), multicast or otherwise
// special.
func Public(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, p := range special {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// Proxy is an HTTP proxy (CONNECT, and plain HTTP with absolute URLs).
type Proxy struct {
	// Allow decides which addresses may be reached; nil means Public.
	Allow func(netip.Addr) bool
	// BlockedPorts are never reached, e.g. 25: mail from a VPS invites
	// abuse complaints.
	BlockedPorts map[uint16]bool
	Resolver     *net.Resolver
	Log          *slog.Logger

	once      sync.Once
	transport *http.Transport
}

// New is the proxy with the defaults: public addresses, no port 25.
func New() *Proxy {
	return &Proxy{BlockedPorts: map[uint16]bool{25: true}}
}

func (p *Proxy) allow(ip netip.Addr) bool {
	if p.Allow != nil {
		return p.Allow(ip)
	}
	return Public(ip)
}

// Dial connects to host:port if one of its addresses is allowed. The
// address checked is the address dialed, so a name cannot resolve to a
// public address for the check and a private one for the connection.
func (p *Proxy) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 || p.BlockedPorts[uint16(port)] {
		return nil, errDenied
	}
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip}
	} else {
		resolver := p.Resolver
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		if ips, err = resolver.LookupNetIP(ctx, "ip", host); err != nil {
			return nil, err
		}
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	lastErr := errDenied
	for _, ip := range ips {
		if !p.allow(ip) {
			continue
		}
		c, err := d.DialContext(ctx, "tcp", netip.AddrPortFrom(ip.Unmap(), uint16(port)).String())
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.connect(w, r)
		return
	}
	if !r.URL.IsAbs() || r.URL.Scheme != "http" {
		http.Error(w, "this is a proxy", http.StatusBadRequest)
		return
	}
	p.once.Do(func() {
		p.transport = &http.Transport{DialContext: p.Dial, MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second}
	})
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL = r.URL
			pr.Out.Host = r.URL.Host
		},
		Transport: p.transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			p.refuse(w, r.Host, err)
		},
	}
	rp.ServeHTTP(w, r)
}

func (p *Proxy) connect(w http.ResponseWriter, r *http.Request) {
	up, err := p.Dial(r.Context(), "tcp", r.Host)
	if err != nil {
		p.refuse(w, r.Host, err)
		return
	}
	defer up.Close()
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "cannot tunnel", http.StatusInternalServerError)
		return
	}
	down, buf, err := hj.Hijack()
	if err != nil {
		return
	}
	defer down.Close()
	if _, err := io.WriteString(down, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	splice(down, buf.Reader, up)
}

// splice copies both ways until either side ends; buffered holds what the
// client sent after its request.
func splice(down net.Conn, buffered *bufio.Reader, up net.Conn) {
	done := make(chan struct{}, 2)
	go func() { io.Copy(up, buffered); done <- struct{}{} }()
	go func() { io.Copy(down, up); done <- struct{}{} }()
	<-done
}

func (p *Proxy) refuse(w http.ResponseWriter, host string, err error) {
	if errors.Is(err, errDenied) {
		if p.Log != nil {
			p.Log.Info("egress refused", "host", host)
		}
		http.Error(w, "destination not allowed", http.StatusForbidden)
		return
	}
	http.Error(w, "cannot reach "+host, http.StatusBadGateway)
}
