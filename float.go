package pyjson

import (
	"math"
	"strconv"
)

// appendFloat appends the text Python's repr(float) produces for a finite f.
// NaN and the infinities are handled by the caller because their text depends
// on Options.DisallowNaN.
func appendFloat(dst []byte, f float64) []byte {
	if f == 0 {
		if math.Signbit(f) {
			return append(dst, "-0.0"...)
		}
		return append(dst, "0.0"...)
	}
	var tmp [32]byte
	// 'e' with precision -1 gives the shortest digits that round-trip, as
	// Python's repr does. The layout is [-]d[.ddd]e(+|-)dd[d].
	b := strconv.AppendFloat(tmp[:0], f, 'e', -1, 64)
	neg := false
	if b[0] == '-' {
		neg = true
		b = b[1:]
	}
	ePos := 0
	for i, c := range b {
		if c == 'e' {
			ePos = i
			break
		}
	}
	var digitBuf [24]byte
	digits := digitBuf[:0]
	for _, c := range b[:ePos] {
		if c != '.' {
			digits = append(digits, c)
		}
	}
	exp, _ := strconv.Atoi(string(b[ePos+1:]))
	if neg {
		dst = append(dst, '-')
	}
	if exp < -4 || exp >= 16 {
		dst = append(dst, digits[0])
		if len(digits) > 1 {
			dst = append(dst, '.')
			dst = append(dst, digits[1:]...)
		}
		dst = append(dst, 'e')
		if exp < 0 {
			dst = append(dst, '-')
			exp = -exp
		} else {
			dst = append(dst, '+')
		}
		if exp < 10 {
			dst = append(dst, '0')
		}
		return strconv.AppendInt(dst, int64(exp), 10)
	}
	decpt := exp + 1
	switch {
	case decpt <= 0:
		dst = append(dst, '0', '.')
		for i := decpt; i < 0; i++ {
			dst = append(dst, '0')
		}
		dst = append(dst, digits...)
	case decpt >= len(digits):
		dst = append(dst, digits...)
		for i := len(digits); i < decpt; i++ {
			dst = append(dst, '0')
		}
		dst = append(dst, '.', '0')
	default:
		dst = append(dst, digits[:decpt]...)
		dst = append(dst, '.')
		dst = append(dst, digits[decpt:]...)
	}
	return dst
}

// FormatFloat returns the text Python's repr(f) gives for a float, including
// "nan", "inf" and "-inf" for the non-finite values. It is not the text
// json.dumps writes for those (see Dumps).
func FormatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	return string(appendFloat(nil, f))
}

func nan() float64         { return math.NaN() }
func inf(sign int) float64 { return math.Inf(sign) }
