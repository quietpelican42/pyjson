package pyjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"math/big"
	"reflect"
	"strconv"
)

// Value is a Python-shaped JSON value. Dumps accepts more than these types
// (see the package documentation), but Loads produces only these:
//
//	nil        None
//	bool       bool
//	int64      int that fits in 64 bits
//	*big.Int   any other int
//	float64    float
//	string     str (lone surrogates are held in WTF-8 form)
//	[]any      list
//	*Dict      dict, in insertion order
type Value = any

// Dict is an insertion-ordered dictionary with Python's key equality: 1, 1.0
// and true are the same key, and setting an existing key keeps its original
// position. The zero value is not usable; call NewDict.
type Dict struct {
	keys []any
	vals []any
	idx  map[dictKey]int
}

type dictKey struct {
	kind byte
	n    uint64
	s    string
	o    any
}

// ErrUnhashableKey reports a Dict key that cannot be used as a map key.
var ErrUnhashableKey = errors.New("pyjson: unhashable dict key")

// NewDict returns an empty Dict.
func NewDict() *Dict {
	return &Dict{idx: make(map[dictKey]int)}
}

// Len returns the number of entries.
func (d *Dict) Len() int {
	if d == nil {
		return 0
	}
	return len(d.keys)
}

// Set stores val under key. If an equal key exists, its position and key
// value are kept and only the value is replaced, as Python's dict does.
// Keys may be nil, bool, integers, *big.Int, floats, strings, or any other
// comparable value (Dumps rejects the last group unless SkipKeys is set).
func (d *Dict) Set(key, val any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %T", ErrUnhashableKey, key)
		}
	}()
	dk, ok := normKey(key)
	if ok {
		if i, found := d.idx[dk]; found {
			d.vals[i] = val
			return nil
		}
		d.idx[dk] = len(d.keys)
	}
	d.keys = append(d.keys, key)
	d.vals = append(d.vals, val)
	return nil
}

// Get returns the value stored under key.
func (d *Dict) Get(key any) (val any, ok bool) {
	if d == nil {
		return nil, false
	}
	defer func() {
		if r := recover(); r != nil {
			val, ok = nil, false
		}
	}()
	dk, hashable := normKey(key)
	if !hashable {
		return nil, false
	}
	i, found := d.idx[dk]
	if !found {
		return nil, false
	}
	return d.vals[i], true
}

// Keys returns the keys in insertion order.
func (d *Dict) Keys() []any {
	if d == nil {
		return nil
	}
	return append([]any(nil), d.keys...)
}

// All iterates over the entries in insertion order.
func (d *Dict) All() iter.Seq2[any, any] {
	return func(yield func(any, any) bool) {
		if d == nil {
			return
		}
		for i, k := range d.keys {
			if !yield(k, d.vals[i]) {
				return
			}
		}
	}
}

var (
	twoTo63 = new(big.Int).Lsh(big.NewInt(1), 63)
	minI64  = new(big.Int).Neg(twoTo63)
)

func intKey(n int64) dictKey { return dictKey{kind: 'i', n: uint64(n)} }

func bigKey(z *big.Int) dictKey {
	if z.IsInt64() {
		return intKey(z.Int64())
	}
	return dictKey{kind: 'I', s: z.String()}
}

// normKey maps a key to a comparable identity following Python equality.
// ok is false for NaN, which is never equal to anything (itself included).
func normKey(k any) (dictKey, bool) {
	switch v := k.(type) {
	case nil:
		return dictKey{kind: 'n'}, true
	case string:
		return dictKey{kind: 's', s: v}, true
	case bool:
		if v {
			return intKey(1), true
		}
		return intKey(0), true
	case int64:
		return intKey(v), true
	case float64:
		return floatKey(v)
	case *big.Int:
		if v == nil {
			return dictKey{kind: 'n'}, true
		}
		return bigKey(v), true
	}
	if z, ok := integerOf(k); ok {
		return bigKey(z), true
	}
	switch v := k.(type) {
	case float32:
		return floatKey(float64(v))
	case json.Number:
		return dictKey{kind: 's', s: string(v)}, true
	}
	rv := reflect.ValueOf(k)
	switch rv.Kind() {
	case reflect.String:
		return dictKey{kind: 's', s: rv.String()}, true
	case reflect.Bool:
		return normKey(rv.Bool())
	case reflect.Float32, reflect.Float64:
		return floatKey(rv.Float())
	}
	return dictKey{kind: 'o', o: k}, true
}

func floatKey(f float64) (dictKey, bool) {
	if math.IsNaN(f) {
		return dictKey{}, false
	}
	if !math.IsInf(f, 0) && f == math.Trunc(f) {
		if f >= -(1<<63) && f < 1<<63 {
			return intKey(int64(f)), true
		}
		z, _ := new(big.Float).SetFloat64(f).Int(nil)
		return bigKey(z), true
	}
	return dictKey{kind: 'f', n: math.Float64bits(f)}, true
}

// integerOf returns k as a big.Int when k is any Go integer type, including
// named integer types.
func integerOf(k any) (*big.Int, bool) {
	switch v := k.(type) {
	case int:
		return big.NewInt(int64(v)), true
	case int8, int16, int32, int64:
		return big.NewInt(reflect.ValueOf(v).Int()), true
	case uint, uint8, uint16, uint32, uint64, uintptr:
		return new(big.Int).SetUint64(reflect.ValueOf(v).Uint()), true
	case *big.Int:
		return v, v != nil
	case big.Int:
		return &v, true
	}
	if k == nil {
		return nil, false
	}
	rv := reflect.ValueOf(k)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return big.NewInt(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return new(big.Int).SetUint64(rv.Uint()), true
	}
	return nil, false
}

// intText renders z as Python's str(int), enforcing the default 4300-digit
// limit that CPython applies when converting between int and str.
func intText(z *big.Int) (string, error) {
	if z.BitLen() > 14300 {
		return "", ErrIntTooLarge
	}
	s := z.String()
	digits := len(s)
	if z.Sign() < 0 {
		digits--
	}
	if digits > maxIntDigits {
		return "", ErrIntTooLarge
	}
	return s, nil
}

const maxIntDigits = 4300

// ErrIntTooLarge reports an integer with more than 4300 decimal digits, the
// default limit CPython enforces in int-to-str and str-to-int conversion.
var ErrIntTooLarge = errors.New("pyjson: integer exceeds the 4300-digit limit")

func appendInt64(dst []byte, n int64) []byte { return strconv.AppendInt(dst, n, 10) }
