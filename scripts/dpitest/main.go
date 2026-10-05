// dpitest is a tiny TLS client used by scripts/e2e.sh from *inside* the Android
// emulator, so its traffic traverses the Zapret VPN. Built as a static
// GOOS=linux binary (runnable on Android).
//
//	dpitest frag <ip:port> <sni>   connect and send a ClientHello, then exit
//	dpitest conn <ip:port> <sni>   full handshake + HTTP GET, print the result
package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Println("usage: dpitest <frag|conn> <ip:port> <sni>")
		os.Exit(2)
	}
	mode, addr, sni := os.Args[1], os.Args[2], os.Args[3]

	raw, err := net.DialTimeout("tcp", addr, 8*time.Second)
	if err != nil {
		fmt.Printf("RESULT dial-error %v\n", err)
		os.Exit(1)
	}
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(8 * time.Second))

	conn := tls.Client(raw, &tls.Config{ServerName: sni, InsecureSkipVerify: true})

	if mode == "frag" {
		// We only need the ClientHello to be written; the capture server reads it
		// and closes, so the handshake itself is expected to fail.
		_ = conn.Handshake()
		fmt.Println("RESULT clienthello-sent")
		return
	}

	if err := conn.Handshake(); err != nil {
		fmt.Printf("RESULT handshake-error %v\n", err)
		os.Exit(1)
	}
	req := "GET / HTTP/1.1\r\nHost: " + sni + "\r\nConnection: close\r\nUser-Agent: dpitest\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		fmt.Printf("RESULT write-error %v\n", err)
		os.Exit(1)
	}
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	line := string(buf[:n])
	if len(line) > 0 {
		fmt.Printf("RESULT handshake-ok first-bytes=%q\n", firstLine(line))
		return
	}
	fmt.Println("RESULT handshake-ok no-data")
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' || s[i] == '\n' {
			return s[:i]
		}
	}
	if len(s) > 60 {
		return s[:60]
	}
	return s
}
