// Command incumbents measures Go JSON encoders against Python's json.dumps on
// the corpus written by ../gen_corpus.py.
//
//	python3 ../gen_corpus.py --out corpus
//	go run . -corpus corpus
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	dockerjson "github.com/docker/go/canonical/json"
	goccy "github.com/goccy/go-json"
	gowebpki "github.com/gowebpki/jcs"
	jsoniter "github.com/json-iterator/go"
	pj "github.com/quietpelican42/pyjson"
	segmentio "github.com/segmentio/encoding/json"
	tentjson "github.com/tent/canonical-json-go"
	ucarion "github.com/ucarion/jcs"
	virtuald "github.com/virtuald/go-pyjson"
	olympos "olympos.io/encoding/cjson"
)

type tuple struct{}

// item is one corpus value in the forms the encoders need.
type item struct {
	py       any // Python-shaped value for pyjson
	native   any // plain Go value for the incumbents (nil + ok=false when not expressible)
	ok       bool
	repr     bool // exactly expressible with Go's native types
	bestCase bool // repr and every object already in key order
	float    bool // top-level value is a finite float
	want     []*string
}

type conv struct {
	ok, repr, sorted bool
}

func (c *conv) walk(x any) (py any, native any) {
	switch v := x.(type) {
	case nil, bool:
		return v, v
	case []any:
		p := make([]any, len(v))
		n := make([]any, len(v))
		for i, e := range v {
			p[i], n[i] = c.walk(e)
		}
		return p, n
	case map[string]any:
		if s, ok := v["i"]; ok {
			z, _ := new(big.Int).SetString(s.(string), 10)
			if z.IsInt64() {
				return z.Int64(), z.Int64()
			}
			c.repr = false
			return z, z
		}
		if s, ok := v["f"]; ok {
			bits, _ := strconv.ParseUint(s.(string), 16, 64)
			f := math.Float64frombits(bits)
			if math.IsNaN(f) || math.IsInf(f, 0) {
				c.repr = false
			}
			return f, f
		}
		if s, ok := v["s"]; ok {
			return s, s
		}
		if s, ok := v["sc"]; ok {
			var b []byte
			for _, cp := range s.([]any) {
				n, _ := cp.(json.Number).Int64()
				b = appendWTF8(b, rune(n))
			}
			c.repr = false
			return string(b), string(b)
		}
		if _, ok := v["t"]; ok {
			c.repr, c.ok = false, false
			return tuple{}, tuple{}
		}
		pairs := v["d"].([]any)
		d := pj.NewDict()
		m := make(map[string]any, len(pairs))
		var prev string
		for i, p := range pairs {
			kv := p.([]any)
			kp, kn := c.walk(kv[0])
			vp, vn := c.walk(kv[1])
			d.Set(kp, vp)
			var ks string
			switch k := kn.(type) {
			case string:
				ks = k
			default:
				c.repr = false
				ks = pyKey(kn)
			}
			if i > 0 && ks < prev {
				c.sorted = false
			}
			prev = ks
			m[ks] = vn
		}
		return d, m
	}
	panic(fmt.Sprintf("bad tag %#v", x))
}

func pyKey(k any) string {
	switch v := k.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case *big.Int:
		return v.String()
	case float64:
		switch {
		case math.IsNaN(v):
			return "NaN"
		case math.IsInf(v, 1):
			return "Infinity"
		case math.IsInf(v, -1):
			return "-Infinity"
		}
		return pj.FormatFloat(v)
	}
	return fmt.Sprint(k)
}

func appendWTF8(b []byte, r rune) []byte {
	if r >= 0xD800 && r <= 0xDFFF {
		return append(b, 0xE0|byte(r>>12), 0x80|byte(r>>6)&0x3F, 0x80|byte(r)&0x3F)
	}
	return utf8.AppendRune(b, r)
}

type encoder struct {
	name string
	note string
	fn   func(v any) ([]byte, error)
	// indent post-processing: encoders have no separators or ensure_ascii
	// options, so they get the same generous stdlib Indent for indent sets.
}

var jit = jsoniter.Config{EscapeHTML: false, SortMapKeys: true}.Froze()

func stdNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func stdMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func encoders() []encoder {
	return []encoder{
		{"encoding/json (Marshal)", "", stdMarshal},
		{"encoding/json (EscapeHTML off)", "", stdNoEscape},
		{"goccy/go-json", "", func(v any) ([]byte, error) { return goccy.MarshalWithOption(v, goccy.DisableHTMLEscape()) }},
		{"json-iterator/go", "", func(v any) ([]byte, error) { return jit.Marshal(v) }},
		{"segmentio/encoding/json", "", func(v any) ([]byte, error) { return segmentio.Append(nil, v, segmentio.SortMapKeys) }},
		{"virtuald/go-pyjson", "", func(v any) ([]byte, error) { return virtuald.Marshal(v) }},
		{"ucarion/jcs (RFC 8785)", "", func(v any) ([]byte, error) {
			s, err := ucarion.Format(v)
			return []byte(s), err
		}},
		{"gowebpki/jcs (RFC 8785)", "", func(v any) ([]byte, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			return gowebpki.Transform(b)
		}},
		{"olympos.io/encoding/cjson", "", func(v any) ([]byte, error) { return olympos.Marshal(v) }},
		{"tent/canonical-json-go", "", func(v any) ([]byte, error) { return tentjson.Marshal(v) }},
		{"docker/go canonical/json", "", func(v any) ([]byte, error) { return dockerjson.Marshal(v) }},
	}
}

type optset struct {
	name   string
	kw     map[string]any
	indent *string
}

type tally struct{ all, allOK, repr, reprOK, best, bestOK, fl, flOK int }

func main() {
	dir := flag.String("corpus", "corpus", "directory holding dumps.jsonl")
	flag.Parse()
	f, err := os.Open(filepath.Join(*dir, "dumps.jsonl"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer f.Close()
	rd := bufio.NewReaderSize(f, 1<<20)
	hdrLine, _ := rd.ReadBytes('\n')
	var hdr struct {
		Optsets [][]json.RawMessage `json:"optsets"`
		Python  string              `json:"python"`
	}
	json.Unmarshal(hdrLine, &hdr)
	var sets []optset
	for _, o := range hdr.Optsets {
		var s optset
		json.Unmarshal(o[0], &s.name)
		json.Unmarshal(o[1], &s.kw)
		if v, ok := s.kw["indent"]; ok {
			var ind string
			switch x := v.(type) {
			case float64:
				ind = strings.Repeat(" ", int(x))
			case string:
				ind = x
			}
			s.indent = &ind
		}
		sets = append(sets, s)
	}
	var items []item
	for {
		line, err := rd.ReadBytes('\n')
		if len(line) > 0 {
			var rec struct {
				V json.RawMessage `json:"v"`
				X []*string       `json:"x"`
			}
			if e := json.Unmarshal(line, &rec); e != nil {
				panic(e)
			}
			dec := json.NewDecoder(bytes.NewReader(rec.V))
			dec.UseNumber()
			var raw any
			dec.Decode(&raw)
			c := conv{ok: true, repr: true, sorted: true}
			py, native := c.walk(raw)
			items = append(items, item{py: py, native: native, ok: c.ok, repr: c.repr, bestCase: c.repr && c.sorted, float: isFiniteFloat(py), want: rec.X})
		}
		if err != nil {
			break
		}
	}
	var nRepr, nBest int
	for _, it := range items {
		if it.repr {
			nRepr++
		}
		if it.bestCase {
			nBest++
		}
	}
	fmt.Printf("Python %s, %d values (%d Go-representable, %d also key-sorted)\n\n", hdr.Python, len(items), nRepr, nBest)

	type row struct {
		name string
		per  []tally
	}
	var rows []row
	names := []string{"pyjson (this package)"}
	fns := []func(any, optset) ([]byte, error){nil}
	for _, e := range encoders() {
		e := e
		names = append(names, e.name)
		fns = append(fns, func(v any, s optset) ([]byte, error) {
			b, err := e.fn(v)
			if err != nil || s.indent == nil {
				return b, err
			}
			var out bytes.Buffer
			if err := json.Indent(&out, b, "", *s.indent); err != nil {
				return nil, err
			}
			return out.Bytes(), nil
		})
	}
	for ei, name := range names {
		r := row{name: name, per: make([]tally, len(sets))}
		for si, s := range sets {
			t := &r.per[si]
			for _, it := range items {
				var got []byte
				var err error
				if ei == 0 {
					got, err = pj.Dumps(it.py, optionsFor(s))
				} else if !it.ok {
					err = fmt.Errorf("value not expressible")
				} else {
					got, err = fns[ei](it.native, s)
				}
				match := false
				if it.want[si] == nil {
					match = err != nil
				} else if err == nil {
					want, _ := base64.StdEncoding.DecodeString(*it.want[si])
					match = bytes.Equal(got, want)
				}
				t.all++
				if match {
					t.allOK++
				}
				if it.repr {
					t.repr++
					if match {
						t.reprOK++
					}
				}
				if it.float {
					t.fl++
					if match {
						t.flOK++
					}
				}
				if it.bestCase {
					t.best++
					if match {
						t.bestOK++
					}
				}
			}
		}
		rows = append(rows, r)
	}
	pct := func(a, b int) string { return fmt.Sprintf("%.1f%%", 100*float64(a)/float64(b)) }
	for _, view := range []string{"all values", "Go-representable values", "representable and already key-sorted", "top-level finite floats only"} {
		fmt.Printf("### %s\n\n| encoder |", view)
		for _, s := range sets {
			fmt.Printf(" %s |", s.name)
		}
		fmt.Printf("\n|---|")
		for range sets {
			fmt.Printf("---:|")
		}
		fmt.Println()
		for _, r := range rows {
			fmt.Printf("| %s |", r.name)
			for _, t := range r.per {
				switch view {
				case "all values":
					fmt.Printf(" %s |", pct(t.allOK, t.all))
				case "Go-representable values":
					fmt.Printf(" %s |", pct(t.reprOK, t.repr))
				case "top-level finite floats only":
					fmt.Printf(" %s |", pct(t.flOK, t.fl))
				default:
					fmt.Printf(" %s |", pct(t.bestOK, t.best))
				}
			}
			fmt.Println()
		}
		fmt.Println()
	}
}

func isFiniteFloat(v any) bool {
	f, ok := v.(float64)
	return ok && !math.IsNaN(f) && !math.IsInf(f, 0)
}

func optionsFor(s optset) pj.Options {
	var o pj.Options
	for k, v := range s.kw {
		switch k {
		case "sort_keys":
			o.SortKeys = v.(bool)
		case "skipkeys":
			o.SkipKeys = v.(bool)
		case "ensure_ascii":
			o.AllowNonASCII = !v.(bool)
		case "allow_nan":
			o.DisallowNaN = !v.(bool)
		case "indent":
			switch x := v.(type) {
			case float64:
				o.Indent = pj.IndentSpaces(int(x))
			case string:
				o.Indent = pj.IndentString(x)
			}
		case "separators":
			l := v.([]any)
			o.Separators = &pj.Separators{Item: l[0].(string), Key: l[1].(string)}
		}
	}
	return o
}
