package fwd_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"cozycast/internal/fwd"
)

// udpEcho answers every datagram with "echo:" and the datagram.
func udpEcho(t *testing.T) string {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			pc.WriteTo(append([]byte("echo:"), buf[:n]...), from)
		}
	}()
	return pc.LocalAddr().String()
}

func tcpEcho(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	return l.Addr().String()
}

func roundTrip(t *testing.T, c net.Conn, msg, want string) {
	t.Helper()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	n, err := c.Read(buf)
	if err != nil || string(buf[:n]) != want {
		t.Fatalf("got %q %v, want %q", buf[:n], err, want)
	}
}

func TestUDPFlowsPerSender(t *testing.T) {
	target := udpEcho(t)
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go fwd.UDP(ctx, pc, func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", target)
	}, 200*time.Millisecond)
	// Two senders (two viewers), each answered on its own.
	a, _ := net.Dial("udp", pc.LocalAddr().String())
	b, _ := net.Dial("udp", pc.LocalAddr().String())
	defer a.Close()
	defer b.Close()
	roundTrip(t, a, "from a", "echo:from a")
	roundTrip(t, b, "from b", "echo:from b")
	// A flow that went quiet ends; a new datagram starts it again.
	time.Sleep(500 * time.Millisecond)
	roundTrip(t, a, "again", "echo:again")
	cancel()
	time.Sleep(50 * time.Millisecond)
	if _, err := pc.WriteTo([]byte("x"), a.LocalAddr()); err == nil {
		t.Fatal("port still open after ctx ended")
	}
}

func TestPortsOpenClose(t *testing.T) {
	udpTarget, tcpTarget := udpEcho(t), tcpEcho(t)
	ports := &fwd.Ports{Host: "127.0.0.1", Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		// The test's two echo servers stand in for one room's port.
		if network == "udp" {
			address = udpTarget
		} else {
			address = tcpTarget
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}}
	free, _ := net.Listen("tcp", "127.0.0.1:0")
	port := free.Addr().(*net.TCPAddr).Port
	free.Close()
	if err := ports.Open("room", port, "10.77.0.2:52100"); err != nil {
		t.Fatal(err)
	}
	addr := free.Addr().String()
	u, _ := net.Dial("udp", addr)
	defer u.Close()
	roundTrip(t, u, "media", "echo:media")
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, c, "media over tcp", "media over tcp")
	c.Close()
	// Closing frees the port at once; it can be opened again.
	ports.Close("room")
	if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		c.Close()
		t.Fatal("still listening after Close")
	}
	if err := ports.Open("room", port, "10.77.0.2:52100"); err != nil {
		t.Fatal("reopen:", err)
	}
	ports.Close("room")
}
