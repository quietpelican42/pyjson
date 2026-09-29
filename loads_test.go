package pyjson

import (
	"errors"
	"math"
	"math/big"
	"os"
	"testing"
)

func TestLoadsValues(t *testing.T) {
	tests := []struct {
		in   string
		want string // Dumps(Loads(in)) with default options; from Python 3.14.7
	}{
		{`{"a":1,"b":2,"a":3}`, `{"a": 3, "b": 2}`},
		{`{"b":1,"a":2,"b":3,"c":4,"a":5}`, `{"b": 3, "a": 5, "c": 4}`},
		{`1e400`, `Infinity`},
		{`-0`, `0`},
		{`-0.0`, `-0.0`},
		{`123456789012345678901234567890`, `123456789012345678901234567890`},
		{`9223372036854775808`, `9223372036854775808`},
		{`[1.0,1,1e0,10E-1]`, `[1.0, 1, 1.0, 1.0]`},
		{`"\ud83d\ude00"`, `"\ud83d\ude00"`},
		{`"\ud83d"`, `"\ud83d"`},
		{`"\ude00\ud83d"`, `"\ude00\ud83d"`},
		{`[NaN,Infinity,-Infinity]`, `[NaN, Infinity, -Infinity]`},
		{`{"\u0061":1,"a":2}`, `{"a": 2}`},
		{" \t\r\n1 ", `1`},
	}
	for _, tc := range tests {
		v, err := Loads([]byte(tc.in))
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		got, err := DumpsString(v, Options{})
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestLoadsTypes(t *testing.T) {
	v, err := Loads([]byte(`[1, 9223372036854775808, 1.5, "s", null, true, {}]`))
	if err != nil {
		t.Fatal(err)
	}
	l := v.([]any)
	if l[0] != int64(1) || l[2] != 1.5 || l[3] != "s" || l[4] != nil || l[5] != true {
		t.Errorf("unexpected values %#v", l)
	}
	if z, ok := l[1].(*big.Int); !ok || z.String() != "9223372036854775808" {
		t.Errorf("big int = %#v", l[1])
	}
	if _, ok := l[6].(*Dict); !ok {
		t.Errorf("object = %T", l[6])
	}
	nan, _ := Loads([]byte("NaN"))
	if !math.IsNaN(nan.(float64)) {
		t.Errorf("NaN = %v", nan)
	}
}

func TestLoadsRejects(t *testing.T) {
	bad := []string{"", " ", "01", "1.", ".5", "-", "+1", "1e", "nan", "[1,]", "[,1]", "{'a':1}", `{"a":1,}`, `{1:2}`,
		`"\x41"`, `"\u12"`, "\"a\tb\"", `"unterminated`, "1 2", "\xef\xbb\xbf{}", "\xff", `[1 2]`, `{"a" 1}`, "tru", "[", "{"}
	for _, s := range bad {
		if _, err := Loads([]byte(s)); !errors.Is(err, ErrSyntax) {
			t.Errorf("Loads(%q) error = %v, want ErrSyntax", s, err)
		}
	}
	digits := make([]byte, 4301)
	for i := range digits {
		digits[i] = '1'
	}
	if _, err := Loads(digits); !errors.Is(err, ErrIntTooLarge) {
		t.Errorf("4301-digit int error = %v", err)
	}
	if _, err := Loads(digits[:4300]); err != nil {
		t.Errorf("4300-digit int: %v", err)
	}
}

func TestLoadsDepth(t *testing.T) {
	deep := make([]byte, 0, 2*defaultMaxDepth+4)
	for range defaultMaxDepth + 1 {
		deep = append(deep, '[')
	}
	for range defaultMaxDepth + 1 {
		deep = append(deep, ']')
	}
	if _, err := Loads(deep); !errors.Is(err, ErrDepth) {
		t.Errorf("error = %v, want ErrDepth", err)
	}
}

func TestRoundTripAgainstFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/doc.json"
	src := `{"z": [1, 2.0, 1e+16], "a": {"k": "\u00e9"}}`
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := Loads(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DumpsString(v, Options{})
	if err != nil || got != src {
		t.Errorf("round trip changed the text: %q, %v", got, err)
	}
}
