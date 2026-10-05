package desync

import (
	"fmt"
	"strconv"
	"strings"
)

// Method is a single stream-level desync transformation applied to the first
// outbound payload of a connection (the TLS ClientHello or the HTTP request).
//
// Every method here is server-safe: the bytes the peer receives, once
// reassembled, are byte-for-byte identical to the original request. We only
// change how those bytes are framed into TLS records and TCP segments, which is
// what confuses a DPI that keys on a contiguous SNI / Host field. Methods that
// require raw sockets or TCP_REPAIR (TTL "fakes", true reordering) are
// deliberately not implemented because they cannot work in an unprivileged
// Android app and would corrupt the stream if attempted. See ARCHITECTURE.md.
type Method int

const (
	// MethodNone passes the payload through untouched.
	MethodNone Method = iota
	// MethodSplit splits the first TCP write into two segments at Pos.
	MethodSplit
	// MethodTLSRec fragments the TLS ClientHello into two TLS records split at
	// Pos. For non-TLS payloads it degrades to MethodSplit.
	MethodTLSRec
	// MethodOOB splits at Pos and sends a single out-of-band (urgent) byte at
	// the boundary. The peer's in-band stream is unchanged.
	MethodOOB
	// MethodHostSplit splits a plaintext HTTP request inside the Host header
	// value (degrades to MethodSplit if no Host header is found).
	MethodHostSplit
)

// posKind selects how a position value is resolved against the payload.
type posKind int

const (
	posAbs  posKind = iota // absolute byte offset from the start of the payload
	posSNI                 // relative to the middle of the SNI host name
	posHost                // relative to the middle of the HTTP Host value
)

// Pos is a position specifier such as "2", "sni", "sni+1", "host-3".
type Pos struct {
	Kind   posKind
	Offset int
}

// Step is one method with its position.
type Step struct {
	Method Method
	Pos    Pos
}

// Strategy is an ordered list of steps plus connection-level options. Steps are
// composed: e.g. [tlsrec:sni, split:1] fragments the ClientHello record at the
// SNI and additionally guarantees the pieces land in separate TCP segments.
type Strategy struct {
	Steps []Step
	// SegDelayMs, if > 0, waits this long between writing consecutive segments.
	SegDelayMs int
}

// Default is a conservative, widely-effective profile: fragment the TLS
// ClientHello at the SNI and split plaintext HTTP at the Host header.
func Default() Strategy {
	return Strategy{
		Steps: []Step{
			{Method: MethodTLSRec, Pos: Pos{Kind: posSNI, Offset: 0}},
			{Method: MethodHostSplit, Pos: Pos{Kind: posHost, Offset: 0}},
		},
	}
}

func parsePos(s string) (Pos, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Pos{Kind: posAbs, Offset: 1}, nil
	}
	// split a leading keyword from an optional +N / -N tail
	kind := posAbs
	rest := s
	switch {
	case strings.HasPrefix(s, "sni"):
		kind = posSNI
		rest = s[len("sni"):]
	case strings.HasPrefix(s, "host"):
		kind = posHost
		rest = s[len("host"):]
	}
	if rest == "" {
		return Pos{Kind: kind, Offset: 0}, nil
	}
	off, err := strconv.Atoi(rest)
	if err != nil {
		return Pos{}, fmt.Errorf("bad position %q", s)
	}
	return Pos{Kind: kind, Offset: off}, nil
}

var methodNames = map[string]Method{
	"none":      MethodNone,
	"split":     MethodSplit,
	"tlsrec":    MethodTLSRec,
	"tlsfrag":   MethodTLSRec,
	"oob":       MethodOOB,
	"hostsplit": MethodHostSplit,
}

// ParseStrategy parses a strategy string. Grammar (whitespace-insensitive):
//
//	strategy := step ("," step)*  ("@delay=" N)?
//	step     := method (":" pos)?
//
// Examples:
//
//	"tlsrec:sni"
//	"tlsrec:sni,split:1"
//	"split:3"
//	"oob:sni+1 @delay=20"
//
// An empty string yields Default().
func ParseStrategy(s string) (Strategy, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Default(), nil
	}
	var st Strategy
	// optional trailing @delay=N
	if i := strings.Index(s, "@delay="); i >= 0 {
		tail := strings.TrimSpace(s[i+len("@delay="):])
		d, err := strconv.Atoi(tail)
		if err != nil || d < 0 {
			return Strategy{}, fmt.Errorf("bad @delay value %q", tail)
		}
		st.SegDelayMs = d
		s = strings.TrimSpace(s[:i])
	}
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		name := tok
		posStr := ""
		if c := strings.IndexByte(tok, ':'); c >= 0 {
			name = strings.TrimSpace(tok[:c])
			posStr = tok[c+1:]
		}
		m, ok := methodNames[strings.ToLower(name)]
		if !ok {
			return Strategy{}, fmt.Errorf("unknown method %q", name)
		}
		pos, err := parsePos(posStr)
		if err != nil {
			return Strategy{}, err
		}
		st.Steps = append(st.Steps, Step{Method: m, Pos: pos})
	}
	if len(st.Steps) == 0 {
		return Default(), nil
	}
	return st, nil
}

// String renders a strategy back to its canonical textual form.
func (st Strategy) String() string {
	var b strings.Builder
	for i, step := range st.Steps {
		if i > 0 {
			b.WriteByte(',')
		}
		for name, m := range methodNames {
			if m == step.Method && name != "tlsfrag" {
				b.WriteString(name)
				break
			}
		}
		b.WriteByte(':')
		switch step.Pos.Kind {
		case posSNI:
			b.WriteString("sni")
		case posHost:
			b.WriteString("host")
		}
		if step.Pos.Offset != 0 || step.Pos.Kind == posAbs {
			if step.Pos.Offset >= 0 && step.Pos.Kind != posAbs {
				b.WriteByte('+')
			}
			b.WriteString(strconv.Itoa(step.Pos.Offset))
		}
	}
	if st.SegDelayMs > 0 {
		fmt.Fprintf(&b, " @delay=%d", st.SegDelayMs)
	}
	return b.String()
}
