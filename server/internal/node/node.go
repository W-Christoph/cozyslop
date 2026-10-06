// Package node is the agent on a computer that runs a room for a CozyCast
// server elsewhere (docs/home-hosting.md). It pairs with the server once,
// showing a code an admin accepts, then keeps a WireGuard tunnel to it up
// for good: the server reaches the room's neko through the tunnel, and the
// room gets its settings from the server through it.
package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cozycast/internal/egress"
	"cozycast/internal/pairing"
	"cozycast/internal/tunnel"
)

type Config struct {
	Hub      string // the server: "cozy.example.com", or a URL like "http://192.0.2.1:8080"
	Name     string // proposed room name
	StateDir string // keys and pairing, kept across restarts
	EnvFile  string // the room's settings, for its container
	// Forward maps ports on this computer's tunnel address to where the
	// room listens (neko 8080, title 8081, play 8082).
	Forward map[int]string
	// ProxyListen is where the room's apps find their HTTP proxy, e.g.
	// ":3128"; it is carried to the server's, the room's only way out.
	ProxyListen string
	Out         io.Writer // messages for the person running it

	CheckEvery time.Duration // how often the server is asked for settings; 15 s
	RetryEvery time.Duration // waits after failures; 5 s
}

// state is what the agent remembers in StateDir.
type state struct {
	Hub        string `json:"hub"` // base URL that worked
	PrivateKey string `json:"privateKey"`
	HubKey     string `json:"hubKey,omitempty"`
	TunnelPort int    `json:"tunnelPort,omitempty"`
	Address    string `json:"address,omitempty"`
	HubAddress string `json:"hubAddress,omitempty"`
	Network    string `json:"network,omitempty"`
	Room       string `json:"room,omitempty"`
	Rejected   bool   `json:"rejected,omitempty"`
}

// ErrRejected: an admin said no. The agent remembers it, so that a
// restart does not ask again; deleting its state does.
var ErrRejected = errors.New("an admin rejected this computer; delete the node's state to ask again")

func (c *Config) say(format string, args ...any) {
	fmt.Fprintf(c.Out, format+"\n", args...)
}

// Run pairs if needed, then keeps the tunnel up until ctx ends.
func Run(ctx context.Context, cfg Config) error {
	if cfg.CheckEvery == 0 {
		cfg.CheckEvery = 15 * time.Second
	}
	if cfg.RetryEvery == 0 {
		cfg.RetryEvery = 5 * time.Second
	}
	if cfg.Out == nil {
		cfg.Out = os.Stdout
	}
	st, err := loadState(cfg.StateDir)
	if err != nil {
		return err
	}
	if st.Rejected {
		return ErrRejected
	}
	if st.Address == "" {
		if err := pair(ctx, &cfg, &st); err != nil {
			return err
		}
	}
	return connect(ctx, &cfg, st)
}

func statePath(dir string) string { return filepath.Join(dir, "node.json") }

// loadState reads the remembered state, creating a key on first start.
func loadState(dir string) (state, error) {
	var st state
	data, err := os.ReadFile(statePath(dir))
	if err == nil {
		err = json.Unmarshal(data, &st)
		return st, err
	}
	if !errors.Is(err, os.ErrNotExist) {
		return st, err
	}
	key, err := tunnel.GenerateKey()
	if err != nil {
		return st, err
	}
	st.PrivateKey = key.String()
	return st, saveState(dir, st)
}

func saveState(dir string, st state) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(statePath(dir), data)
}

// writeFile replaces path atomically, readable by its owner only.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// hubURLs are the addresses to try: as given, or HTTPS before plain HTTP.
func hubURLs(hub string) []string {
	hub = strings.TrimRight(strings.TrimSpace(hub), "/")
	if strings.Contains(hub, "://") {
		return []string{hub}
	}
	return []string{"https://" + hub, "http://" + hub}
}

type pairAnswer struct {
	ID         string `json:"id"`
	Secret     string `json:"secret"`
	HubKey     string `json:"hubKey"`
	Nonce      string `json:"nonce"`
	TunnelPort int    `json:"tunnelPort"`
	Error      string `json:"error"`
}

type pairStatus struct {
	Status     string `json:"status"`
	Room       string `json:"room"`
	Address    string `json:"address"`
	HubAddress string `json:"hubAddress"`
	Network    string `json:"network"`
	Error      string `json:"error"`
}

// pair asks the server until an admin answers.
func pair(ctx context.Context, cfg *Config, st *state) error {
	if cfg.Hub == "" {
		return errors.New("set COZYCAST_HUB to the server's address, e.g. cozy.example.com")
	}
	key, err := tunnel.ParseKey(st.PrivateKey)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 40 * time.Second}
	for {
		base, ans, err := requestPairing(ctx, client, cfg, key.Public())
		if err != nil {
			return err
		}
		hubKey, err := tunnel.ParseKey(ans.HubKey)
		nonce, nerr := base64.StdEncoding.DecodeString(ans.Nonce)
		if err != nil || nerr != nil {
			return errors.New("the server sent an invalid pairing answer")
		}
		cfg.say("Pairing with %s as %q.", base, cfg.Name)
		if strings.HasPrefix(base, "http://") {
			cfg.say("Warning: %s is plain HTTP. Compare the code carefully;\nanyone between you and the server could otherwise pose as it.", base)
		}
		// Computed here, from the keys as this computer received them.
		cfg.say("Code: %s", pairing.Code(hubKey, key.Public(), nonce))
		cfg.say("Ask an admin to accept this code under Admin > Rooms. Waiting...")
		status, err := waitPairing(ctx, client, cfg, base, ans)
		if err != nil {
			return err
		}
		switch status.Status {
		case "accepted":
			st.Hub, st.HubKey, st.TunnelPort = base, ans.HubKey, ans.TunnelPort
			st.Address, st.HubAddress, st.Network, st.Room = status.Address, status.HubAddress, status.Network, status.Room
			if err := saveState(cfg.StateDir, *st); err != nil {
				return err
			}
			cfg.say("Accepted as room %q.", status.Room)
			return nil
		case "rejected":
			st.Rejected = true
			if err := saveState(cfg.StateDir, *st); err != nil {
				return err
			}
			return ErrRejected
		}
		cfg.say("The request expired; asking again.")
	}
}

// requestPairing files the request, trying HTTPS before plain HTTP and
// retrying while the server cannot be reached.
func requestPairing(ctx context.Context, client *http.Client, cfg *Config, pub tunnel.Key) (string, pairAnswer, error) {
	body, _ := json.Marshal(map[string]string{"name": cfg.Name, "nodeKey": pub.String()})
	for {
		var lastErr error
		for _, base := range hubURLs(cfg.Hub) {
			var ans pairAnswer
			status, err := doJSON(ctx, client, http.MethodPost, base+"/api/nodes/pair", "", body, &ans)
			if err != nil {
				lastErr = err
				continue
			}
			switch {
			case status == http.StatusCreated:
				return base, ans, nil
			case status == http.StatusTooManyRequests:
				lastErr = errors.New(ans.Error)
			default:
				return "", ans, fmt.Errorf("the server refused: %s", ans.Error)
			}
		}
		cfg.say("Cannot reach %s (%v); retrying.", cfg.Hub, lastErr)
		if err := sleep(ctx, cfg.RetryEvery*6); err != nil {
			return "", pairAnswer{}, err
		}
	}
}

func waitPairing(ctx context.Context, client *http.Client, cfg *Config, base string, ans pairAnswer) (pairStatus, error) {
	for {
		var st pairStatus
		code, err := doJSON(ctx, client, http.MethodGet, base+"/api/nodes/pair/"+url.PathEscape(ans.ID), ans.Secret, nil, &st)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return st, ctx.Err()
			}
			cfg.say("Lost the server (%v); still waiting.", err)
			if err := sleep(ctx, cfg.RetryEvery); err != nil {
				return st, err
			}
		case code == http.StatusNotFound:
			// Forgotten (the server restarted): ask again.
			return pairStatus{Status: "expired"}, nil
		case code != http.StatusOK:
			return st, fmt.Errorf("the server refused: %s", st.Error)
		case st.Status != "pending":
			return st, nil
		}
	}
}

func doJSON(ctx context.Context, client *http.Client, method, url, bearer string, body []byte, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out); err != nil {
		return res.StatusCode, fmt.Errorf("not a CozyCast server? %w", err)
	}
	return res.StatusCode, nil
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// connect keeps the tunnel up and the room's settings current.
func connect(ctx context.Context, cfg *Config, st state) error {
	key, err := tunnel.ParseKey(st.PrivateKey)
	if err != nil {
		return err
	}
	hubKey, err := tunnel.ParseKey(st.HubKey)
	if err != nil {
		return err
	}
	addr, err := netip.ParseAddr(st.Address)
	if err != nil {
		return err
	}
	hubAddr, err := netip.ParseAddr(st.HubAddress)
	if err != nil {
		return err
	}
	network, err := netip.ParsePrefix(st.Network)
	if err != nil {
		return err
	}
	tun, err := tunnel.Open(tunnel.Config{PrivateKey: key, Address: addr, Network: network})
	if err != nil {
		return err
	}
	defer tun.Close()
	peer := tunnel.Peer{Key: hubKey, Address: hubAddr, Keepalive: 25 * time.Second}
	setEndpoint := func() {
		ep, err := hubEndpoint(ctx, st)
		if err != nil {
			cfg.say("Cannot look up %s (%v); retrying.", st.Hub, err)
			return
		}
		if ep != peer.Endpoint {
			peer.Endpoint = ep
			if err := tun.SetPeer(peer); err != nil {
				cfg.say("Tunnel: %v", err)
			}
		}
	}
	setEndpoint()
	for port, target := range cfg.Forward {
		l, err := tun.Listen(port)
		if err != nil {
			return err
		}
		defer l.Close()
		go forward(ctx, l, func(ctx context.Context) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", target)
		})
	}
	if cfg.ProxyListen != "" {
		l, err := net.Listen("tcp", cfg.ProxyListen)
		if err != nil {
			return err
		}
		defer l.Close()
		egressAddr := net.JoinHostPort(hubAddr.String(), strconv.Itoa(egress.Port))
		go forward(ctx, l, func(ctx context.Context) (net.Conn, error) {
			return tun.DialContext(ctx, "tcp", egressAddr)
		})
	}

	// A new boot ID tells the server this agent started anew, so that it
	// drops connections through the old tunnel at once.
	boot := make([]byte, 8)
	rand.Read(boot)
	configURL := "http://" + net.JoinHostPort(hubAddr.String(), "80") + "/node/config?boot=" + hex.EncodeToString(boot)
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DialContext: tun.DialContext}}
	failures, connected := 0, false
	var lostSince time.Time
	var env string
	for {
		var res struct {
			Room      string `json:"room"`
			NekoToken string `json:"nekoToken"`
			Error     string `json:"error"`
		}
		// The check is also what brings the tunnel back after the server
		// restarted: WireGuard only renews a dead session for real data.
		code, err := doJSON(ctx, client, http.MethodGet, configURL, "", nil, &res)
		switch {
		case ctx.Err() != nil:
			return nil
		case err == nil && code == http.StatusOK:
			if !connected {
				cfg.say("Connected to %s as room %q.", st.Hub, res.Room)
			}
			connected, failures, lostSince = true, 0, time.Time{}
			next := fmt.Sprintf("COZYCAST_ROOM='%s'\nCOZYCAST_NEKO_TOKEN='%s'\n", res.Room, res.NekoToken)
			if next != env {
				if env != "" {
					cfg.say("The room's settings changed; restart the room container.")
				}
				if err := writeFile(cfg.EnvFile, []byte(next)); err != nil {
					return err
				}
				env = next
			}
		default:
			if err == nil {
				err = errors.New(res.Error)
			}
			failures++
			if lostSince.IsZero() {
				lostSince = time.Now()
				cfg.say("No contact with the server (%v); retrying.", err)
			}
			connected = false
			// The server may have moved: look its address up again.
			if failures%4 == 0 {
				setEndpoint()
			}
			if failures%20 == 0 {
				cfg.say("No contact with the server since %s. If this computer was removed there, delete the node's state and start it again to pair anew.", lostSince.Format(time.DateTime))
			}
		}
		if err := sleep(ctx, cfg.CheckEvery); err != nil {
			return nil
		}
	}
}

// hubEndpoint is the server's WireGuard address: its host name looked up,
// with the tunnel port.
func hubEndpoint(ctx context.Context, st state) (netip.AddrPort, error) {
	u, err := url.Parse(st.Hub)
	if err != nil {
		return netip.AddrPort{}, err
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", u.Hostname())
	if err != nil {
		return netip.AddrPort{}, err
	}
	if len(ips) == 0 {
		return netip.AddrPort{}, errors.New("no address")
	}
	// Prefer IPv4: home connections and VPS firewalls handle it most often.
	ip := ips[0]
	for _, candidate := range ips {
		if candidate.Unmap().Is4() {
			ip = candidate.Unmap()
			break
		}
	}
	return netip.AddrPortFrom(ip, uint16(st.TunnelPort)), nil
}

// forward hands every connection on l to a connection from dial, until
// ctx ends.
func forward(ctx context.Context, l net.Listener, dial func(context.Context) (net.Conn, error)) {
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			up, err := dial(dialCtx)
			cancel()
			if err != nil {
				return
			}
			defer up.Close()
			// Stopping the agent ends its connections too.
			stop := context.AfterFunc(ctx, func() { c.Close(); up.Close() })
			defer stop()
			// Either side ending ends both (the deferred closes stop the
			// other copy).
			done := make(chan struct{}, 2)
			go func() { io.Copy(up, c); done <- struct{}{} }()
			go func() { io.Copy(c, up); done <- struct{}{} }()
			<-done
		}()
	}
}

// PortsFromEnv parses "8080=room:8080,8081=room:8081".
func PortsFromEnv(s string) (map[int]string, error) {
	m := map[int]string{}
	for _, part := range strings.Split(s, ",") {
		port, target, ok := strings.Cut(strings.TrimSpace(part), "=")
		p, err := strconv.Atoi(port)
		if !ok || err != nil || p < 1 || p > 65535 || target == "" {
			return nil, fmt.Errorf("invalid forward %q (want port=host:port)", part)
		}
		m[p] = target
	}
	return m, nil
}
