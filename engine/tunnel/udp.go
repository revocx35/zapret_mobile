package tunnel

import (
	"context"
	"net"
	"syscall"

	"github.com/revocx35/zapret_mobile/engine/desync"
)

// dialProtectedUDP opens a connected UDP socket to the real destination, with
// the socket protected from the VPN so the datagrams leave the device instead of
// looping back into the TUN.
func dialProtectedUDP(ctx context.Context, ip net.IP, port int, p desync.Protector) (*net.UDPConn, error) {
	var d net.Dialer
	d.Control = func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			if p != nil {
				p.Protect(int(fd))
			}
		})
	}
	c, err := d.DialContext(ctx, "udp", net.JoinHostPort(ip.String(), itoa(port)))
	if err != nil {
		return nil, err
	}
	return c.(*net.UDPConn), nil
}

func itoa(v int) string {
	// small, allocation-free positive int formatting
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
