package desync

// Minimal TLS ClientHello parser. We only need to recognise a ClientHello and
// locate the SNI host name inside it, so that the desync layer can fragment the
// record across the SNI. We never decrypt or modify TLS semantics.

// TLS record content types.
const (
	recordHandshake = 0x16
)

// Handshake message types.
const (
	hsClientHello = 0x01
)

// TLSRecord describes the first TLS record found at the start of a buffer.
type TLSRecord struct {
	// PayloadOff is the offset of the record fragment (just after the 5-byte
	// record header) within the original buffer.
	PayloadOff int
	// PayloadLen is the length of the record fragment.
	PayloadLen int
}

// parseFirstRecord returns the first TLS record header if buf starts with a
// handshake record, otherwise ok is false.
func parseFirstRecord(buf []byte) (TLSRecord, bool) {
	if len(buf) < 5 || buf[0] != recordHandshake {
		return TLSRecord{}, false
	}
	// buf[1], buf[2] are the legacy record version (major/minor). We don't rely
	// on an exact value; many stacks send 0x0301..0x0303.
	if buf[1] != 0x03 {
		return TLSRecord{}, false
	}
	l := int(buf[3])<<8 | int(buf[4])
	if l <= 0 {
		return TLSRecord{}, false
	}
	return TLSRecord{PayloadOff: 5, PayloadLen: l}, true
}

// IsClientHello reports whether buf starts with a TLS handshake record whose
// first handshake message is a ClientHello.
func IsClientHello(buf []byte) bool {
	rec, ok := parseFirstRecord(buf)
	if !ok {
		return false
	}
	if rec.PayloadOff >= len(buf) {
		return false
	}
	return buf[rec.PayloadOff] == hsClientHello
}

// sniRange locates the SNI host_name bytes inside a ClientHello. It returns the
// absolute [start,end) offsets of the host name string within buf. ok is false
// when no SNI is present or the message is malformed/incomplete.
//
// ClientHello layout (RFC 8446 / 5246), all lengths big-endian:
//
//	handshake:  type(1) length(3) body...
//	body:       version(2) random(32) session_id(1+n)
//	            cipher_suites(2+n) compression(1+n)
//	            extensions(2+n)
//	extension:  type(2) length(2) data...
//	SNI (type 0): server_name_list(2) then entries of
//	            name_type(1) host_name_len(2) host_name(n)
func sniRange(buf []byte) (start, end int, ok bool) {
	rec, ok := parseFirstRecord(buf)
	if !ok {
		return 0, 0, false
	}
	// We parse within the record fragment only. If the ClientHello spans more
	// than the captured buffer we bail out (caller falls back to a fixed split).
	end = rec.PayloadOff + rec.PayloadLen
	if end > len(buf) {
		end = len(buf)
	}
	p := newParser(buf, rec.PayloadOff, end)

	hsType, ok := p.u8()
	if !ok || hsType != hsClientHello {
		return 0, 0, false
	}
	hsLen, ok := p.u24()
	if !ok {
		return 0, 0, false
	}
	// Constrain parsing to the handshake body.
	bodyEnd := p.pos + hsLen
	if bodyEnd > p.end {
		bodyEnd = p.end
	}
	p.end = bodyEnd

	if !p.skip(2) { // legacy_version
		return 0, 0, false
	}
	if !p.skip(32) { // random
		return 0, 0, false
	}
	if !p.skipVec(1) { // session_id
		return 0, 0, false
	}
	if !p.skipVec(2) { // cipher_suites
		return 0, 0, false
	}
	if !p.skipVec(1) { // compression_methods
		return 0, 0, false
	}
	// extensions
	extTotal, ok := p.u16()
	if !ok {
		return 0, 0, false
	}
	extEnd := p.pos + extTotal
	if extEnd > p.end {
		extEnd = p.end
	}
	for p.pos < extEnd {
		extType, ok := p.u16()
		if !ok {
			return 0, 0, false
		}
		extLen, ok := p.u16()
		if !ok {
			return 0, 0, false
		}
		extDataEnd := p.pos + extLen
		if extDataEnd > extEnd {
			return 0, 0, false
		}
		if extType == 0x0000 { // server_name
			// server_name_list length
			sub := newParser(buf, p.pos, extDataEnd)
			if !sub.skip(2) { // list length
				return 0, 0, false
			}
			nameType, ok := sub.u8()
			if !ok {
				return 0, 0, false
			}
			nameLen, ok := sub.u16()
			if !ok {
				return 0, 0, false
			}
			if nameType != 0x00 { // host_name
				return 0, 0, false
			}
			s := sub.pos
			e := sub.pos + nameLen
			if e > extDataEnd {
				return 0, 0, false
			}
			return s, e, true
		}
		p.pos = extDataEnd
	}
	return 0, 0, false
}
