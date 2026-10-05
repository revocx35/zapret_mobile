// Package zapret is the gomobile-bound surface of the DPI-bypass engine. It is
// compiled to an Android .aar and driven from the Kotlin VpnService.
//
// All exported identifiers here use only gomobile-compatible types (basic types,
// string, error, and bound interfaces) so the generated Java/Kotlin API stays
// clean.
package zapret

import (
	"errors"
	"net"
	"sync"

	"github.com/revocx35/zapret_mobile/engine/desync"
	"github.com/revocx35/zapret_mobile/engine/tunnel"
)

// Protector is implemented on the Kotlin side to wrap VpnService.protect(fd),
// which keeps the engine's own upstream sockets outside the VPN.
type Protector interface {
	Protect(fd int) bool
}

// Logger is implemented on the Kotlin side to surface diagnostics.
type Logger interface {
	Log(line string)
}

// Engine owns a running tunnel and/or local SOCKS proxy.
//
// Two locks on purpose: mu guards the running components (tun/socksLn), while
// cbMu guards the callbacks. logf/Protect take only cbMu, so the engine can hold
// mu across tunnel.Start — which logs through logf — without deadlocking.
type Engine struct {
	mu      sync.Mutex
	tun     *tunnel.Tunnel
	socksLn net.Listener

	cbMu      sync.RWMutex
	protector Protector
	logger    Logger
}

// New creates an idle engine.
func NewEngine() *Engine { return &Engine{} }

// SetProtector installs the socket protector. Call before Start*.
func (e *Engine) SetProtector(p Protector) {
	e.cbMu.Lock()
	e.protector = p
	e.cbMu.Unlock()
}

// SetLogger installs the diagnostics sink.
func (e *Engine) SetLogger(l Logger) {
	e.cbMu.Lock()
	e.logger = l
	e.cbMu.Unlock()
}

func (e *Engine) logf(s string) {
	e.cbMu.RLock()
	l := e.logger
	e.cbMu.RUnlock()
	if l != nil {
		l.Log(s)
	}
}

func (e *Engine) currentProtector() desync.Protector {
	e.cbMu.RLock()
	defer e.cbMu.RUnlock()
	if e.protector == nil {
		return nil
	}
	return e.protector
}

// ValidateStrategy returns an empty string if the strategy parses, otherwise the
// parse error message. Useful for live validation in the UI.
func ValidateStrategy(strategy string) string {
	if _, err := desync.ParseStrategy(strategy); err != nil {
		return err.Error()
	}
	return ""
}

// StartTunnel begins forwarding packets from the VpnService TUN fd. The fd is
// dup'd internally; the caller keeps ownership of the ParcelFileDescriptor.
func (e *Engine) StartTunnel(fd int, mtu int, strategy string, blockQUIC bool) error {
	st, err := desync.ParseStrategy(strategy)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.tun != nil {
		return errors.New("tunnel already running")
	}
	prot := e.currentProtector()
	t, err := tunnel.Start(tunnel.Options{
		FD:        fd,
		MTU:       mtu,
		Strategy:  st,
		Protector: prot,
		BlockQUIC: blockQUIC,
		Log:       e.logf,
	})
	if err != nil {
		return err
	}
	e.tun = t
	return nil
}

// StartSocks runs a local SOCKS5 desync proxy on addr (e.g. "127.0.0.1:1080").
// This is an alternative to StartTunnel for proxy-only use.
func (e *Engine) StartSocks(addr string, strategy string) error {
	st, err := desync.ParseStrategy(strategy)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.socksLn != nil {
		return errors.New("socks already running")
	}
	prot := e.currentProtector()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := desync.NewServer(desync.NewDialer(desync.Config{Strategy: st, Protector: prot}), e.logf)
	e.socksLn = ln
	go func() {
		if err := srv.Serve(ln); err != nil {
			e.logf("socks server stopped: " + err.Error())
		}
	}()
	e.logf("socks proxy listening on " + addr)
	return nil
}

// Stop tears down the tunnel and/or SOCKS proxy. Safe to call multiple times.
func (e *Engine) Stop() {
	e.mu.Lock()
	tun := e.tun
	ln := e.socksLn
	e.tun = nil
	e.socksLn = nil
	e.mu.Unlock()

	if tun != nil {
		tun.Stop()
	}
	if ln != nil {
		_ = ln.Close()
	}
}

// Running reports whether a tunnel is active.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tun != nil
}
