package desync

import (
	"bytes"
	"crypto/tls"
	"net"
	"testing"
	"time"
)

// captureClientHello returns the raw bytes of a real TLS ClientHello produced by
// crypto/tls for the given server name.
func captureClientHello(t *testing.T, serverName string) []byte {
	t.Helper()
	c1, c2 := net.Pipe()
	defer c2.Close()
	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		_ = c2.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := c2.Read(buf)
		done <- append([]byte(nil), buf[:n]...)
	}()
	cfg := &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12}
	client := tls.Client(c1, cfg)
	go client.Handshake() // will block after the first flight; we only need it
	select {
	case ch := <-done:
		c1.Close()
		if len(ch) == 0 {
			t.Fatal("captured empty ClientHello")
		}
		return ch
	case <-time.After(3 * time.Second):
		t.Fatal("timed out capturing ClientHello")
		return nil
	}
}

// reassembleHandshake walks every TLS record in buf and concatenates the record
// fragments, i.e. reconstructs the byte stream the peer's TLS layer sees.
func reassembleHandshake(t *testing.T, buf []byte) []byte {
	t.Helper()
	var out []byte
	i := 0
	for i+5 <= len(buf) {
		if buf[i] != recordHandshake {
			break
		}
		l := int(buf[i+3])<<8 | int(buf[i+4])
		if i+5+l > len(buf) {
			t.Fatalf("record at %d overruns buffer", i)
		}
		out = append(out, buf[i+5:i+5+l]...)
		i += 5 + l
	}
	return out
}

func countRecords(buf []byte) int {
	n, i := 0, 0
	for i+5 <= len(buf) {
		if buf[i] != recordHandshake {
			break
		}
		l := int(buf[i+3])<<8 | int(buf[i+4])
		if i+5+l > len(buf) {
			break
		}
		n++
		i += 5 + l
	}
	return n
}

func joinNonOOB(segs []Segment) []byte {
	var b []byte
	for _, s := range segs {
		if !s.OOB {
			b = append(b, s.Data...)
		}
	}
	return b
}

func TestIsClientHelloAndSNI(t *testing.T) {
	ch := captureClientHello(t, "www.example.com")
	if !IsClientHello(ch) {
		t.Fatal("expected ClientHello to be recognised")
	}
	s, e, ok := sniRange(ch)
	if !ok {
		t.Fatal("SNI not found")
	}
	if got := string(ch[s:e]); got != "www.example.com" {
		t.Fatalf("SNI = %q, want www.example.com", got)
	}
}

func TestTLSRecFragmentIsServerSafe(t *testing.T) {
	ch := captureClientHello(t, "www.example.com")
	origPayload := reassembleHandshake(t, ch)

	st, err := ParseStrategy("tlsrec:sni")
	if err != nil {
		t.Fatal(err)
	}
	segs := BuildSegments(ch, st)
	if len(segs) < 2 {
		t.Fatalf("expected the ClientHello to be fragmented, got %d segment(s)", len(segs))
	}
	out := joinNonOOB(segs)

	// The fragmented buffer must reassemble to the identical handshake stream.
	if got := reassembleHandshake(t, out); !bytes.Equal(got, origPayload) {
		t.Fatal("reassembled handshake differs from the original (not server-safe)")
	}
	if n := countRecords(out); n != 2 {
		t.Fatalf("expected 2 TLS records, got %d", n)
	}
	// The SNI must not appear in full inside the first record alone.
	firstRecLen := 5 + (int(out[3])<<8 | int(out[4]))
	if bytes.Contains(out[:firstRecLen], []byte("www.example.com")) {
		t.Fatal("full SNI still present in the first record; DPI split ineffective")
	}
}

func TestSplitAbsolute(t *testing.T) {
	data := []byte("GET / HTTP/1.1\r\n\r\n")
	st, _ := ParseStrategy("split:5")
	segs := BuildSegments(data, st)
	if len(segs) != 2 {
		t.Fatalf("want 2 segments, got %d", len(segs))
	}
	if !bytes.Equal(joinNonOOB(segs), data) {
		t.Fatal("split changed the byte stream")
	}
	if len(segs[0].Data) != 5 {
		t.Fatalf("first segment len = %d, want 5", len(segs[0].Data))
	}
}

func TestHostSplit(t *testing.T) {
	data := []byte("GET / HTTP/1.1\r\nHost: www.example.com\r\nAccept: */*\r\n\r\n")
	st, _ := ParseStrategy("hostsplit:host")
	segs := BuildSegments(data, st)
	if len(segs) < 2 {
		t.Fatalf("expected a split inside Host, got %d segment(s)", len(segs))
	}
	if !bytes.Equal(joinNonOOB(segs), data) {
		t.Fatal("hostsplit changed the byte stream")
	}
	// The boundary must land inside the host value so "example" is cut.
	if bytes.Contains(segs[0].Data, []byte("www.example.com")) {
		t.Fatal("full Host value present in first segment")
	}
}

func TestOOBIsExtraByte(t *testing.T) {
	data := []byte("GET / HTTP/1.1\r\n\r\n")
	st, _ := ParseStrategy("oob:4")
	segs := BuildSegments(data, st)
	var oobCount int
	for _, s := range segs {
		if s.OOB {
			oobCount++
			if len(s.Data) != 1 {
				t.Fatalf("OOB segment len = %d, want 1", len(s.Data))
			}
		}
	}
	if oobCount != 1 {
		t.Fatalf("want exactly 1 OOB segment, got %d", oobCount)
	}
	// The in-band stream (OOB excluded) must be unchanged.
	if !bytes.Equal(joinNonOOB(segs), data) {
		t.Fatal("OOB injection corrupted the in-band stream")
	}
}

func TestTLSRecPlusSplit(t *testing.T) {
	ch := captureClientHello(t, "secure.example.org")
	origPayload := reassembleHandshake(t, ch)
	st, _ := ParseStrategy("tlsrec:sni,split:1")
	segs := BuildSegments(ch, st)
	if len(segs) < 3 {
		t.Fatalf("expected tlsrec+split to yield >=3 segments, got %d", len(segs))
	}
	if got := reassembleHandshake(t, joinNonOOB(segs)); !bytes.Equal(got, origPayload) {
		t.Fatal("tlsrec+split not server-safe")
	}
}

func TestDefaultStrategyOnNonTLS(t *testing.T) {
	// Default includes tlsrec+hostsplit; on a plain HTTP request only hostsplit
	// should act, and the stream must stay intact.
	data := []byte("GET / HTTP/1.1\r\nHost: a.example.net\r\n\r\n")
	segs := BuildSegments(data, Default())
	if !bytes.Equal(joinNonOOB(segs), data) {
		t.Fatal("default strategy corrupted a plain HTTP request")
	}
}
