package desync

import (
	"io"
	"net"
	"sync"
)

// halfCloser is implemented by connections that support shutting down one
// direction (e.g. *net.TCPConn and gVisor's gonet.TCPConn).
type halfCloser interface {
	CloseWrite() error
}

// Relay copies data in both directions between a and b until both directions
// close, then closes both connections. The first write from a to b is where the
// desync strategy takes effect (when b is a *desyncConn).
func Relay(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		copyAndCloseWrite(b, a)
	}()
	go func() {
		defer wg.Done()
		copyAndCloseWrite(a, b)
	}()
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}

func copyAndCloseWrite(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	if hc, ok := dst.(halfCloser); ok {
		_ = hc.CloseWrite()
	}
}
