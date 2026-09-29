package pyjson

import (
	"math"
	"regexp"
	"strconv"
	"testing"
	"unicode/utf8"
)

var pyFloatRE = regexp.MustCompile(`^-?(0\.0*[1-9][0-9]*|[1-9][0-9]*\.[0-9]+|[1-9](\.[0-9]+)?e[+-][0-9]{2,3}|0\.0)$`)

// FuzzFloat checks that every finite float formats in Python's repr shape and
// parses back to the same bits.
func FuzzFloat(f *testing.F) {
	for _, u := range []uint64{0, 1 << 63, 0x3ff0000000000000, 0x4341c37937e08000, 0x3ee4f8b588e368f1, 1, 0x7fefffffffffffff} {
		f.Add(u)
	}
	f.Fuzz(func(t *testing.T, bits uint64) {
		x := math.Float64frombits(bits)
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return
		}
		s := FormatFloat(x)
		if !pyFloatRE.MatchString(s) {
			t.Fatalf("%q does not look like a Python float repr", s)
		}
		y, err := strconv.ParseFloat(s, 64)
		if err != nil || math.Float64bits(y) != bits {
			t.Fatalf("%q does not round-trip (%v)", s, err)
		}
	})
}

// FuzzLoadsDumps checks that anything Loads accepts can be dumped and that
// loading the dump again gives the same text.
func FuzzLoadsDumps(f *testing.F) {
	for _, s := range []string{`{"a":[1,2.5,null]}`, `"\ud83d\ude00"`, `[NaN,-Infinity]`, `{"a":1,"a":2}`, `1e400`, `"\ud800"`, `[[[]]]`, `-0`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := Loads(data)
		if err != nil {
			return
		}
		for _, o := range []Options{{}, {SortKeys: true, Indent: IndentSpaces(2), AllowNonASCII: true}} {
			out, err := Dumps(v, o)
			if err != nil {
				t.Fatalf("Dumps failed on Loads output: %v", err)
			}
			if !utf8.Valid(out) {
				continue // lone surrogates written raw are WTF-8, which Loads rejects
			}
			v2, err := Loads(out)
			if err != nil {
				t.Fatalf("Loads rejected its own Dumps output %q: %v", out, err)
			}
			out2, err := Dumps(v2, o)
			if err != nil || string(out2) != string(out) {
				t.Fatalf("not stable: %q then %q (%v)", out, out2, err)
			}
		}
	})
}
