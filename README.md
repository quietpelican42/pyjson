# pyjson

Go encoder and decoder whose JSON text is byte-identical to Python's `json.dumps` and `json.loads` round trip.

```go
d := pyjson.NewDict()
d.Set("b", 1.0)
d.Set("a", []any{1e16, 1e-5, math.Copysign(0, -1)})
out, _ := pyjson.Dumps(d, pyjson.Options{})
fmt.Println(string(out))
// {"b": 1.0, "a": [1e+16, 1e-05, -0.0]}
```

## Why

Hashes and signatures over JSON only match across services when both sides write the same bytes. Go's `encoding/json` and Python's `json.dumps` differ on:

- Floats: Go writes `1.0` as `1`, `1e16` as `10000000000000000`, `1e-5` as `0.00001` and `-0.0` as `-0`. Python writes `1.0`, `1e+16`, `1e-05` and `-0.0`.
- Separators: Python defaults to `", "` and `": "`.
- Strings: Python escapes every non-ASCII character as `\uXXXX` (surrogate pairs above U+FFFF) and DEL as `\u007f`. Go writes UTF-8 and escapes `<`, `>`, `&`.
- Key order: Python keeps insertion order. Go maps are unordered and `encoding/json` sorts them.
- `NaN` and the infinities: Python writes `NaN`, `Infinity`, `-Infinity`. Go returns an error.
- Integers: Python ints are unbounded. `sort_keys` on mixed int, float and bool keys compares numerically.

Measured: `python3 bench/gen_corpus.py --out corpus && (cd bench/incumbents && go run . -corpus ../../corpus)`. The corpus is 20,000 random values (floats of every magnitude, integers up to 300 digits, strings with controls and astral characters, nested lists and dicts, non-string dict keys) with the bytes Python 3.14.7 produced under 10 option sets. Share of values whose output is byte-identical, where a value Python rejects counts as a match if the encoder also fails:

| Encoder | default | sort_keys | compact | indent=2 | ensure_ascii=False | default, top-level floats only |
|---|---:|---:|---:|---:|---:|---:|
| **pyjson** | **100.0%** | **100.0%** | **100.0%** | **100.0%** | **100.0%** | **100.0%** |
| encoding/json (Marshal) | 45.3% | 48.8% | 47.2% | 47.2% | 57.3% | 67.6% |
| encoding/json (EscapeHTML off) | 45.8% | 49.2% | 47.7% | 47.7% | 61.0% | 67.6% |
| goccy/go-json | 45.8% | 49.2% | 47.7% | 47.7% | 61.0% | 67.6% |
| json-iterator/go | 46.0% | 49.4% | 47.9% | 47.9% | 61.2% | 70.5% |
| segmentio/encoding/json | 45.8% | 49.2% | 47.7% | 47.7% | 61.0% | 67.6% |
| virtuald/go-pyjson | 46.0% | 46.0% | 47.8% | 46.9% | 57.3% | 67.6% |
| ucarion/jcs (RFC 8785) | 35.3% | 44.3% | 36.4% | 36.4% | 51.8% | 67.6% |
| gowebpki/jcs (RFC 8785) | 41.3% | 44.8% | 42.9% | 42.9% | 57.9% | 67.6% |
| olympos.io/encoding/cjson | 29.1% | 32.5% | 29.9% | 29.9% | 45.6% | 0.0% |
| tent/canonical-json-go | 32.0% | 40.9% | 33.0% | 33.0% | 40.1% | 0.0% |
| docker/go canonical/json | 47.3% | 50.7% | 49.3% | 49.3% | 58.6% | 79.7% |

The corpus is adversarial on purpose and is not a sample of real documents. The incumbents get generous treatment: maps are converted to Go maps with Python's key-to-string rule applied, and `indent` sets get `encoding/json.Indent` applied to their output. On the subset that Go can represent exactly and whose dicts are already key-sorted (13,746 values), the best incumbent reaches 57.1% for default and 59.4% for indent=2. Full tables, including the other five option sets, are printed by the command above. The JCS packages implement RFC 8785, which is a different canonicalization from `json.dumps` (UTF-16 key order, ECMAScript number format), so a low score there is expected. `virtuald/go-pyjson` is a fork of `encoding/json` that adds NaN and Infinity.

Incumbents checked: `encoding/json`, `goccy/go-json`, `json-iterator/go`, `segmentio/encoding`, `virtuald/go-pyjson`, `ucarion/jcs`, `gowebpki/jcs`, `olympos.io/encoding/cjson`, `tent/canonical-json-go` and `docker/go/canonical/json`. A GitHub and pkg.go.dev search found no Go package that reproduces `json.dumps`. Applications carry private copies instead, for example `Agent-Field/pr-af` (`go/internal/prompts/pyjson.go`).

`go run ./examples/compare` shows one document through both encoders:

```
encoding/json: {"a":[10000000000000000,0.00001,-0],"b":1,"s":"café \u003cb\u003e"}
pyjson:        {"b": 1.0, "a": [1e+16, 1e-05, -0.0], "s": "caf\u00e9 <b>"}
```

Python 3.14.7 prints the second line for the same value.

## Installation

```
go get github.com/quietpelican42/pyjson
```

The package uses only the standard library.

## Usage

### Match Python's defaults

`Dumps(v, Options{})` equals `json.dumps(v)`. Use `*Dict` for objects when key order matters. Plain Go maps are written with keys sorted.

### Match common option sets

```go
// json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
pyjson.Dumps(v, pyjson.Options{
    SortKeys:      true,
    Separators:    &pyjson.Separators{Item: ",", Key: ":"},
    AllowNonASCII: true,
})

// json.dumps(v, indent=2)
pyjson.Dumps(v, pyjson.Options{Indent: pyjson.IndentSpaces(2)})
```

### Load and re-dump

```go
v, err := pyjson.Loads(data)   // *Dict, []any, int64 or *big.Int, float64, string, bool, nil
out, err := pyjson.Dumps(v, pyjson.Options{})
```

`Loads` then `Dumps` reproduces `json.dumps(json.loads(s))`, including big integers, duplicate keys (last value wins, first position kept) and `NaN`.

### Encode Go structs

Structs become dicts in field order. The `json` tag supplies the name, `-` skips the field and `omitempty` skips zero values. No other tag options are read.

## Reference

### Functions

| Name | Description |
|---|---|
| `Dumps(v any, opts Options) ([]byte, error)` | Encode like `json.dumps`. |
| `DumpsString(v any, opts Options) (string, error)` | `Dumps` returning a string. |
| `Loads(data []byte) (Value, error)` | Decode like `json.loads`. |
| `FormatFloat(f float64) string` | Python's `repr(float)`, including `nan`, `inf`, `-inf`. |
| `NewDict() *Dict` | Empty insertion-ordered dict. |
| `IndentSpaces(n int)`, `IndentString(s string)` | Build an `Indent`. |

`Dict` methods: `Set(key, val) error`, `Get(key) (val, ok)`, `Len()`, `Keys()`, `All()` (an `iter.Seq2`). `Value` is an alias of `any`.

### Options

The zero value is `json.dumps(v)`. Options that default to true in Python are inverted so that the zero value stays the default.

| Name | Type | Default | json.dumps argument |
|---|---|---|---|
| `SkipKeys` | `bool` | `false` | `skipkeys` |
| `AllowNonASCII` | `bool` | `false` | `ensure_ascii=False` |
| `NoCircularCheck` | `bool` | `false` | `check_circular=False` |
| `DisallowNaN` | `bool` | `false` | `allow_nan=False` |
| `SortKeys` | `bool` | `false` | `sort_keys` |
| `Indent` | `Indent` | none | `indent` (int or string) |
| `Separators` | `*Separators` | `nil` | `separators` |
| `Numbers` | `NumberMode` | `NumberNormalize` | none. `NumberVerbatim` writes a `json.Number` unchanged. |
| `Default` | `func(any) (any, error)` | `nil` | `default` |
| `MaxDepth` | `int` | 100000 | none (CPython's C stack limit) |

With `nil` separators the pair is `(", ", ": ")`, or `(",", ": ")` when `Indent` is set. `IndentSpaces(0)` gives newlines without indentation, as `indent=0` does.

### Type mapping

| Go | Python |
|---|---|
| `nil`, nil pointer | `None` |
| `bool` | `bool` |
| `int*`, `uint*`, `*big.Int`, `big.Int` | `int` (more than 4300 digits is `ErrIntTooLarge`, as in CPython) |
| `float64`, `float32` | `float` (`float32` is widened exactly) |
| `json.Number` | `int` if it has no `.`, `e` or `E`, else `float` (`NumberNormalize`) |
| `string` | `str`; invalid UTF-8 is an error; WTF-8 surrogates (`ED A0 80`) are lone surrogates |
| `[]T`, `[N]T` | `list` (a nil slice is `[]`) |
| `*Dict` | `dict` in insertion order |
| `map[K]V` | `dict` with keys sorted (a nil map is `{}`) |
| struct | `dict` of exported fields |
| pointer, interface | the value pointed to |

Dict keys follow `json.dumps`: `true` becomes `"true"`, `nil` becomes `"null"`, ints and floats become their `repr` (`1e+16`, `Infinity`). A key of another type is `ErrUnsupportedType`, or is dropped when `SkipKeys` is set. `SortKeys` orders like Python's `sorted()`: strings by code point, numbers numerically and exactly (int against float), and a mix of strings and numbers, or a `nil` key next to others, is `ErrUnorderable`. Python sorts before it drops keys, so `SkipKeys` does not rescue an unorderable mix.

### Errors

`ErrCircular`, `ErrNaN`, `ErrUnsupportedType`, `ErrUnorderable`, `ErrDepth`, `ErrIntTooLarge`, `ErrUnhashableKey` and `ErrSyntax` (from `Loads`) work with `errors.Is`.

## How it works

Floats use `strconv.AppendFloat(f, 'e', -1, 64)` for the shortest round-trip digits, the same digits as CPython's `repr`, and lay them out with Python's rule: exponent form when the decimal exponent is below -4 or at least 16, two-digit signed exponents, `.0` for integral values. Strings are escaped in one pass with Python's two escape sets (DEL is escaped only under `ensure_ascii`). Dicts hold keys with Python equality (`1`, `1.0` and `true` are one key) so a Go `*Dict` behaves like the Python dict it stands for.

Verification runs `bench/gen_corpus.py`, which builds random values and records what real Python produced. The tests rebuild each value in Go and compare bytes.

## Limitations

- Python `str` values that hold lone surrogates are carried as WTF-8 in Go strings. `Loads` rejects raw WTF-8 in its input, and output containing raw surrogates (`AllowNonASCII`) is not valid UTF-8.
- `Loads` accepts UTF-8 only. A byte order mark, UTF-16 and UTF-32 are rejected (Python's `json.loads` on `bytes` detects them).
- There are no `parse_float`, `parse_int`, `object_hook` or `cls` equivalents.
- NaN dict keys are not supported with `SortKeys` (Python's result depends on its sort algorithm). Sorting more than one unsupported key, such as two tuples, is `ErrUnorderable` where Python may succeed.
- Go maps are always written sorted, since they have no order. Use `*Dict` for insertion order.
- `[]byte`, channels, funcs and complex numbers are errors unless `Default` converts them. `Default` is not called for values the mapping already handles.
- Recursion limits differ. Python's limit depends on the platform C stack (100000 nested lists worked and 200000 failed on the test machine). The Go default is 100000 for both `Dumps` and `Loads`.
- The 4300-digit integer limit is CPython's default. Python raises or lifts it with `sys.set_int_max_str_digits`.
- Embedded struct fields are treated as ordinary named fields.

## Compatibility

Tested with Go 1.27.1 and Python 3.14.7 on macOS (arm64). The CI workflow runs the current stable Go on Linux. Other Python versions were not tested. `json.dumps` output for the covered options has been stable across CPython 3 releases, but that is not verified here.

Test results on that setup: 20,000 values times 10 option sets identical (200,000 comparisons), 14,745 `Loads` texts matching, and a second corpus of 60,000 values (seed 424242) with 600,000 comparisons and 44,035 `Loads` texts, all matching.

## Development

```
go build ./...
go vet ./...
go test -race -count=1 ./...
gofmt -l .
go test -fuzz FuzzFloat -fuzztime 30s .
go test -fuzz FuzzLoadsDumps -fuzztime 30s .
```

The differential tests need `python3` on the path and skip loudly without it. Set `PYJSON_CORPUS` to reuse a directory made by `bench/gen_corpus.py`. `go test -short` uses 2,000 values. The incumbent comparison is a separate module in `bench/incumbents` so the main module stays free of dependencies.

## License

MIT. See [LICENSE](LICENSE).
