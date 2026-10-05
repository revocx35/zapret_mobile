package desync

// parser is a tiny bounds-checked byte cursor used by the TLS parser.
type parser struct {
	b   []byte
	pos int
	end int
}

func newParser(b []byte, start, end int) *parser {
	if end > len(b) {
		end = len(b)
	}
	if start < 0 {
		start = 0
	}
	return &parser{b: b, pos: start, end: end}
}

func (p *parser) remaining() int { return p.end - p.pos }

func (p *parser) u8() (int, bool) {
	if p.remaining() < 1 {
		return 0, false
	}
	v := int(p.b[p.pos])
	p.pos++
	return v, true
}

func (p *parser) u16() (int, bool) {
	if p.remaining() < 2 {
		return 0, false
	}
	v := int(p.b[p.pos])<<8 | int(p.b[p.pos+1])
	p.pos += 2
	return v, true
}

func (p *parser) u24() (int, bool) {
	if p.remaining() < 3 {
		return 0, false
	}
	v := int(p.b[p.pos])<<16 | int(p.b[p.pos+1])<<8 | int(p.b[p.pos+2])
	p.pos += 3
	return v, true
}

func (p *parser) skip(n int) bool {
	if p.remaining() < n {
		return false
	}
	p.pos += n
	return true
}

// skipVec skips a length-prefixed vector whose length field is lenBytes wide.
func (p *parser) skipVec(lenBytes int) bool {
	var n int
	switch lenBytes {
	case 1:
		v, ok := p.u8()
		if !ok {
			return false
		}
		n = v
	case 2:
		v, ok := p.u16()
		if !ok {
			return false
		}
		n = v
	default:
		return false
	}
	return p.skip(n)
}
