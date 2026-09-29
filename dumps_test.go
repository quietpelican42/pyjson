package pyjson

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strings"
	"testing"
)

func sumF(a, b float64) float64 { return a + b }

func mustDict(pairs ...any) *Dict {
	d := NewDict()
	for i := 0; i < len(pairs); i += 2 {
		if err := d.Set(pairs[i], pairs[i+1]); err != nil {
			panic(err)
		}
	}
	return d
}

// The expected strings below were produced by Python 3.14.7 json.dumps.
func TestDumpsKnownOutputs(t *testing.T) {
	bigVal, _ := new(big.Int).SetString("1267650600228229401496703205376", 10)
	tests := []struct {
		name string
		v    any
		opts Options
		want string
	}{
		{"float one", 1.0, Options{}, "1.0"},
		{"float 1e16", 1e16, Options{}, "1e+16"},
		{"float 1e-5", 1e-5, Options{}, "1e-05"},
		{"float negative zero", math.Copysign(0, -1), Options{}, "-0.0"},
		{"float 0.0001", 0.0001, Options{}, "0.0001"},
		{"float 1.2345678901234568e17", 123456789012345678.0, Options{}, "1.2345678901234568e+17"},
		{"float 1.5e300", 1.5e300, Options{}, "1.5e+300"},
		{"float min subnormal", 5e-324, Options{}, "5e-324"},
		{"float last plain", 9999999999999998.0, Options{}, "9999999999999998.0"},
		{"float 1e22", 1e22, Options{}, "1e+22"},
		{"float sum", sumF(0.1, 0.2), Options{}, "0.30000000000000004"},
		{"nan", math.NaN(), Options{}, "NaN"},
		{"negative infinity", math.Inf(-1), Options{}, "-Infinity"},
		{"string ascii escapes", "h\u00e9llo \U0001f600 \x7f \x1f <>&\u2028", Options{},
			`"h\u00e9llo \ud83d\ude00 \u007f \u001f <>&\u2028"`},
		{"string utf8", "h\u00e9llo \U0001f600 \x7f \x1f <>&\u2028", Options{AllowNonASCII: true},
			"\"h\u00e9llo \U0001f600 \x7f \\u001f <>&\u2028\""},
		{"object default separators", mustDict("b", []any{1, 2.0, nil, true}, "a", NewDict()), Options{},
			`{"b": [1, 2.0, null, true], "a": {}}`},
		{"indent two", mustDict("b", []any{1, 2.0, nil, true}, "a", NewDict(), "c", []any{}), Options{Indent: IndentSpaces(2)},
			"{\n  \"b\": [\n    1,\n    2.0,\n    null,\n    true\n  ],\n  \"a\": {},\n  \"c\": []\n}"},
		{"tab indent sorted", mustDict("b", []any{1, mustDict("z", 1, "y", 2)}, "a", 1),
			Options{Indent: IndentString("\t"), SortKeys: true},
			"{\n\t\"a\": 1,\n\t\"b\": [\n\t\t1,\n\t\t{\n\t\t\t\"y\": 2,\n\t\t\t\"z\": 1\n\t\t}\n\t]\n}"},
		{"compact separators", mustDict("b", []any{1, 2}, "a", "x"), Options{Separators: &Separators{",", ":"}},
			`{"b":[1,2],"a":"x"}`},
		{"non-string keys collapse like Python", mustDict(1, "a", 2.5, "b", true, "c", nil, "d", "s", "e"), Options{},
			`{"1": "c", "2.5": "b", "null": "d", "s": "e"}`},
		{"big int", bigVal, Options{}, "1267650600228229401496703205376"},
		{"uint64 max", uint64(math.MaxUint64), Options{}, "18446744073709551615"},
		{"float32 widened", float32(0.1), Options{}, "0.10000000149011612"},
		{"go map is sorted", map[string]int{"b": 1, "a": 2}, Options{}, `{"a": 2, "b": 1}`},
		{"int keyed go map sorts numerically", map[int]string{10: "x", 2: "y"}, Options{}, `{"2": "y", "10": "x"}`},
		{"nil slice and map", []any{[]int(nil), map[string]int(nil)}, Options{}, `[[], {}]`},
		{"lone surrogate", "a\xed\xa0\x80b", Options{}, `"a\ud800b"`},
		{"indent zero", []any{1, []any{2}}, Options{Indent: IndentSpaces(0)}, "[\n1,\n[\n2\n]\n]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DumpsString(tc.v, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestDumpsErrors(t *testing.T) {
	circ := make([]any, 2)
	circ[0], circ[1] = 1, circ
	self := NewDict()
	self.Set("me", self)
	tests := []struct {
		name string
		v    any
		opts Options
		want error
	}{
		{"nan disallowed", math.NaN(), Options{DisallowNaN: true}, ErrNaN},
		{"inf nested disallowed", []any{math.Inf(1)}, Options{DisallowNaN: true}, ErrNaN},
		{"nan key disallowed", map[float64]int{math.Inf(1): 1}, Options{DisallowNaN: true}, ErrNaN},
		{"bytes", []byte("x"), Options{}, ErrUnsupportedType},
		{"chan", make(chan int), Options{}, ErrUnsupportedType},
		{"complex", 1i, Options{}, ErrUnsupportedType},
		{"tuple-like key", mustDict([1]int{1}, 1), Options{}, ErrUnsupportedType},
		{"circular slice", circ, Options{}, ErrCircular},
		{"circular dict", self, Options{}, ErrCircular},
		{"circular without check hits depth limit", self, Options{NoCircularCheck: true, MaxDepth: 50}, ErrDepth},
		{"mixed key types when sorting", mustDict("a", 1, 2, 2), Options{SortKeys: true}, ErrUnorderable},
		{"none among keys when sorting", mustDict("a", 1, nil, 2), Options{SortKeys: true}, ErrUnorderable},
		{"unsorted mix is fine but sorted is not, even with SkipKeys", mustDict("a", 1, [1]int{1}, 2), Options{SortKeys: true, SkipKeys: true}, ErrUnorderable},
		{"invalid utf8", "a\xffb", Options{}, errInvalidUTF8},
		{"invalid utf8 key", map[string]int{"\xff": 1}, Options{}, errInvalidUTF8},
		{"bad json.Number", json.Number("1e"), Options{}, ErrUnsupportedType},
		{"int over 4300 digits", new(big.Int).Exp(big.NewInt(10), big.NewInt(4300), nil), Options{}, ErrIntTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Dumps(tc.v, tc.opts)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestIntDigitLimitBoundary(t *testing.T) {
	ok := new(big.Int).Exp(big.NewInt(10), big.NewInt(4299), nil) // 4300 digits
	if _, err := Dumps(ok, Options{}); err != nil {
		t.Fatalf("4300 digits should encode: %v", err)
	}
	if _, err := Dumps(new(big.Int).Neg(ok), Options{}); err != nil {
		t.Fatalf("-(4300 digits) should encode: %v", err)
	}
}

func TestSkipKeys(t *testing.T) {
	d := mustDict("a", 1, [1]int{1}, 2)
	got, err := DumpsString(d, Options{SkipKeys: true})
	if err != nil || got != `{"a": 1}` {
		t.Fatalf("got %q, %v", got, err)
	}
	only := mustDict([1]int{1}, 2)
	got, err = DumpsString(only, Options{SkipKeys: true, SortKeys: true})
	if err != nil || got != `{}` {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestSharedButNotCircular(t *testing.T) {
	shared := []any{1, 2}
	got, err := DumpsString([]any{shared, shared}, Options{})
	if err != nil || got != "[[1, 2], [1, 2]]" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestSortKeysNumericOrder(t *testing.T) {
	big1, _ := new(big.Int).SetString("100000000000000000000", 10)
	d := mustDict(big1, 0, 2.5, 0, int64(2), 0, true, 0, -1, 0)
	got, err := DumpsString(d, Options{SortKeys: true})
	if err != nil {
		t.Fatal(err)
	}
	// Python: sorted([10**20, 2.5, 2, True, -1]) -> -1, True, 2, 2.5, 10**20
	want := `{"-1": 0, "true": 0, "2": 0, "2.5": 0, "100000000000000000000": 0}`
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestSortKeysCodePointOrder(t *testing.T) {
	// Python compares code points, so U+FFFF sorts before U+10000. That is
	// also the byte order of UTF-8, unlike the UTF-16 order RFC 8785 uses.
	got, err := DumpsString(map[string]int{"\U00010000": 1, "\uffff": 2}, Options{SortKeys: true, AllowNonASCII: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\"\uffff\": 2, \"\U00010000\": 1}"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDictSemantics(t *testing.T) {
	d := NewDict()
	d.Set("x", 1)
	d.Set(1, "one")
	d.Set(1.0, "float one")
	d.Set(true, "true")
	d.Set("x", 2)
	if d.Len() != 2 {
		t.Fatalf("Len = %d, want 2", d.Len())
	}
	if v, _ := d.Get(1); v != "true" {
		t.Errorf("Get(1) = %v", v)
	}
	keys := d.Keys()
	if keys[0] != "x" || keys[1] != 1 {
		t.Errorf("keys = %#v, want the first-inserted key objects in order", keys)
	}
	if err := d.Set([]int{1}, 1); !errors.Is(err, ErrUnhashableKey) {
		t.Errorf("slice key error = %v", err)
	}
	nan := math.NaN()
	d.Set(nan, 1)
	d.Set(nan, 2)
	if d.Len() != 4 {
		t.Errorf("NaN keys must not collide, Len = %d", d.Len())
	}
}

func TestDefaultHook(t *testing.T) {
	type custom struct{ n int }
	opts := Options{Default: func(v any) (any, error) {
		if c, ok := v.(chan int); ok {
			return cap(c), nil
		}
		return nil, errors.New("no")
	}}
	got, err := DumpsString([]any{make(chan int, 3)}, opts)
	if err != nil || got != "[3]" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := Dumps(complex(1, 2), opts); err == nil {
		t.Fatal("hook error must propagate")
	}
	ident := Options{Default: func(v any) (any, error) { return v, nil }}
	if _, err := Dumps(make(chan int), ident); err == nil {
		t.Fatal("Default returning its argument must not loop forever")
	}
	_ = custom{}
}

func TestStructs(t *testing.T) {
	type inner struct {
		A int `json:"a"`
	}
	type rec struct {
		Name    string  `json:"name"`
		Skip    int     `json:"-"`
		Opt     string  `json:"opt,omitempty"`
		Ptr     *inner  `json:"ptr"`
		List    []inner `json:"list"`
		private int
		Plain   float64
	}
	got, err := DumpsString(rec{Name: "n", Ptr: &inner{A: 1}, List: []inner{{2}}, Plain: 2}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name": "n", "ptr": {"a": 1}, "list": [{"a": 2}], "Plain": 2.0}`
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestJSONNumber(t *testing.T) {
	tests := []struct {
		in   string
		mode NumberMode
		want string
	}{
		{"100", NumberNormalize, "100"},
		{"-0", NumberNormalize, "0"},
		{"1.0e2", NumberNormalize, "100.0"},
		{"1E400", NumberNormalize, "Infinity"},
		{"123456789012345678901234567890", NumberNormalize, "123456789012345678901234567890"},
		{"1.0e2", NumberVerbatim, "1.0e2"},
		{"-0", NumberVerbatim, "-0"},
	}
	for _, tc := range tests {
		got, err := DumpsString(json.Number(tc.in), Options{Numbers: tc.mode})
		if err != nil || got != tc.want {
			t.Errorf("%s mode %d: got %q, %v; want %q", tc.in, tc.mode, got, err, tc.want)
		}
	}
}

func TestFormatFloat(t *testing.T) {
	tests := map[float64]string{1: "1.0", 1e16: "1e+16", 1e15: "1000000000000000.0", 1e-4: "0.0001", 1e-5: "1e-05", 1.5e-7: "1.5e-07",
		math.NaN(): "nan", math.Inf(1): "inf", math.Inf(-1): "-inf", 100: "100.0", 0.5: "0.5", 123.456: "123.456"}
	for in, want := range tests {
		if got := FormatFloat(in); got != want {
			t.Errorf("FormatFloat(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestLongIndentAndDepth(t *testing.T) {
	var v any = []any{}
	for range 200 {
		v = []any{v}
	}
	got, err := DumpsString(v, Options{Indent: IndentSpaces(1)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "[\n [\n  [") {
		t.Errorf("unexpected prefix %q", got[:20])
	}
	if _, err := Dumps(v, Options{MaxDepth: 100}); !errors.Is(err, ErrDepth) {
		t.Errorf("error = %v, want ErrDepth", err)
	}
}
