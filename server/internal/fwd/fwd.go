// Package fwd forwards connections and datagrams for rooms on other
// computers (docs/home-hosting.md): the server forwards each paired room's
// media port into the tunnel, and the agent forwards from the tunnel to the
// room (and the room's proxy connections the other way).
package fwd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Dial opens the other end of one forwarded connection or flow.
type Dial func(ctx context.Context) (net.Conn, error)

// TCP hands every connection accepted on l to a connection from dial,
// until ctx ends; then it closes l and all connections.
func TCP(ctx context.Context, l net.Listener, dial Dial) {
	stop := context.AfterFunc(ctx, func() { l.Close() })
	defer stop()
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
			// Either side ending ends both (the deferred closes stop the
			// other copy), and so does ctx.
			stop := context.AfterFunc(ctx, func() { c.Close(); up.Close() })
			defer stop()
			done := make(chan struct{}, 2)
			go func() { io.Copy(up, c); done <- struct{}{} }()
			go func() { io.Copy(c, up); done <- struct{}{} }()
			<-done
		}()
	}
}

// MaxFlows bounds the senders forwarded at once on one port.
const MaxFlows = 1024

// UDP forwards datagrams arriving on pc: each sender gets its own flow from
// dial, and what comes back on it goes to that sender. Media is UDP, and
// WebRTC keeps its flows busy (keepalives at least every few seconds); a
// flow quiet for idle ends. When ctx ends, pc and every flow are closed.
func UDP(ctx context.Context, pc net.PacketConn, dial Dial, idle time.Duration) {
	type flow struct {
		conn net.Conn
		last atomic.Int64 // unix ns of the last datagram either way
	}
	var mu sync.Mutex
	flows := map[string]*flow{}
	stop := context.AfterFunc(ctx, func() {
		pc.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, f := range flows {
			f.conn.Close()
		}
	})
	defer stop()
	buf := make([]byte, 64<<10)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
				continue // e.g. an ICMP error for an earlier reply
			}
			return
		}
		key := from.String()
		mu.Lock()
		f := flows[key]
		if f == nil && len(flows) < MaxFlows && ctx.Err() == nil {
			dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			conn, err := dial(dialCtx)
			cancel()
			if err == nil {
				f = &flow{conn: conn}
				flows[key] = f
				go func() {
					defer func() {
						mu.Lock()
						delete(flows, key)
						mu.Unlock()
						conn.Close()
					}()
					back := make([]byte, 64<<10)
					for {
						conn.SetReadDeadline(time.Now().Add(idle))
						n, err := conn.Read(back)
						if err != nil {
							var ne net.Error
							if errors.As(err, &ne) && ne.Timeout() && time.Since(time.Unix(0, f.last.Load())) < idle {
								continue // the sender is still talking
							}
							return
						}
						f.last.Store(time.Now().UnixNano())
						pc.WriteTo(back[:n], from)
					}
				}()
			}
		}
		mu.Unlock()
		if f == nil {
			continue
		}
		f.last.Store(time.Now().UnixNano())
		f.conn.Write(buf[:n])
	}
}

// FreePort is the first port in [first, last] not in used.
func FreePort(first, last int, used []int) (int, bool) {
	taken := map[int]bool{}
	for _, p := range used {
		taken[p] = true
	}
	for p := first; p <= last; p++ {
		if !taken[p] {
			return p, true
		}
	}
	return 0, false
}

// Ports forwards whole ports, UDP and TCP, for as long as they are open.
// The server uses it for each paired room's media port.
type Ports struct {
	Host string // listen address; "" means every interface
	// Dial opens the other end, for network "tcp" or "udp".
	Dial func(ctx context.Context, network, address string) (net.Conn, error)

	mu   sync.Mutex
	open map[string]func() // by name: closes it
}

// Open forwards port (UDP and TCP) to target under name, replacing what was
// open under name before.
func (p *Ports) Open(name string, port int, target string) error {
	p.Close(name)
	addr := net.JoinHostPort(p.Host, fmt.Sprint(port))
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		pc.Close()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	go UDP(ctx, pc, func(ctx context.Context) (net.Conn, error) { return p.Dial(ctx, "udp", target) }, time.Minute)
	go TCP(ctx, l, func(ctx context.Context) (net.Conn, error) { return p.Dial(ctx, "tcp", target) })
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open == nil {
		p.open = map[string]func(){}
	}
	// The ports are free again once Close returns.
	p.open[name] = func() { cancel(); pc.Close(); l.Close() }
	return nil
}

// Close stops forwarding name's port.
func (p *Ports) Close(name string) {
	p.mu.Lock()
	close := p.open[name]
	delete(p.open, name)
	p.mu.Unlock()
	if close != nil {
		close()
	}
}
