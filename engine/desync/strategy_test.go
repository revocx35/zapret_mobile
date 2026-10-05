package desync

import "testing"

func TestParseStrategyErrors(t *testing.T) {
	for _, s := range []string{"bogus", "split:abc", "oob:sni+x", "tlsrec @delay=-1"} {
		if _, err := ParseStrategy(s); err == nil {
			t.Errorf("ParseStrategy(%q) expected error, got nil", s)
		}
	}
}

func TestParseStrategyOK(t *testing.T) {
	st, err := ParseStrategy("tlsrec:sni,split:1 @delay=20")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Steps) != 2 {
		t.Fatalf("want 2 steps, got %d", len(st.Steps))
	}
	if st.Steps[0].Method != MethodTLSRec || st.Steps[0].Pos.Kind != posSNI {
		t.Errorf("step0 = %+v", st.Steps[0])
	}
	if st.Steps[1].Method != MethodSplit || st.Steps[1].Pos.Offset != 1 {
		t.Errorf("step1 = %+v", st.Steps[1])
	}
	if st.SegDelayMs != 20 {
		t.Errorf("delay = %d, want 20", st.SegDelayMs)
	}
}

func TestParseEmptyIsDefault(t *testing.T) {
	st, err := ParseStrategy("")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Steps) != len(Default().Steps) {
		t.Fatalf("empty strategy should equal Default()")
	}
}

func TestPosParsing(t *testing.T) {
	cases := map[string]Pos{
		"sni":    {Kind: posSNI, Offset: 0},
		"sni+1":  {Kind: posSNI, Offset: 1},
		"sni-2":  {Kind: posSNI, Offset: -2},
		"host":   {Kind: posHost, Offset: 0},
		"5":      {Kind: posAbs, Offset: 5},
		"host+3": {Kind: posHost, Offset: 3},
	}
	for in, want := range cases {
		got, err := parsePos(in)
		if err != nil {
			t.Errorf("parsePos(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parsePos(%q) = %+v, want %+v", in, got, want)
		}
	}
}
