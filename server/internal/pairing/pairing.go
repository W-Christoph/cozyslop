// Package pairing lets a computer at someone's home ask to run a room
// (docs/home-hosting.md, "Pairing"). The computer sends its WireGuard public
// key and shows a code; an admin who sees the same code on the admin page
// accepts it. Requests live in memory only: after a server restart the
// computer simply asks again.
package pairing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"slices"
	"sync"
	"time"

	"cozycast/internal/tunnel"
)

// Code is what both sides show: 40 bits of a hash over both public keys and
// the request's nonce, so a key swapped on the way gives different codes.
func Code(hubKey, nodeKey tunnel.Key, nonce []byte) string {
	h := sha256.New()
	h.Write([]byte("cozycast-pair"))
	h.Write(hubKey[:])
	h.Write(nodeKey[:])
	h.Write(nonce)
	s := crockford.EncodeToString(h.Sum(nil)[:5])
	return s[:4] + "-" + s[4:]
}

// Crockford's base32: no I, L, O or U, easy to read out loud.
var crockford = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

const (
	TTL       = 10 * time.Minute // a request waits this long for an admin
	keep      = 10 * time.Minute // an answered request is kept for the computer to collect
	MaxTotal  = 20
	MaxPerIP  = 3
	nonceSize = 16
)

var (
	ErrTooMany = errors.New("pairing: too many pending requests")
	ErrUnknown = errors.New("pairing: unknown request")
	ErrDone    = errors.New("pairing: request already answered")
)

type State string

const (
	Pending  State = "pending"
	Accepted State = "accepted"
	Rejected State = "rejected"
	Expired  State = "expired"
)

// Request is one computer asking to be paired.
type Request struct {
	ID      string
	Name    string // proposed room name
	NodeKey tunnel.Key
	Nonce   []byte
	Code    string
	IP      string
	Created time.Time
	Expires time.Time
	State   State
	Room    string // set when accepted
	claimed bool   // an admin's accept is being carried out
	secret  string
	changed chan struct{} // closed and replaced on every state change
}

type Manager struct {
	hubKey tunnel.Key
	now    func() time.Time

	mu   sync.Mutex
	byID map[string]*Request
}

func NewManager(hubKey tunnel.Key) *Manager {
	return &Manager{hubKey: hubKey, now: time.Now, byID: make(map[string]*Request)}
}

// Create files a request, or renews the pending one of the same key (a
// computer that restarted while waiting). The secret lets the computer, and
// only it, collect the answer.
func (m *Manager) Create(name string, nodeKey tunnel.Key, ip string) (Request, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	m.sweepLocked(now)
	pending, fromIP := 0, 0
	for _, r := range m.byID {
		if r.State != Pending {
			continue
		}
		if r.NodeKey == nodeKey && !r.claimed {
			r.Name, r.IP, r.Expires = name, ip, now.Add(TTL)
			return *r, r.secret, nil
		}
		pending++
		if r.IP == ip {
			fromIP++
		}
	}
	if pending >= MaxTotal || fromIP >= MaxPerIP {
		return Request{}, "", ErrTooMany
	}
	nonce := make([]byte, nonceSize)
	rand.Read(nonce)
	r := &Request{
		ID: randomHex(8), Name: name, NodeKey: nodeKey, Nonce: nonce, IP: ip,
		Code: Code(m.hubKey, nodeKey, nonce), Created: now, Expires: now.Add(TTL),
		State: Pending, secret: randomHex(16), changed: make(chan struct{}),
	}
	m.byID[r.ID] = r
	return *r, r.secret, nil
}

// Wait returns the request once it is answered or expired, or when ctx
// ends, whichever is first.
func (m *Manager) Wait(ctx context.Context, id, secret string) (Request, error) {
	for {
		m.mu.Lock()
		r, ok := m.byID[id]
		if !ok || subtle.ConstantTimeCompare([]byte(secret), []byte(r.secret)) != 1 {
			m.mu.Unlock()
			return Request{}, ErrUnknown
		}
		m.expireLocked(r, m.now())
		view, changed := *r, r.changed
		m.mu.Unlock()
		if view.State != Pending {
			return view, nil
		}
		// At least a moment: an expired request being accepted stays pending.
		timer := time.NewTimer(max(view.Expires.Sub(m.now()), 0) + 100*time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return view, nil
		case <-changed:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// Pending lists the requests waiting for an admin, oldest first.
func (m *Manager) Pending() []Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(m.now())
	list := []Request{}
	for _, r := range m.byID {
		if r.State == Pending {
			list = append(list, *r)
		}
	}
	slices.SortFunc(list, func(a, b Request) int { return a.Created.Compare(b.Created) })
	return list
}

// Claim reserves a pending request for an admin's accept; Finish or Release
// must follow.
func (m *Manager) Claim(id string) (Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.byID[id]
	if !ok {
		return Request{}, ErrUnknown
	}
	m.expireLocked(r, m.now())
	if r.State != Pending || r.claimed {
		return Request{}, ErrDone
	}
	r.claimed = true
	return *r, nil
}

// Release gives a claimed request back, e.g. after a failed accept.
func (m *Manager) Release(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.byID[id]; ok {
		r.claimed = false
	}
}

// Finish answers a claimed request as accepted into room.
func (m *Manager) Finish(id, room string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.byID[id]; ok {
		r.claimed, r.Room = false, room
		m.setLocked(r, Accepted)
	}
}

// Reject answers a pending request with no.
func (m *Manager) Reject(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.byID[id]
	if !ok {
		return ErrUnknown
	}
	m.expireLocked(r, m.now())
	if r.State != Pending || r.claimed {
		return ErrDone
	}
	m.setLocked(r, Rejected)
	return nil
}

func (m *Manager) setLocked(r *Request, s State) {
	r.State = s
	r.Expires = m.now().Add(keep) // now: how long the answer is kept
	close(r.changed)
	r.changed = make(chan struct{})
}

func (m *Manager) expireLocked(r *Request, now time.Time) {
	if r.State == Pending && !r.claimed && now.After(r.Expires) {
		m.setLocked(r, Expired)
	}
}

// sweepLocked expires pending requests and forgets answered ones the
// computer had time to collect.
func (m *Manager) sweepLocked(now time.Time) {
	for id, r := range m.byID {
		if r.State != Pending && now.After(r.Expires) {
			delete(m.byID, id)
			continue
		}
		m.expireLocked(r, now)
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
