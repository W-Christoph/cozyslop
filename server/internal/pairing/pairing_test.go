package pairing

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"cozycast/internal/tunnel"
)

func key(t *testing.T) tunnel.Key {
	t.Helper()
	k, err := tunnel.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k.Public()
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }
func manager(t *testing.T) (*Manager, *clock) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	m := NewManager(key(t))
	m.now = c.now
	return m, c
}

func TestCode(t *testing.T) {
	hub, node := key(t), key(t)
	nonce := []byte("0123456789abcdef")
	code := Code(hub, node, nonce)
	if !regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}$`).MatchString(code) {
		t.Fatalf("code %q", code)
	}
	if Code(hub, node, nonce) != code {
		t.Fatal("not deterministic")
	}
	// Any swapped part changes it.
	for _, other := range []string{Code(key(t), node, nonce), Code(hub, key(t), nonce), Code(hub, node, []byte("another nonce..."))} {
		if other == code {
			t.Fatal("collision")
		}
	}
}

func TestRequestLifecycle(t *testing.T) {
	m, c := manager(t)
	node := key(t)
	req, secret, err := m.Create("home", node, "203.0.113.1")
	if err != nil {
		t.Fatal(err)
	}
	if req.Code != Code(m.hubKey, node, req.Nonce) || req.State != Pending || secret == "" {
		t.Fatalf("request: %+v", req)
	}
	// The same computer asking again (it restarted) gets the same request.
	c.add(time.Minute)
	again, secret2, err := m.Create("renamed", node, "203.0.113.1")
	if err != nil || again.ID != req.ID || secret2 != secret || again.Name != "renamed" || !again.Expires.After(req.Expires) {
		t.Fatalf("renewal: %+v %v", again, err)
	}
	if list := m.Pending(); len(list) != 1 || list[0].Code != req.Code {
		t.Fatalf("pending: %+v", list)
	}
	if _, err := m.Wait(context.Background(), req.ID, "wrong"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("wrong secret: %v", err)
	}

	// A waiting computer hears the answer as soon as it is given.
	answer := make(chan Request, 1)
	go func() {
		r, _ := m.Wait(context.Background(), req.ID, secret)
		answer <- r
	}()
	claimed, err := m.Claim(req.ID)
	if err != nil || claimed.NodeKey != node {
		t.Fatal(err)
	}
	if _, err := m.Claim(req.ID); !errors.Is(err, ErrDone) {
		t.Fatal("claimed twice")
	}
	if err := m.Reject(req.ID); !errors.Is(err, ErrDone) {
		t.Fatal("rejected while being accepted")
	}
	m.Release(req.ID)
	if _, err := m.Claim(req.ID); err != nil {
		t.Fatal("not claimable after release:", err)
	}
	m.Finish(req.ID, "home")
	select {
	case r := <-answer:
		if r.State != Accepted || r.Room != "home" {
			t.Fatalf("answer: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiting computer not told")
	}
	if len(m.Pending()) != 0 {
		t.Fatal("answered request still pending")
	}
	// The answer can be collected for a while, then it is forgotten.
	if r, err := m.Wait(context.Background(), req.ID, secret); err != nil || r.State != Accepted {
		t.Fatalf("collect: %+v %v", r, err)
	}
	c.add(keep + time.Second)
	m.Pending()
	if _, err := m.Wait(context.Background(), req.ID, secret); !errors.Is(err, ErrUnknown) {
		t.Fatal("kept forever")
	}
}

func TestRejectAndExpire(t *testing.T) {
	m, c := manager(t)
	rejected, secret, _ := m.Create("a", key(t), "198.51.100.1")
	if err := m.Reject(rejected.ID); err != nil {
		t.Fatal(err)
	}
	if r, _ := m.Wait(context.Background(), rejected.ID, secret); r.State != Rejected {
		t.Fatalf("rejected: %+v", r)
	}
	if err := m.Reject("missing"); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}

	late, secret, _ := m.Create("b", key(t), "198.51.100.1")
	c.add(TTL + time.Second)
	if len(m.Pending()) != 0 {
		t.Fatal("expired request listed")
	}
	if r, err := m.Wait(context.Background(), late.ID, secret); err != nil || r.State != Expired {
		t.Fatalf("expired: %+v %v", r, err)
	}
	if _, err := m.Claim(late.ID); !errors.Is(err, ErrDone) {
		t.Fatal("expired request accepted")
	}

	// A wait ends with its context while still pending.
	open, secret, _ := m.Create("c", key(t), "198.51.100.1")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if r, err := m.Wait(ctx, open.ID, secret); err != nil || r.State != Pending {
		t.Fatalf("long poll: %+v %v", r, err)
	}
}

func TestLimits(t *testing.T) {
	m, _ := manager(t)
	for i := range MaxPerIP {
		if _, _, err := m.Create(fmt.Sprint(i), key(t), "192.0.2.1"); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := m.Create("x", key(t), "192.0.2.1"); !errors.Is(err, ErrTooMany) {
		t.Fatal("per-IP limit not enforced")
	}
	for i := MaxPerIP; i < MaxTotal; i++ {
		if _, _, err := m.Create(fmt.Sprint(i), key(t), fmt.Sprintf("192.0.2.%d", 10+i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := m.Create("y", key(t), "192.0.2.200"); !errors.Is(err, ErrTooMany) {
		t.Fatal("total limit not enforced")
	}
}
