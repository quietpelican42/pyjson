// Package pyjson writes JSON byte-for-byte the way Python's json.dumps does,
// so hashes and signatures computed over the text match across Go and Python.
//
// Go values map onto Python values as follows:
//
//	nil, nil pointer                None
//	bool                            bool
//	int*, uint*, *big.Int, big.Int  int (unbounded; more than 4300 digits is an error)
//	float32, float64                float (float32 is widened exactly)
//	json.Number                     see Options.Numbers
//	string                          str (invalid UTF-8 is an error)
//	[]T, [N]T                       list (nil slice is [])
//	*Dict                           dict in insertion order
//	map[K]V                         dict, entries sorted by key (nil map is {})
//	struct                          dict of exported fields, json tag names honored
//	pointer, interface              the value pointed to
//
// []byte, channels, funcs and complex numbers are not serializable; use
// Options.Default to convert them.
package pyjson

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strings"
)

// Dumps returns the JSON text Python's json.dumps(v, ...) would produce for
// the Python value equivalent to v.
func Dumps(v any, opts Options) ([]byte, error) {
	e := &encoder{opts: opts}
	e.ensureASCII = !opts.AllowNonASCII
	e.itemSep, e.keySep = ", ", ": "
	if opts.Indent.set {
		e.itemSep = ","
		e.indent = opts.Indent.s
		e.hasIndent = true
	}
	if opts.Separators != nil {
		e.itemSep, e.keySep = opts.Separators.Item, opts.Separators.Key
	}
	e.maxDepth = opts.MaxDepth
	if e.maxDepth <= 0 {
		e.maxDepth = defaultMaxDepth
	}
	if !opts.NoCircularCheck {
		e.seen = make(map[seenKey]struct{})
	}
	if err := e.encode(v, 0); err != nil {
		return nil, err
	}
	return e.buf, nil
}

// DumpsString is Dumps returning a string.
func DumpsString(v any, opts Options) (string, error) {
	b, err := Dumps(v, opts)
	return string(b), err
}

const defaultMaxDepth = 100000

type seenKey struct {
	ptr uintptr
	n   int
}

type encoder struct {
	opts        Options
	buf         []byte
	ensureASCII bool
	itemSep     string
	keySep      string
	indent      string
	hasIndent   bool
	maxDepth    int
	seen        map[seenKey]struct{}
}

func (e *encoder) newline(level int) {
	e.buf = append(e.buf, '\n')
	for range level {
		e.buf = append(e.buf, e.indent...)
	}
}

func (e *encoder) float(f float64) error {
	switch {
	case math.IsNaN(f), math.IsInf(f, 0):
		if e.opts.DisallowNaN {
			return fmt.Errorf("%w: %s", ErrNaN, FormatFloat(f))
		}
		switch {
		case math.IsNaN(f):
			e.buf = append(e.buf, "NaN"...)
		case f > 0:
			e.buf = append(e.buf, "Infinity"...)
		default:
			e.buf = append(e.buf, "-Infinity"...)
		}
	default:
		e.buf = appendFloat(e.buf, f)
	}
	return nil
}

func (e *encoder) bigInt(z *big.Int) error {
	s, err := intText(z)
	if err != nil {
		return err
	}
	e.buf = append(e.buf, s...)
	return nil
}

func (e *encoder) str(s string) error {
	var err error
	e.buf, err = appendString(e.buf, s, e.ensureASCII)
	return err
}

func (e *encoder) enter(ptr uintptr, n int) (func(), error) {
	if e.seen == nil || n == 0 || ptr == 0 {
		return func() {}, nil
	}
	k := seenKey{ptr, n}
	if _, dup := e.seen[k]; dup {
		return nil, ErrCircular
	}
	e.seen[k] = struct{}{}
	return func() { delete(e.seen, k) }, nil
}

func (e *encoder) encode(v any, depth int) error {
	switch x := v.(type) {
	case nil:
		e.buf = append(e.buf, "null"...)
	case bool:
		if x {
			e.buf = append(e.buf, "true"...)
		} else {
			e.buf = append(e.buf, "false"...)
		}
	case string:
		return e.str(x)
	case int:
		e.buf = appendInt64(e.buf, int64(x))
	case int64:
		e.buf = appendInt64(e.buf, x)
	case float64:
		return e.float(x)
	case float32:
		return e.float(float64(x))
	case *big.Int:
		if x == nil {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		return e.bigInt(x)
	case big.Int:
		return e.bigInt(&x)
	case json.Number:
		return e.number(x)
	case *Dict:
		if x == nil {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		return e.dict(x, depth)
	case []any:
		return e.list(reflect.ValueOf(x), depth)
	default:
		return e.reflectValue(reflect.ValueOf(v), v, depth)
	}
	return nil
}

func (e *encoder) number(n json.Number) error {
	s := string(n)
	if !validNumber(s) {
		return fmt.Errorf("%w: invalid json.Number %q", ErrUnsupportedType, s)
	}
	if e.opts.Numbers == NumberVerbatim {
		e.buf = append(e.buf, s...)
		return nil
	}
	if !strings.ContainsAny(s, ".eE") {
		z, ok := new(big.Int).SetString(s, 10)
		if !ok {
			return fmt.Errorf("%w: invalid json.Number %q", ErrUnsupportedType, s)
		}
		return e.bigInt(z)
	}
	return e.float(parseFloat(s))
}

func (e *encoder) reflectValue(rv reflect.Value, v any, depth int) error {
	if !rv.IsValid() {
		e.buf = append(e.buf, "null"...)
		return nil
	}
	switch rv.Kind() {
	case reflect.Bool:
		return e.encode(rv.Bool(), depth)
	case reflect.String:
		return e.str(rv.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.buf = appendInt64(e.buf, rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return e.bigInt(new(big.Int).SetUint64(rv.Uint()))
	case reflect.Float32, reflect.Float64:
		return e.float(rv.Float())
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		if rv.Kind() == reflect.Pointer {
			leave, err := e.enter(rv.Pointer(), 1)
			if err != nil {
				return err
			}
			defer leave()
			if depth >= e.maxDepth {
				return ErrDepth
			}
			return e.encode(rv.Elem().Interface(), depth)
		}
		return e.encode(rv.Elem().Interface(), depth)
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
			return e.fallback(v, depth, rv)
		}
		return e.list(rv, depth)
	case reflect.Map:
		return e.goMap(rv, depth)
	case reflect.Struct:
		return e.structValue(rv, depth)
	}
	return e.fallback(v, depth, rv)
}

func (e *encoder) fallback(v any, depth int, rv reflect.Value) error {
	if e.opts.Default == nil {
		return fmt.Errorf("%w: %T", ErrUnsupportedType, v)
	}
	if depth >= e.maxDepth {
		return ErrDepth
	}
	r, err := e.opts.Default(v)
	if err != nil {
		return err
	}
	if reflect.TypeOf(r) == reflect.TypeOf(v) && reflect.TypeOf(r) != nil && reflect.TypeOf(r).Comparable() && any(r) == v {
		return fmt.Errorf("%w: Default returned its argument for %T", ErrCircular, v)
	}
	return e.encode(r, depth+1)
}

func (e *encoder) list(rv reflect.Value, depth int) error {
	n := rv.Len()
	if n == 0 {
		e.buf = append(e.buf, '[', ']')
		return nil
	}
	if depth >= e.maxDepth {
		return ErrDepth
	}
	if rv.Kind() == reflect.Slice {
		leave, err := e.enter(rv.Pointer(), n)
		if err != nil {
			return err
		}
		defer leave()
	}
	e.buf = append(e.buf, '[')
	for i := 0; i < n; i++ {
		if i > 0 {
			e.buf = append(e.buf, e.itemSep...)
		}
		if e.hasIndent {
			e.newline(depth + 1)
		}
		if err := e.encode(rv.Index(i).Interface(), depth+1); err != nil {
			return err
		}
	}
	if e.hasIndent {
		e.newline(depth)
	}
	e.buf = append(e.buf, ']')
	return nil
}

type entry struct {
	key any
	ks  string
	val any
}

func (e *encoder) dict(d *Dict, depth int) error {
	if len(d.keys) == 0 {
		e.buf = append(e.buf, '{', '}')
		return nil
	}
	if depth >= e.maxDepth {
		return ErrDepth
	}
	leave, err := e.enter(reflect.ValueOf(d).Pointer(), 1)
	if err != nil {
		return err
	}
	defer leave()
	entries := make([]entry, 0, len(d.keys))
	for i, k := range d.keys {
		entries = append(entries, entry{key: k, val: d.vals[i]})
	}
	return e.entries(entries, depth, e.opts.SortKeys, false)
}

func (e *encoder) goMap(rv reflect.Value, depth int) error {
	n := rv.Len()
	if n == 0 {
		e.buf = append(e.buf, '{', '}')
		return nil
	}
	if depth >= e.maxDepth {
		return ErrDepth
	}
	leave, err := e.enter(rv.Pointer(), n)
	if err != nil {
		return err
	}
	defer leave()
	entries := make([]entry, 0, n)
	it := rv.MapRange()
	for it.Next() {
		entries = append(entries, entry{key: it.Key().Interface(), val: it.Value().Interface()})
	}
	// Go maps have no order, so they are always sorted.
	return e.entries(entries, depth, true, !e.opts.SortKeys)
}

// entries writes the members of a dict. When sortKeys is set the entries are
// ordered as Python's sorted() would order them; lenient makes an unorderable
// mix fall back to ordering by key text instead of failing.
func (e *encoder) entries(entries []entry, depth int, sortKeys, lenient bool) error {
	// Python sorts every item first and only then drops keys it cannot
	// serialize, so an unorderable mix fails even under SkipKeys.
	if sortKeys {
		if err := sortEntries(entries); err != nil {
			if !lenient {
				return err
			}
			for i := range entries {
				entries[i].ks, _, _ = e.keyString(entries[i].key)
			}
			slices.SortStableFunc(entries, func(a, b entry) int { return strings.Compare(a.ks, b.ks) })
		}
	}
	kept := entries[:0]
	for _, en := range entries {
		ks, ok, err := e.keyString(en.key)
		if err != nil {
			return err
		}
		if !ok {
			if e.opts.SkipKeys {
				continue
			}
			return fmt.Errorf("%w: dict key of type %T", ErrUnsupportedType, en.key)
		}
		en.ks = ks
		kept = append(kept, en)
	}
	entries = kept
	if len(entries) == 0 {
		e.buf = append(e.buf, '{', '}')
		return nil
	}
	e.buf = append(e.buf, '{')
	for i, en := range entries {
		if i > 0 {
			e.buf = append(e.buf, e.itemSep...)
		}
		if e.hasIndent {
			e.newline(depth + 1)
		}
		var err error
		e.buf, err = appendString(e.buf, en.ks, e.ensureASCII)
		if err != nil {
			return err
		}
		e.buf = append(e.buf, e.keySep...)
		if err := e.encode(en.val, depth+1); err != nil {
			return err
		}
	}
	if e.hasIndent {
		e.newline(depth)
	}
	e.buf = append(e.buf, '}')
	return nil
}

// keyString converts a dict key to its JSON text the way json.dumps does.
// ok is false for key types Python refuses (SkipKeys drops those).
func (e *encoder) keyString(k any) (string, bool, error) {
	switch x := k.(type) {
	case nil:
		return "null", true, nil
	case string:
		return x, true, nil
	case bool:
		if x {
			return "true", true, nil
		}
		return "false", true, nil
	case float64:
		return e.floatKey(x)
	case float32:
		return e.floatKey(float64(x))
	case json.Number:
		var sub encoder
		sub.opts = Options{Numbers: e.opts.Numbers, DisallowNaN: e.opts.DisallowNaN}
		if err := sub.number(x); err != nil {
			return "", false, err
		}
		return string(sub.buf), true, nil
	}
	if z, ok := integerOf(k); ok {
		s, err := intText(z)
		return s, err == nil, err
	}
	rv := reflect.ValueOf(k)
	switch rv.Kind() {
	case reflect.String:
		return rv.String(), true, nil
	case reflect.Bool:
		return e.keyString(rv.Bool())
	case reflect.Float32, reflect.Float64:
		return e.floatKey(rv.Float())
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return "null", true, nil
		}
	}
	return "", false, nil
}

func (e *encoder) floatKey(f float64) (string, bool, error) {
	var sub encoder
	sub.opts = e.opts
	if err := sub.float(f); err != nil {
		return "", false, err
	}
	return string(sub.buf), true, nil
}

type field struct {
	name      string
	index     int
	omitEmpty bool
}

func structFields(t reflect.Type) []field {
	var fs []field
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		name, opt, _ := strings.Cut(sf.Tag.Get("json"), ",")
		if name == "-" && opt == "" {
			continue
		}
		if name == "" {
			name = sf.Name
		}
		fs = append(fs, field{name: name, index: i, omitEmpty: strings.Contains(","+opt+",", ",omitempty,")})
	}
	return fs
}

func (e *encoder) structValue(rv reflect.Value, depth int) error {
	d := NewDict()
	for _, f := range structFields(rv.Type()) {
		fv := rv.Field(f.index)
		if f.omitEmpty && fv.IsZero() {
			continue
		}
		if err := d.Set(f.name, fv.Interface()); err != nil {
			return err
		}
	}
	if d.Len() == 0 {
		e.buf = append(e.buf, '{', '}')
		return nil
	}
	if depth >= e.maxDepth {
		return ErrDepth
	}
	entries := make([]entry, 0, d.Len())
	for i, k := range d.keys {
		entries = append(entries, entry{key: k, val: d.vals[i]})
	}
	return e.entries(entries, depth, e.opts.SortKeys, false)
}
