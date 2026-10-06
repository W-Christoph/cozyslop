package egress_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"cozycast/internal/egress"
)

func TestPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"93.184.216.34": true, "1.1.1.1": true, "2606:4700::1111": true,
		"127.0.0.1": false, "::1": false, "10.77.0.1": false, "172.17.0.1": false, "192.168.1.1": false,
		"169.254.169.254": false, "fe80::1": false, "fd00::1": false, "100.64.0.1": false,
		"0.0.0.0": false, "224.0.0.1": false, "255.255.255.255": false, "203.0.113.5": false,
		"::ffff:192.168.1.1": false, "::ffff:1.1.1.1": true,
	} {
		if got := egress.Public(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Public(%s) = %v", addr, got)
		}
	}
}

// A proxy that may reach the test's loopback "internet" only.
func proxy(t *testing.T, allowed string) *httptest.Server {
	p := egress.New()
	p.Allow = func(ip netip.Addr) bool { return ip.String() == allowed }
	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)
	return srv
}

func TestPlainHTTPAndRefusals(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello %s", r.URL.Path)
	}))
	defer site.Close()
	srv := proxy(t, "127.0.0.1")
	proxyURL, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	res, err := client.Get(site.URL + "/page")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "hello /page" {
		t.Fatalf("through the proxy: %d %q", res.StatusCode, body)
	}
	// Not allowed: another address (the "home network"), and port 25.
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(site.URL, "http://"))
	for _, target := range []string{"http://127.0.0.2:" + port + "/", "http://127.0.0.1:25/"} {
		res, err := client.Get(target)
		if err != nil || res.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %v %v", target, res.StatusCode, err)
		}
		res.Body.Close()
	}
	// Not a proxy request at all.
	res, err = http.Get(srv.URL + "/")
	if err != nil || res.StatusCode != http.StatusBadRequest {
		t.Fatalf("direct request: %v %v", res.StatusCode, err)
	}
}

func TestConnect(t *testing.T) {
	site := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "secret page") }))
	defer site.Close()
	srv := proxy(t, "127.0.0.1")
	proxyURL, _ := url.Parse(srv.URL)
	transport := site.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	res, err := (&http.Client{Transport: transport}).Get(site.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "secret page" {
		t.Fatalf("CONNECT: %q", body)
	}

	// Refused before anything is tunneled.
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "CONNECT 169.254.169.254:80 HTTP/1.1\r\nHost: 169.254.169.254:80\r\n\r\n")
	reply, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil || reply.StatusCode != http.StatusForbidden {
		t.Fatalf("metadata address: %v %v", reply, err)
	}
}

// A name is checked by the address it resolves to, and that address is the
// one dialed.
func TestNamesAreCheckedByAddress(t *testing.T) {
	p := egress.New()
	p.Allow = func(ip netip.Addr) bool { return false }
	if _, err := p.Dial(context.Background(), "tcp", "localhost:80"); err == nil {
		t.Fatal("dialed a name that resolves to loopback")
	}
}
