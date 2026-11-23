package diag

import (
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

// MaxText is how long a scrubbed message or value may be.
const MaxText = 500

var (
	urlPattern   = regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'<>]+`)
	ipv4Pattern  = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	ipv6Pattern  = regexp.MustCompile(`\[?[0-9a-fA-F]{0,4}(?::[0-9a-fA-F]{0,4}){2,7}(?:%[0-9a-zA-Z]+)?(?:(?:\.\d{1,3}){3})?\]?`)
	emailPattern = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	// Tokens, keys and hashes: long runs of base64 or hex.
	tokenPattern = regexp.MustCompile(`[A-Za-z0-9+/_=-]{24,}`)
)

// Scrub removes from s what might say who or what someone does: public IP
// addresses, the path and query of URLs (where page addresses and tokens
// go), e-mail addresses and anything that looks like a token. It cuts s to
// MaxText bytes.
func Scrub(s string) string {
	s = urlPattern.ReplaceAllStringFunc(s, scrubURL)
	s = emailPattern.ReplaceAllString(s, "<email>")
	if strings.Count(s, ":") >= 2 { // first: IPv6 may end in IPv4
		s = ipv6Pattern.ReplaceAllStringFunc(s, scrubIP)
	}
	s = ipv4Pattern.ReplaceAllStringFunc(s, scrubIP)
	s = tokenPattern.ReplaceAllStringFunc(s, scrubToken)
	return cut(s, MaxText)
}

// ScrubIPs masks public IP addresses only, for text where other long
// strings mean something (goroutine dumps).
func ScrubIPs(s string) string {
	s = ipv6Pattern.ReplaceAllStringFunc(s, scrubIP)
	return ipv4Pattern.ReplaceAllStringFunc(s, scrubIP)
}

// scrubURL keeps a URL's scheme and host, and port: enough to say where a
// request went.
func scrubURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "<url>"
	}
	host := u.Host
	if h := u.Hostname(); h != "" {
		if scrubbed := scrubIP(h); scrubbed != h {
			host = strings.Replace(host, h, scrubbed, 1)
		}
	}
	out := u.Scheme + "://" + host
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" {
		out += "/…"
	}
	return out
}

// scrubIP masks s if it is a public IP address. Private, loopback and
// link-local ones (the server's, its rooms' and the tunnel's) say nothing
// about anyone and help troubleshooting.
func scrubIP(s string) string {
	inner := strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	if i := strings.IndexByte(inner, '%'); i >= 0 {
		inner = inner[:i]
	}
	ip, err := netip.ParseAddr(inner)
	if err != nil {
		return s
	}
	ip = ip.Unmap()
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() ||
		ip.IsMulticast() || ip.IsLinkLocalMulticast() {
		return s
	}
	return "<ip>"
}

// scrubToken masks s if it has digits and letters: words and paths of
// letters alone stay readable.
func scrubToken(s string) string {
	letters := strings.ContainsFunc(s, func(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' })
	digits := strings.ContainsFunc(s, func(r rune) bool { return r >= '0' && r <= '9' })
	if letters && digits {
		return "<redacted>"
	}
	return s
}

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8Start(s[n]) {
		n--
	}
	return s[:n] + "…"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
