package pyjson

import "errors"

// Indent selects pretty-printing, mirroring json.dumps(indent=...). The zero
// value means no indentation and no newlines (indent=None).
type Indent struct {
	set bool
	s   string
}

// IndentSpaces indents with n spaces per level. Zero (or a negative n) puts
// each element on its own line without indentation, as indent=0 does.
func IndentSpaces(n int) Indent {
	if n < 0 {
		n = 0
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return Indent{set: true, s: string(b)}
}

// IndentString indents with s per level, like json.dumps(indent="\t").
func IndentString(s string) Indent { return Indent{set: true, s: s} }

// Separators is the (item_separator, key_separator) pair of json.dumps.
type Separators struct {
	Item string
	Key  string
}

// NumberMode selects how json.Number values are written.
type NumberMode int

const (
	// NumberNormalize writes a json.Number the way Python would after
	// json.loads: integer text becomes an int, anything else becomes the
	// repr of the parsed float64 (1.0e2 becomes 100.0). This is the default.
	NumberNormalize NumberMode = iota
	// NumberVerbatim validates a json.Number as a JSON number and writes its
	// text unchanged. The result differs from Python for floats.
	NumberVerbatim
)

// Options mirrors the keyword arguments of json.dumps. The zero value is
// Python's default call, json.dumps(v). Boolean options that default to true
// in Python are inverted here so that the zero value stays the default.
//
//	Options field     json.dumps argument
//	SkipKeys          skipkeys
//	AllowNonASCII     ensure_ascii=False
//	NoCircularCheck   check_circular=False
//	DisallowNaN       allow_nan=False
//	Indent            indent
//	Separators        separators
//	SortKeys          sort_keys
//	Default           default
type Options struct {
	SkipKeys        bool
	AllowNonASCII   bool
	NoCircularCheck bool
	DisallowNaN     bool
	SortKeys        bool
	Indent          Indent
	// Separators overrides the separators. When nil they are (", ", ": ")
	// without indentation and (",", ": ") with it.
	Separators *Separators
	// Numbers controls json.Number output.
	Numbers NumberMode
	// Default, if set, is called for a value Dumps cannot encode and returns
	// a replacement, like the default argument of json.dumps.
	Default func(v any) (any, error)
	// MaxDepth bounds container nesting. Zero means 100000, near what CPython
	// allows on a typical platform. It also stops cycles when NoCircularCheck is set.
	MaxDepth int
}

var (
	// ErrCircular reports a container that contains itself.
	ErrCircular = errors.New("pyjson: circular reference detected")
	// ErrNaN reports NaN or an infinity when DisallowNaN is set.
	ErrNaN = errors.New("pyjson: out of range float values are not JSON compliant")
	// ErrUnsupportedType reports a value or dict key of a type json.dumps
	// would reject.
	ErrUnsupportedType = errors.New("pyjson: object is not JSON serializable")
	// ErrUnorderable reports SortKeys on keys that Python cannot compare
	// (a mix of strings and numbers, for example).
	ErrUnorderable = errors.New("pyjson: dict keys cannot be sorted")
	// ErrDepth reports nesting deeper than MaxDepth.
	ErrDepth = errors.New("pyjson: maximum nesting depth exceeded")
	// ErrSyntax reports malformed JSON text in Loads.
	ErrSyntax = errors.New("pyjson: invalid JSON")
)
