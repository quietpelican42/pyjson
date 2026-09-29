package pyjson

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The differential tests compare against a corpus that bench/gen_corpus.py
// builds with real Python. Set PYJSON_CORPUS to a directory that already holds
// dumps.jsonl and loads.jsonl to reuse one; otherwise the corpus is generated
// into a temporary directory. They skip when python3 is unavailable.

type tupleKey struct{ n int }

func corpusDir(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("PYJSON_CORPUS"); d != "" {
		return d
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found: install it or set PYJSON_CORPUS to a directory made by bench/gen_corpus.py")
	}
	dir := t.TempDir()
	n := "20000"
	if testing.Short() {
		n = "2000"
	}
	out, err := exec.Command(py, filepath.Join("bench", "gen_corpus.py"), "--out", dir, "--n", n).CombinedOutput()
	if err != nil {
		t.Fatalf("generating corpus: %v\n%s", err, out)
	}
	return dir
}

func untag(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	var probe any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&probe); err != nil {
		t.Fatal(err)
	}
	return untagValue(t, probe)
}

func untagValue(t *testing.T, x any) any {
	switch v := x.(type) {
	case nil, bool:
		return v
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = untagValue(t, e)
		}
		return out
	case map[string]any:
		if s, ok := v["i"]; ok {
			z, ok := new(big.Int).SetString(s.(string), 10)
			if !ok {
				t.Fatalf("bad int %v", s)
			}
			if z.IsInt64() {
				return z.Int64()
			}
			return z
		}
		if s, ok := v["f"]; ok {
			var bits uint64
			fmt.Sscanf(s.(string), "%x", &bits)
			return math.Float64frombits(bits)
		}
		if s, ok := v["s"]; ok {
			return s.(string)
		}
		if s, ok := v["sc"]; ok {
			var b []byte
			for _, c := range s.([]any) {
				n, _ := c.(json.Number).Int64()
				b = appendRuneWTF8(b, rune(n))
			}
			return string(b)
		}
		if s, ok := v["t"]; ok {
			n, _ := s.(json.Number).Int64()
			return tupleKey{int(n)}
		}
		if pairs, ok := v["d"]; ok {
			d := NewDict()
			for _, p := range pairs.([]any) {
				kv := p.([]any)
				if err := d.Set(untagValue(t, kv[0]), untagValue(t, kv[1])); err != nil {
					t.Fatal(err)
				}
			}
			return d
		}
	}
	t.Fatalf("unknown tag %#v", x)
	return nil
}

func optionsFrom(t *testing.T, kw map[string]any) Options {
	var o Options
	for k, v := range kw {
		switch k {
		case "sort_keys":
			o.SortKeys = v.(bool)
		case "skipkeys":
			o.SkipKeys = v.(bool)
		case "ensure_ascii":
			o.AllowNonASCII = !v.(bool)
		case "allow_nan":
			o.DisallowNaN = !v.(bool)
		case "check_circular":
			o.NoCircularCheck = !v.(bool)
		case "indent":
			switch x := v.(type) {
			case float64:
				o.Indent = IndentSpaces(int(x))
			case string:
				o.Indent = IndentString(x)
			}
		case "separators":
			l := v.([]any)
			o.Separators = &Separators{Item: l[0].(string), Key: l[1].(string)}
		default:
			t.Fatalf("unmapped option %q", k)
		}
	}
	return o
}

func scanLines(t *testing.T, path string, fn func(line []byte)) {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := sc.ReadBytes('\n')
		if len(line) > 0 {
			fn(line)
		}
		if err != nil {
			return
		}
	}
}

func b64dec(t *testing.T, s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDumpsMatchesPython(t *testing.T) {
	dir := corpusDir(t)
	var header struct {
		Optsets [][]json.RawMessage `json:"optsets"`
		Python  string              `json:"python"`
	}
	first := true
	var names []string
	var opts []Options
	total := make([]int, 0)
	fails := make([]int, 0)
	errAgree := 0
	shown := 0
	scanLines(t, filepath.Join(dir, "dumps.jsonl"), func(line []byte) {
		if first {
			first = false
			if err := json.Unmarshal(line, &header); err != nil {
				t.Fatal(err)
			}
			for _, os := range header.Optsets {
				var name string
				var kw map[string]any
				json.Unmarshal(os[0], &name)
				json.Unmarshal(os[1], &kw)
				names = append(names, name)
				opts = append(opts, optionsFrom(t, kw))
			}
			total = make([]int, len(names))
			fails = make([]int, len(names))
			return
		}
		var rec struct {
			V json.RawMessage `json:"v"`
			X []*string       `json:"x"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		v := untag(t, rec.V)
		for i, o := range opts {
			total[i]++
			got, err := Dumps(v, o)
			var want []byte
			if rec.X[i] != nil {
				want = b64dec(t, *rec.X[i])
			}
			switch {
			case rec.X[i] == nil && err != nil:
				errAgree++
			case rec.X[i] == nil && err == nil:
				fails[i]++
				if shown < 5 {
					shown++
					t.Errorf("%s: Python raised, Go returned %.200q", names[i], got)
				}
			case err != nil:
				fails[i]++
				if shown < 5 {
					shown++
					t.Errorf("%s: Go failed (%v), Python wrote %.200q", names[i], err, want)
				}
			case !bytes.Equal(got, want):
				fails[i]++
				if shown < 5 {
					shown++
					t.Errorf("%s:\n got  %.300q\n want %.300q", names[i], got, want)
				}
			}
		}
	})
	for i, n := range names {
		t.Logf("python %s  %-30s %d/%d identical", header.Python, n, total[i]-fails[i], total[i])
	}
	t.Logf("both sides rejected the value in %d cases", errAgree)
}

func TestLoadsMatchesPython(t *testing.T) {
	dir := corpusDir(t)
	total, fails, accepted, rejected := 0, 0, 0, 0
	shown := 0
	scanLines(t, filepath.Join(dir, "loads.jsonl"), func(line []byte) {
		var rec struct {
			T string  `json:"t"`
			X *string `json:"x"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		text := b64dec(t, rec.T)
		total++
		v, err := Loads(text)
		var got []byte
		if err == nil {
			got, err = Dumps(v, Options{})
		}
		switch {
		case rec.X == nil && err != nil:
			rejected++
			return
		case rec.X == nil:
			// Python raised at loads or at the re-dump.
			fails++
			if shown < 8 {
				shown++
				t.Errorf("Python rejected %.120q, Go produced %.120q", text, got)
			}
		case err != nil:
			fails++
			if shown < 8 {
				shown++
				t.Errorf("Go rejected %.120q (%v), Python accepted", text, err)
			}
		case !bytes.Equal(got, b64dec(t, *rec.X)):
			fails++
			if shown < 8 {
				shown++
				t.Errorf("text %.120q\n got  %.200q\n want %.200q", text, got, b64dec(t, *rec.X))
			}
		default:
			accepted++
		}
	})
	t.Logf("%d texts: %d identical after loads+dumps, %d rejected by both, %d mismatches", total, accepted, rejected, fails)
}
