package desync

import (
	"context"
	"errors"
	"net"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Protector lets the host exclude a socket from the VPN so that the engine's own
// upstream connections do not loop back through the TUN. On Android this wraps
// VpnService.protect(fd). It may be nil on desktop or in tests.
type Protector interface {
	Protect(fd int) bool
}

// ErrProtectFailed is returned when the host refuses to protect a socket.
var ErrProtectFailed = errors.New("protect socket failed")

// Config configures a Dialer.
type Config struct {
	Strategy  Strategy
	Protector Protector
}

// Dialer dials upstream TCP connections that apply the desync strategy to their
// first write.
type Dialer struct {
	cfg Config
	nd  net.Dialer
}

// NewDialer builds a Dialer. The returned dialer sets TCP_NODELAY (so split
// segments are not coalesced) and protects every socket it opens.
func NewDialer(cfg Config) *Dialer {
	d := &Dialer{cfg: cfg}
	d.nd.Control = func(network, address string, c syscall.RawConn) error {
		var perr error
		cerr := c.Control(func(fd uintptr) {
			_ = unix.SetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_NODELAY, 1)
			// Fail closed: an unprotected socket would be routed back into the TUN
			// and loop forever.
			if cfg.Protector != nil && !cfg.Protector.Protect(int(fd)) {
				perr = ErrProtectFailed
			}
		})
		if cerr != nil {
			return cerr
		}
		return perr
	}
	return d
}

// DialContext dials addr ("host:port"; host may be a name or IP) and returns a
// connection whose first Write is transformed by the strategy.
func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	c, err := d.nd.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	return &desyncConn{Conn: c, strat: d.cfg.Strategy}, nil
}

// desyncConn applies the strategy to the first application write on a connection
// and passes all later writes through unchanged.
type desyncConn struct {
	net.Conn
	strat     Strategy
	mu        sync.Mutex
	firstDone bool
}

func (c *desyncConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	first := !c.firstDone
	c.firstDone = true
	c.mu.Unlock()
	if !first || len(b) == 0 {
		return c.Conn.Write(b)
	}
	return c.writeFirst(b)
}

// CloseWrite forwards a half-close to the underlying connection so that EOF
// propagates to the peer after the relay finishes one direction.
func (c *desyncConn) CloseWrite() error {
	if hc, ok := c.Conn.(halfCloser); ok {
		return hc.CloseWrite()
	}
	return nil
}

func (c *desyncConn) writeFirst(b []byte) (int, error) {
	segs := BuildSegments(b, c.strat)
	total := 0
	for i, s := range segs {
		if i > 0 && c.strat.SegDelayMs > 0 {
			time.Sleep(time.Duration(c.strat.SegDelayMs) * time.Millisecond)
		}
		if s.OOB {
			// Urgent byte is extra data the peer drops; errors are non-fatal.
			_ = c.sendOOB(s.Data[0])
			continue
		}
		n, err := c.Conn.Write(s.Data)
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// sendOOB writes a single byte as TCP urgent (out-of-band) data, integrating
// with the runtime network poller so it respects writability.
func (c *desyncConn) sendOOB(bt byte) error {
	sc, ok := c.Conn.(syscall.Conn)
	if !ok {
		return nil
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return err
	}
	var serr error
	werr := raw.Write(func(fd uintptr) bool {
		e := unix.Send(int(fd), []byte{bt}, unix.MSG_OOB)
		if e == unix.EAGAIN {
			return false // not ready; poller will retry
		}
		serr = e
		return true
	})
	if werr != nil {
		return werr
	}
	return serr
}
