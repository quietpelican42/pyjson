package pyjson

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strings"
)

// Comparison classes: Python can order values only within a class.
const (
	classNum = iota
	classStr
	classNone
	classOther
)

type sortItem struct {
	cls int
	s   string
	i   int64
	b   *big.Int // set when the value is an integer outside int64
	f   float64
	isF bool
	ent entry
}

func classify(en entry) (sortItem, bool) {
	it := sortItem{ent: en}
	switch k := en.key.(type) {
	case nil:
		it.cls = classNone
		return it, true
	case string:
		it.cls, it.s = classStr, k
		return it, true
	case bool:
		it.cls = classNum
		if k {
			it.i = 1
		}
		return it, true
	case float64:
		it.cls, it.f, it.isF = classNum, k, true
		return it, !math.IsNaN(k)
	case float32:
		it.cls, it.f, it.isF = classNum, float64(k), true
		return it, !math.IsNaN(float64(k))
	}
	if z, ok := integerOf(en.key); ok {
		it.cls = classNum
		if z.IsInt64() {
			it.i = z.Int64()
		} else {
			it.b = z
		}
		return it, true
	}
	if s, ok := stringLike(en.key); ok {
		it.cls, it.s = classStr, s
		return it, true
	}
	if f, ok := floatLike(en.key); ok {
		it.cls, it.f, it.isF = classNum, f, true
		return it, !math.IsNaN(f)
	}
	it.cls = classOther
	return it, true
}

func cmpNum(a, b sortItem) int {
	switch {
	case !a.isF && !b.isF && a.b == nil && b.b == nil:
		switch {
		case a.i < b.i:
			return -1
		case a.i > b.i:
			return 1
		}
		return 0
	case a.isF && b.isF:
		switch {
		case a.f < b.f:
			return -1
		case a.f > b.f:
			return 1
		}
		return 0
	}
	return toBigFloat(a).Cmp(toBigFloat(b))
}

// toBigFloat converts a numeric item exactly. Infinities are valid big.Float
// values, so int-versus-inf comparisons still work.
func toBigFloat(it sortItem) *big.Float {
	switch {
	case it.isF:
		return new(big.Float).SetFloat64(it.f)
	case it.b != nil:
		return new(big.Float).SetInt(it.b)
	}
	return new(big.Float).SetInt64(it.i)
}

// sortEntries orders entries by key the way Python's sorted() orders dict
// items, and fails where Python raises TypeError.
func sortEntries(entries []entry) error {
	if len(entries) < 2 {
		return nil
	}
	items := make([]sortItem, len(entries))
	for i, en := range entries {
		it, ok := classify(en)
		if !ok {
			return fmt.Errorf("%w: NaN key", ErrUnorderable)
		}
		items[i] = it
		if it.cls != items[0].cls {
			return fmt.Errorf("%w: mixed key types", ErrUnorderable)
		}
	}
	if items[0].cls == classNone || items[0].cls == classOther {
		return fmt.Errorf("%w: key type %T", ErrUnorderable, items[0].ent.key)
	}
	slices.SortStableFunc(items, func(a, b sortItem) int {
		if a.cls == classStr {
			return strings.Compare(a.s, b.s)
		}
		return cmpNum(a, b)
	})
	for i := range items {
		entries[i] = items[i].ent
	}
	return nil
}

func stringLike(k any) (string, bool) {
	rv := reflect.ValueOf(k)
	if rv.Kind() == reflect.String {
		return rv.String(), true
	}
	return "", false
}

func floatLike(k any) (float64, bool) {
	rv := reflect.ValueOf(k)
	if rv.Kind() == reflect.Float32 || rv.Kind() == reflect.Float64 {
		return rv.Float(), true
	}
	return 0, false
}
