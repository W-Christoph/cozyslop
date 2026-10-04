// Package auth handles passwords, login sessions and anonymous identities.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"cozycast/internal/store"
)

const (
	sessionCookie = "cozy_session"
	anonCookie    = "cozy_anon"
	anonCookieTTL = 365 * 24 * time.Hour
	bcryptCost    = 10 // matches CozyCast, so imported hashes cost the same to check
)

var ErrInvalidCredentials = errors.New("invalid username or password")

func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < 8 || len(password) > 72 {
		return errors.New("Passwords must be at least 8 characters and at most 72 bytes.")
	}
	return nil
}

// Identity is who is making a request: an account, or an anonymous browser.
type Identity struct {
	User        *store.User // nil for anonymous
	AnonID      string      // always set; stable per browser, derived from the cozy_anon cookie
	IP          string
	SessionHash []byte `json:"-"` // account session's stored hash; never sent to clients
}

// Key groups all connections of one person: "u:<id>" or "a:<anon id>".
func (i Identity) Key() string {
	if i.User != nil {
		return "u:" + strconv.FormatInt(i.User.ID, 10)
	}
	return "a:" + i.AnonID
}

type Service struct {
	store      *store.Store
	trustProxy bool
	dummyHash  []byte // compared against when the user does not exist, to keep timing equal
}

// New returns the auth service. With trustProxy, the client IP and scheme
// come from X-Forwarded-For / X-Forwarded-Proto.
func New(s *store.Store, trustProxy bool) *Service {
	dummy, err := bcrypt.GenerateFromPassword([]byte("dummy password"), bcryptCost)
	if err != nil {
		panic(err)
	}
	return &Service{store: s, trustProxy: trustProxy, dummyHash: dummy}
}

func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(h), err
}

// Identify resolves the request's identity and makes sure the browser has an
// anonymous id cookie. A missing or invalid session is not an error.
func (a *Service) Identify(w http.ResponseWriter, r *http.Request) (Identity, error) {
	id := Identity{IP: a.ClientIP(r)}

	if c, err := r.Cookie(anonCookie); err == nil && validToken(c.Value) {
		id.AnonID = anonID(c.Value)
	} else {
		token := newToken()
		id.AnonID = anonID(token)
		a.setCookie(w, r, anonCookie, token, anonCookieTTL)
	}

	if c, err := r.Cookie(sessionCookie); err == nil && validToken(c.Value) {
		hash := hashToken(c.Value)
		u, extended, err := a.store.SessionUser(r.Context(), hash)
		switch {
		case err == nil:
			id.User = u
			id.SessionHash = hash
			if extended {
				a.setCookie(w, r, sessionCookie, c.Value, store.SessionTTL)
			}
		case errors.Is(err, store.ErrNotFound):
			a.clearCookie(w, r, sessionCookie)
		default:
			return id, err
		}
	}
	return id, nil
}

// Login checks credentials and starts a session.
func (a *Service) Login(w http.ResponseWriter, r *http.Request, username, password string) (*store.User, error) {
	u, err := a.store.UserByUsername(r.Context(), username)
	if errors.Is(err, store.ErrNotFound) {
		bcrypt.CompareHashAndPassword(a.dummyHash, []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !CheckPassword(u, password) || u.Disabled {
		return nil, ErrInvalidCredentials
	}
	return u, a.StartSession(w, r, u)
}

// LegacyLogin starts a session for a browser that was logged in to the old
// CozyCast and still holds its refresh token. Each token works once.
func (a *Service) LegacyLogin(w http.ResponseWriter, r *http.Request, token string) (*store.User, error) {
	if token == "" || len(token) > 512 {
		return nil, ErrInvalidCredentials
	}
	u, err := a.store.RedeemLegacyLogin(r.Context(), legacyTokenHashes(token)...)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	return u, a.StartSession(w, r, u)
}

// legacyTokenHashes returns the hashes a refresh token of the old server may
// be stored under. That server kept a random key and gave the browser the
// key signed (a compact JWS with the key as payload), so the key's hash
// comes first; the token as presented is the fallback. The signature is not
// checked: knowing the key is what proves the login.
func legacyTokenHashes(token string) [][]byte {
	var hashes [][]byte
	if parts := strings.Split(token, "."); len(parts) == 3 {
		if key, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil && len(key) > 0 {
			hashes = append(hashes, hashToken(string(key)))
		}
	}
	return append(hashes, hashToken(token))
}

// CheckPassword reports whether password is the user's current password.
func CheckPassword(u *store.User, password string) bool {
	return len(password) <= 72 && bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}

func (a *Service) StartSession(w http.ResponseWriter, r *http.Request, u *store.User) error {
	token := newToken()
	if err := a.store.CreateSession(r.Context(), hashToken(token), u.ID); err != nil {
		return err
	}
	a.setCookie(w, r, sessionCookie, token, store.SessionTTL)
	return nil
}

// Logout ends the request's session and returns its hash for live revocation.
func (a *Service) Logout(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	a.clearCookie(w, r, sessionCookie)
	if c, err := r.Cookie(sessionCookie); err == nil {
		hash := hashToken(c.Value)
		return hash, a.store.DeleteSession(r.Context(), hash)
	}
	return nil, nil
}

// LogoutOthers ends every session of u except the request's own, returning
// the hash to keep for live revocation.
func (a *Service) LogoutOthers(ctx context.Context, r *http.Request, u *store.User) ([]byte, error) {
	var keep []byte
	if c, err := r.Cookie(sessionCookie); err == nil {
		keep = hashToken(c.Value)
	}
	return keep, a.store.DeleteUserSessions(ctx, u.ID, keep)
}

// ClientIP is the address rate limits and anonymous bans apply to.
func (a *Service) ClientIP(r *http.Request) string {
	if a.trustProxy {
		if values := r.Header.Values("X-Forwarded-For"); len(values) > 0 {
			fwd := values[len(values)-1]
			last := fwd[strings.LastIndex(fwd, ",")+1:]
			if ip, err := netip.ParseAddr(strings.TrimSpace(last)); err == nil {
				return ip.Unmap().String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *Service) secure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return a.trustProxy && r.Header.Get("X-Forwarded-Proto") == "https"
}

func (a *Service) setCookie(w http.ResponseWriter, r *http.Request, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   a.secure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *Service) clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.secure(r), SameSite: http.SameSiteLaxMode,
	})
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// validToken rejects junk before it reaches the database.
func validToken(s string) bool {
	if len(s) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// anonID is the public id of an anonymous browser. Everyone in a room sees
// it (as "a:<anon id>"), so it must not be the cookie itself: knowing it is
// not enough to become that person.
func anonID(token string) string {
	h := sha256.Sum256([]byte("cozycast anon id:" + token))
	return base64.RawURLEncoding.EncodeToString(h[:16])
}
