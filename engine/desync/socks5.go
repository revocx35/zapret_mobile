package desync

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// SOCKS5 constants.
const (
	socksVersion = 0x05

	cmdConnect = 0x01

	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04

	repSucceeded          = 0x00
	repGeneralFailure     = 0x01
	repCommandNotSupport  = 0x07
	repAddrTypeNotSupport = 0x08
)

// Server is a minimal SOCKS5 proxy (no authentication, CONNECT only) that dials
// upstream through a desync Dialer. It lets the engine run as a plain local
// proxy, and is the unit the TUN tunnel reuses.
type Server struct {
	dialer *Dialer
	log    func(string)
}

// NewServer creates a SOCKS5 server using the given dialer. log may be nil.
func NewServer(dialer *Dialer, log func(string)) *Server {
	if log == nil {
		log = func(string) {}
	}
	return &Server{dialer: dialer, log: log}
}

// Serve accepts connections on ln until it is closed.
func (s *Server) Serve(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(c)
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))

	target, err := socksHandshake(c)
	if err != nil {
		s.log("socks handshake: " + err.Error())
		return
	}
	// Clear the handshake deadline before relaying.
	_ = c.SetDeadline(time.Time{})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	up, err := s.dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		_ = writeSocksReply(c, repGeneralFailure)
		s.log("dial " + target + ": " + err.Error())
		return
	}
	if err := writeSocksReply(c, repSucceeded); err != nil {
		up.Close()
		return
	}
	Relay(c, up)
}

// socksHandshake performs method negotiation and reads the CONNECT request,
// returning the "host:port" target.
func socksHandshake(c net.Conn) (string, error) {
	// Version + method selection.
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return "", err
	}
	if hdr[0] != socksVersion {
		return "", fmt.Errorf("bad socks version %d", hdr[0])
	}
	methods := make([]byte, int(hdr[1]))
	if _, err := io.ReadFull(c, methods); err != nil {
		return "", err
	}
	// Reply: no authentication required.
	if _, err := c.Write([]byte{socksVersion, 0x00}); err != nil {
		return "", err
	}

	// Request: VER CMD RSV ATYP ...
	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return "", err
	}
	if req[0] != socksVersion {
		return "", fmt.Errorf("bad request version %d", req[0])
	}
	if req[1] != cmdConnect {
		_ = writeSocksReply(c, repCommandNotSupport)
		return "", fmt.Errorf("unsupported command %d", req[1])
	}

	var host string
	switch req[3] {
	case atypIPv4:
		b := make([]byte, 4)
		if _, err := io.ReadFull(c, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	case atypIPv6:
		b := make([]byte, 16)
		if _, err := io.ReadFull(c, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	case atypDomain:
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return "", err
		}
		b := make([]byte, int(l[0]))
		if _, err := io.ReadFull(c, b); err != nil {
			return "", err
		}
		host = string(b)
	default:
		_ = writeSocksReply(c, repAddrTypeNotSupport)
		return "", fmt.Errorf("unsupported address type %d", req[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(c, portBuf); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBuf)
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

// writeSocksReply sends a reply with a zero BND.ADDR/BND.PORT.
func writeSocksReply(c net.Conn, rep byte) error {
	_, err := c.Write([]byte{socksVersion, rep, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
	return err
}
