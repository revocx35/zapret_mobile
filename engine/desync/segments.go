package desync

import (
	"bytes"
	"sort"
)

// Segment is one unit the relay writes to the upstream socket. Normal segments
// become one (or more) TCP segments; an OOB segment is a single urgent byte.
type Segment struct {
	Data []byte
	OOB  bool
}

// oobFiller is the throwaway byte injected as TCP urgent data. With the default
// socket option SO_OOBINLINE disabled, the peer delivers it out of band and its
// in-band stream is unaffected, so the request the server reassembles is intact.
const oobFiller = 0x61 // 'a'

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// hostRange locates the Host header value in a plaintext HTTP request and
// returns its absolute [start,end) byte offsets.
func hostRange(data []byte) (start, end int, ok bool) {
	// Only look at the head of the request.
	head := data
	if len(head) > 2048 {
		head = head[:2048]
	}
	lower := bytes.ToLower(head)
	i := bytes.Index(lower, []byte("\nhost:"))
	if i < 0 {
		if bytes.HasPrefix(lower, []byte("host:")) {
			i = -1 // value starts right after "host:"
		} else {
			return 0, 0, false
		}
	}
	vs := i + len("\nhost:")
	if i == -1 {
		vs = len("host:")
	}
	// skip optional spaces
	for vs < len(data) && (data[vs] == ' ' || data[vs] == '\t') {
		vs++
	}
	ve := vs
	for ve < len(data) && data[ve] != '\r' && data[ve] != '\n' {
		ve++
	}
	if ve <= vs {
		return 0, 0, false
	}
	return vs, ve, true
}

// resolvePos maps a Pos to an absolute byte offset in data, in [1,len(data)-1].
func resolvePos(data []byte, pos Pos) int {
	n := len(data)
	if n < 2 {
		return 1
	}
	switch pos.Kind {
	case posSNI:
		if s, e, ok := sniRange(data); ok {
			return clampInt((s+e)/2+pos.Offset, 1, n-1)
		}
		return clampInt(n/2+pos.Offset, 1, n-1)
	case posHost:
		if s, e, ok := hostRange(data); ok {
			return clampInt((s+e)/2+pos.Offset, 1, n-1)
		}
		return clampInt(n/2+pos.Offset, 1, n-1)
	default: // posAbs
		return clampInt(pos.Offset, 1, n-1)
	}
}

// tlsFragment rewrites a TLS handshake record so its fragment is split into two
// records at absolute offset p (which must fall inside the record payload). It
// returns the new buffer and the offset in the new buffer where the second
// record begins (a forced TCP cut point). ok is false if p is not inside a
// handshake record payload.
func tlsFragment(data []byte, p int) (out []byte, boundary int, ok bool) {
	rec, found := parseFirstRecord(data)
	if !found {
		return nil, 0, false
	}
	payStart := rec.PayloadOff
	payEnd := rec.PayloadOff + rec.PayloadLen
	if payEnd > len(data) {
		// ClientHello not fully captured; cannot safely rewrite record lengths.
		return nil, 0, false
	}
	if p <= payStart || p >= payEnd {
		return nil, 0, false
	}
	k := p - payStart // bytes of payload in the first record
	l1 := k
	l2 := rec.PayloadLen - k

	out = make([]byte, 0, len(data)+5)
	// record 1 header + fragment
	out = append(out, data[0], data[1], data[2], byte(l1>>8), byte(l1))
	out = append(out, data[payStart:payStart+k]...)
	// record 2 header + fragment (reuse original content type + version bytes)
	out = append(out, data[0], data[1], data[2], byte(l2>>8), byte(l2))
	out = append(out, data[payStart+k:payEnd]...)
	// trailing bytes after the first record, untouched
	out = append(out, data[payEnd:]...)
	boundary = 5 + k // length of record 1
	return out, boundary, true
}

// BuildSegments turns the first outbound payload into the ordered list of
// segments to write, according to the strategy. The concatenation of all
// non-OOB segment data is byte-for-byte equal to a server-visible copy of the
// original payload (OOB bytes are extra and dropped by the peer).
func BuildSegments(data []byte, st Strategy) []Segment {
	if len(data) < 2 || len(st.Steps) == 0 {
		return []Segment{{Data: data}}
	}

	out := data
	cuts := map[int]bool{}
	oobAt := map[int]bool{}
	// Shift maps an original offset to the current `out` coordinates after any
	// record fragmentation inserted header bytes.
	shift := func(o int) int { return o }

	for _, step := range st.Steps {
		switch step.Method {
		case MethodNone:
			// no-op
		case MethodTLSRec:
			p := resolvePos(data, step.Pos)
			if frag, boundary, ok := tlsFragment(data, p); ok {
				out = frag
				splitAt := p
				shift = func(o int) int {
					if o >= splitAt {
						return o + 5
					}
					return o
				}
				cuts[boundary] = true
			} else {
				// Not a (complete) TLS record: fall back to a plain split.
				cuts[shift(p)] = true
			}
		case MethodSplit:
			cuts[shift(resolvePos(data, step.Pos))] = true
		case MethodHostSplit:
			if _, _, ok := hostRange(data); ok {
				cuts[shift(resolvePos(data, step.Pos))] = true
			}
			// If there is no Host header this step does nothing (e.g. a TLS
			// connection already handled by tlsrec).
		case MethodOOB:
			c := shift(resolvePos(data, step.Pos))
			cuts[c] = true
			oobAt[c] = true
		}
	}

	if len(cuts) == 0 {
		return []Segment{{Data: out}}
	}

	bounds := make([]int, 0, len(cuts))
	for c := range cuts {
		if c > 0 && c < len(out) {
			bounds = append(bounds, c)
		}
	}
	sort.Ints(bounds)

	var segs []Segment
	prev := 0
	for _, b := range bounds {
		if b > prev {
			segs = append(segs, Segment{Data: out[prev:b]})
		}
		if oobAt[b] {
			segs = append(segs, Segment{Data: []byte{oobFiller}, OOB: true})
		}
		prev = b
	}
	if prev < len(out) {
		segs = append(segs, Segment{Data: out[prev:]})
	}
	if len(segs) == 0 {
		return []Segment{{Data: out}}
	}
	return segs
}
