package auth

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	for _, tc := range []struct {
		name    string
		trusted bool
		peer    string
		headers []string
		want    string
	}{
		{"untrusted", false, "192.0.2.1:123", []string{"203.0.113.1"}, "192.0.2.1"},
		{"last entry", true, "192.0.2.1:123", []string{"spoofed, 203.0.113.2"}, "203.0.113.2"},
		{"last header", true, "192.0.2.1:123", []string{"203.0.113.1", "spoofed, 203.0.113.2"}, "203.0.113.2"},
		{"IPv6", true, "192.0.2.1:123", []string{"spoofed,  2001:db8::1 "}, "2001:db8::1"},
		{"mapped IPv4", true, "192.0.2.1:123", []string{"::ffff:203.0.113.2"}, "203.0.113.2"},
		{"invalid", true, "192.0.2.1:123", []string{"203.0.113.1, invalid"}, "192.0.2.1"},
		{"with port", true, "192.0.2.1:123", []string{"203.0.113.1:123"}, "192.0.2.1"},
		{"empty last entry", true, "192.0.2.1:123", []string{"203.0.113.1,"}, "192.0.2.1"},
		{"empty last header", true, "192.0.2.1:123", []string{"203.0.113.1", ""}, "192.0.2.1"},
		{"no header", true, "[2001:db8::2]:123", nil, "2001:db8::2"},
		{"bare peer", true, "192.0.2.1", nil, "192.0.2.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.peer
			for _, value := range tc.headers {
				r.Header.Add("X-Forwarded-For", value)
			}
			a := &Service{trustProxy: tc.trusted}
			if got := a.ClientIP(r); got != tc.want {
				t.Fatalf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}
