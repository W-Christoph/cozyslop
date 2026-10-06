// Package tunnel is the server's end of the WireGuard tunnels to rooms on
// other machines (docs/home-hosting.md). It runs in the process, in
// userspace (wireguard-go on gVisor's network stack): no root, kernel module
// or network interface on the host, only one UDP port. The server reaches a
// room's neko through it like through any other network.
package tunnel

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// Key is a WireGuard (Curve25519) key, private or public.
type Key [32]byte

// GenerateKey returns a new private key.
func GenerateKey() (Key, error) {
	var k Key
	if _, err := rand.Read(k[:]); err != nil {
		return k, err
	}
	// Clamp, as WireGuard does.
	k[0] &= 248
	k[31] = (k[31] & 127) | 64
	return k, nil
}

// ParseKey reads a key in WireGuard's base64 form.
func ParseKey(s string) (Key, error) {
	var k Key
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != len(k) {
		return k, errors.New("tunnel: invalid key")
	}
	copy(k[:], b)
	return k, nil
}

// String is the key in WireGuard's base64 form.
func (k Key) String() string { return base64.StdEncoding.EncodeToString(k[:]) }

// Public is the public key of a private key.
func (k Key) Public() Key {
	var pub Key
	out, _ := curve25519.X25519(k[:], curve25519.Basepoint)
	copy(pub[:], out)
	return pub
}

// LoadKey reads the private key at path, creating it on first use. Losing
// it means every node has to pair again.
func LoadKey(path string) (Key, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return ParseKey(string(data))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Key{}, err
	}
	k, err := GenerateKey()
	if err != nil {
		return k, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return k, err
	}
	// O_EXCL: never overwrite a key another process just wrote.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return k, err
	}
	_, err = f.WriteString(k.String() + "\n")
	return k, errors.Join(err, f.Close())
}

// Peer is the other end of one tunnel.
type Peer struct {
	Key     Key
	Address netip.Addr // its one address inside the tunnel
	// Where to reach it. A node always sets the hub's; the hub learns each
	// node's from its packets and only restores the last one it saw.
	Endpoint netip.AddrPort
	// Keepalive keeps a NAT mapping open; set by the side behind the NAT.
	Keepalive time.Duration
}

// Config describes this end.
type Config struct {
	PrivateKey Key
	Port       int          // UDP; 0 picks a free one
	Address    netip.Addr   // this end's address inside the tunnel
	Network    netip.Prefix // the tunnel's addresses; dials outside it fail
}

type Tunnel struct {
	dev     *device.Device
	net     *netstack.Net
	cfg     Config
	port    int
	mu      sync.Mutex
	closed  bool
	address netip.Addr
}

// Open starts this end of the tunnel with no peers.
func Open(cfg Config) (*Tunnel, error) {
	if !cfg.Network.Contains(cfg.Address) {
		return nil, fmt.Errorf("tunnel: address %s is outside %s", cfg.Address, cfg.Network)
	}
	tdev, tnet, err := netstack.CreateNetTUN([]netip.Addr{cfg.Address}, nil, device.DefaultMTU)
	if err != nil {
		return nil, err
	}
	log := slog.With("component", "wireguard")
	dev := device.NewDevice(tdev, conn.NewDefaultBind(), &device.Logger{
		Verbosef: func(string, ...any) {},
		Errorf: func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			// Expected for every computer that is off: the room shows offline.
			if !strings.Contains(msg, "no known endpoint") {
				log.Warn(msg)
			}
		},
	})
	t := &Tunnel{dev: dev, net: tnet, cfg: cfg, address: cfg.Address}
	err = dev.IpcSet(fmt.Sprintf("private_key=%s\nlisten_port=%d\n", hex.EncodeToString(cfg.PrivateKey[:]), cfg.Port))
	if err == nil {
		err = dev.Up()
	}
	if err == nil {
		t.port, err = t.listenPort()
	}
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("tunnel: %w", err)
	}
	return t, nil
}

// PublicKey is this end's public key, which peers need.
func (t *Tunnel) PublicKey() Key { return t.cfg.PrivateKey.Public() }

// Address is this end's address inside the tunnel.
func (t *Tunnel) Address() netip.Addr { return t.address }

// Network is the tunnel's address range.
func (t *Tunnel) Network() netip.Prefix { return t.cfg.Network }

// Port is the UDP port the tunnel listens on.
func (t *Tunnel) Port() int { return t.port }

// SetPeer adds a peer, or replaces the one with the same key.
func (t *Tunnel) SetPeer(p Peer) error {
	if !t.cfg.Network.Contains(p.Address) || p.Address == t.address {
		return fmt.Errorf("tunnel: peer address %s is not usable in %s", p.Address, t.cfg.Network)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "public_key=%s\nreplace_allowed_ips=true\nallowed_ip=%s\n",
		hex.EncodeToString(p.Key[:]), netip.PrefixFrom(p.Address, p.Address.BitLen()))
	if p.Endpoint.IsValid() {
		fmt.Fprintf(&b, "endpoint=%s\n", p.Endpoint)
	}
	if p.Keepalive > 0 {
		fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", int(p.Keepalive/time.Second))
	}
	return t.dev.IpcSet(b.String())
}

// RemovePeer cuts a peer off at once. Removing an unknown key does nothing.
func (t *Tunnel) RemovePeer(key Key) error {
	return t.dev.IpcSet(fmt.Sprintf("public_key=%s\nremove=true\n", hex.EncodeToString(key[:])))
}

// HasPeer reports whether key is a peer.
func (t *Tunnel) HasPeer(key Key) bool {
	want, found := hex.EncodeToString(key[:]), false
	_ = t.ipcGet(func(k, v string) {
		if k == "public_key" && v == want {
			found = true
		}
	})
	return found
}

// LastHandshake is when the peer last completed a handshake; zero if never
// (or if the peer is unknown). WireGuard renews it at least every two
// minutes while packets flow, and keepalives make them flow.
func (t *Tunnel) LastHandshake(key Key) time.Time {
	var sec, nsec int64
	want := hex.EncodeToString(key[:])
	current := false
	_ = t.ipcGet(func(k, v string) {
		switch k {
		case "public_key":
			current = v == want
		case "last_handshake_time_sec":
			if current {
				sec, _ = strconv.ParseInt(v, 10, 64)
			}
		case "last_handshake_time_nsec":
			if current {
				nsec, _ = strconv.ParseInt(v, 10, 64)
			}
		}
	})
	if sec == 0 && nsec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, nsec)
}

// Endpoint is where the peer's packets last came from; invalid if none yet.
// The hub stores it: after a restart it can start the handshake itself,
// instead of waiting minutes for the node to notice (WireGuard only renews a
// session that stopped answering when it has real data to send).
func (t *Tunnel) Endpoint(key Key) netip.AddrPort {
	var ep netip.AddrPort
	want := hex.EncodeToString(key[:])
	current := false
	_ = t.ipcGet(func(k, v string) {
		switch k {
		case "public_key":
			current = v == want
		case "endpoint":
			if current {
				ep, _ = netip.ParseAddrPort(v)
			}
		}
	})
	return ep
}

// DialContext connects to an address inside the tunnel ("10.77.0.2:8080").
func (t *Tunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return nil, fmt.Errorf("tunnel: dial %s: an address inside the tunnel is needed", address)
	}
	if !t.cfg.Network.Contains(ap.Addr()) {
		return nil, fmt.Errorf("tunnel: dial %s: outside %s", address, t.cfg.Network)
	}
	return t.net.DialContext(ctx, network, address)
}

// Owns reports whether rawURL points into the tunnel, so that it must be
// reached through it. False for a nil Tunnel (tunnels off).
func (t *Tunnel) Owns(rawURL string) bool {
	if t == nil {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	addr, err := netip.ParseAddr(u.Hostname())
	return err == nil && t.cfg.Network.Contains(addr.Unmap())
}

// ListenUDP receives datagrams on this end's tunnel address.
func (t *Tunnel) ListenUDP(port int) (net.PacketConn, error) {
	return t.net.ListenUDPAddrPort(netip.AddrPortFrom(t.address, uint16(port)))
}

// Listen accepts TCP connections on this end's tunnel address.
func (t *Tunnel) Listen(port int) (net.Listener, error) {
	return t.net.ListenTCPAddrPort(netip.AddrPortFrom(t.address, uint16(port)))
}

// Close stops the tunnel; open connections through it fail.
func (t *Tunnel) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		t.dev.Close()
	}
	return nil
}

func (t *Tunnel) listenPort() (int, error) {
	port := -1
	err := t.ipcGet(func(k, v string) {
		if k == "listen_port" {
			port, _ = strconv.Atoi(v)
		}
	})
	if err == nil && port < 0 {
		err = errors.New("no listen port")
	}
	return port, err
}

func (t *Tunnel) ipcGet(each func(key, value string)) error {
	text, err := t.dev.IpcGet()
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			each(k, v)
		}
	}
	return sc.Err()
}

// NextAddress is the first address of network that is neither the network's
// own, nor taken by this end (hub) or one of used.
func NextAddress(network netip.Prefix, hub netip.Addr, used []netip.Addr) (netip.Addr, error) {
	taken := map[netip.Addr]bool{hub: true}
	for _, a := range used {
		taken[a] = true
	}
	network = network.Masked()
	for a := network.Addr().Next(); network.Contains(a); a = a.Next() {
		// Skip the IPv4 broadcast address too.
		if !network.Contains(a.Next()) && a.Is4() {
			break
		}
		if !taken[a] {
			return a, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("tunnel: no free address left in %s", network)
}
