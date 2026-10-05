package tunnel

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/revocx35/zapret_mobile/engine/desync"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

// pump copies link-layer frames from one channel endpoint into another, acting
// as the wire between the two stacks.
func pump(ctx context.Context, from, to *channel.Endpoint) {
	for {
		pkt := from.ReadContext(ctx)
		if pkt == nil {
			return
		}
		buf := pkt.ToBuffer()
		v := buf.Flatten()
		pkt.DecRef()
		if len(v) == 0 {
			continue
		}
		proto := header.IPv4ProtocolNumber
		if v[0]>>4 == 6 {
			proto = header.IPv6ProtocolNumber
		}
		np := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(v),
		})
		to.InjectInbound(proto, np)
		np.DecRef()
	}
}

func allowLoopback(t *testing.T, s *stack.Stack) {
	t.Helper()
	opt := tcpip.AllowExternalLoopbackTrafficOption(true)
	if e := s.SetNetworkProtocolOption(ipv4.ProtocolNumber, &opt); e != nil {
		t.Fatalf("allow loopback: %v", e)
	}
}

func commonProtocols() stack.Options {
	return stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{
			ipv4.NewProtocol, ipv6.NewProtocol,
		},
		TransportProtocols: []stack.TransportProtocolFactory{
			tcp.NewProtocol, udp.NewProtocol,
			icmp.NewProtocol4, icmp.NewProtocol6,
		},
	}
}

// TestTunnelTCPFlow drives a real TCP flow from a client gVisor stack, through
// our tunnel stack's forwarder and desync dialer, to a real echo server, and
// checks the bytes round-trip intact (i.e. the desync is server-safe end to end).
func TestTunnelTCPFlow(t *testing.T) {
	// Real echo server on loopback.
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { io.Copy(c, c); c.Close() }(c)
		}
	}()
	echoPort := echo.Addr().(*net.TCPAddr).Port

	// Tunnel stack on a channel endpoint.
	tunEP := channel.New(512, 1500, "")
	st, _ := desync.ParseStrategy("split:4")
	tun, err := startWithLink(tunEP, Options{MTU: 1500, Strategy: st})
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Stop()
	// The echo server is on loopback; allow martian (loopback-addressed) packets
	// so the test can use it as the "external" target. Not needed in production,
	// where destinations are real routable addresses.
	allowLoopback(t, tun.stack)

	// Client stack on a channel endpoint.
	cs := stack.New(commonProtocols())
	defer cs.Close()
	cEP := channel.New(512, 1500, "")
	if e := cs.CreateNIC(1, cEP); e != nil {
		t.Fatalf("client nic: %v", e)
	}
	cAddr := tcpip.AddrFromSlice(net.IP{10, 0, 0, 1}.To4())
	cs.AddProtocolAddress(1, tcpip.ProtocolAddress{
		Protocol:          ipv4.ProtocolNumber,
		AddressWithPrefix: cAddr.WithPrefix(),
	}, stack.AddressProperties{})
	cs.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: 1}})
	allowLoopback(t, cs)

	// Wire the two endpoints together.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pump(ctx, cEP, tunEP)
	go pump(ctx, tunEP, cEP)

	// Dial the echo server *through* the tunnel: the client sends to
	// 127.0.0.1:echoPort; the tunnel forwarder re-dials that real address.
	dialCtx, dcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dcancel()
	conn, err := gonet.DialContextTCP(dialCtx, cs, tcpip.FullAddress{
		NIC:  1,
		Addr: tcpip.AddrFromSlice(net.IP{127, 0, 0, 1}.To4()),
		Port: uint16(echoPort),
	}, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatalf("client dial through tunnel: %v", err)
	}
	defer conn.Close()

	payload := []byte("the quick brown fox jumps over the lazy dog")
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("echo mismatch: got %q want %q", got, payload)
	}
}
