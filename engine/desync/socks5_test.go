package desync

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

// TestSocksEndToEnd drives a real SOCKS5 CONNECT through the server to a local
// TCP echo-ish target, verifying the handshake and that a fragmented first
// payload is delivered to the target byte-for-byte intact.
func TestSocksEndToEnd(t *testing.T) {
	// Upstream target: reads everything it is sent until the client half-closes,
	// then reports the bytes back over a channel.
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	got := make(chan []byte, 1)
	go func() {
		c, err := target.Accept()
		if err != nil {
			got <- nil
			return
		}
		defer c.Close()
		b, _ := io.ReadAll(c)
		got <- b
	}()

	// SOCKS server with a fragmenting strategy and no protector (host test).
	st, _ := ParseStrategy("split:3")
	srv := NewServer(NewDialer(Config{Strategy: st}), nil)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.Serve(ln)

	// SOCKS client.
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	// Negotiate: no-auth.
	if _, err := c.Write([]byte{socksVersion, 1, 0x00}); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(c, resp); err != nil {
		t.Fatal(err)
	}
	if resp[0] != socksVersion || resp[1] != 0x00 {
		t.Fatalf("bad method reply %v", resp)
	}

	// CONNECT to the target by IPv4.
	ta := target.Addr().(*net.TCPAddr)
	req := []byte{socksVersion, cmdConnect, 0x00, atypIPv4}
	req = append(req, ta.IP.To4()...)
	p := make([]byte, 2)
	binary.BigEndian.PutUint16(p, uint16(ta.Port))
	req = append(req, p...)
	if _, err := c.Write(req); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != repSucceeded {
		t.Fatalf("connect failed, rep=%d", reply[1])
	}

	payload := []byte("hello-dpi-world")
	if _, err := c.Write(payload); err != nil {
		t.Fatal(err)
	}
	// Half-close so the target's ReadAll returns.
	c.(*net.TCPConn).CloseWrite()

	select {
	case b := <-got:
		if !bytes.Equal(b, payload) {
			t.Fatalf("target received %q, want %q", b, payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for target")
	}
}
