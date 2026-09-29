package pyjson

import (
	"errors"
	"unicode/utf8"
)

const hexDigits = "0123456789abcdef"

var errInvalidUTF8 = errors.New("invalid UTF-8 in string")

// decodeRune decodes the code point at the start of s. It accepts the
// WTF-8 form of a lone surrogate (ED A0..BF 80..BF), which is how this package
// represents Python strings that contain surrogate code points.
func decodeRune(s string) (rune, int, bool) {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size <= 1 {
		if len(s) >= 3 && s[0] == 0xED && s[1]&0xE0 == 0xA0 && s[2]&0xC0 == 0x80 {
			return 0xD000 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3, true
		}
		return 0, 1, false
	}
	return r, size, true
}

func appendU(dst []byte, r rune) []byte {
	return append(dst, '\\', 'u', hexDigits[r>>12&15], hexDigits[r>>8&15], hexDigits[r>>4&15], hexDigits[r&15])
}

// appendString appends s as a JSON string literal the way json.dumps does.
func appendString(dst []byte, s string, ensureASCII bool) ([]byte, error) {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c >= 0x20 && c < 0x7f && c != '"' && c != '\\' {
			i++
			continue
		}
		if c >= 0x80 {
			r, size, ok := decodeRune(s[i:])
			if !ok {
				return dst, errInvalidUTF8
			}
			if !ensureASCII {
				i += size
				continue
			}
			dst = append(dst, s[start:i]...)
			if r >= 0x10000 {
				r -= 0x10000
				dst = appendU(dst, 0xD800+(r>>10))
				dst = appendU(dst, 0xDC00+(r&0x3FF))
			} else {
				dst = appendU(dst, r)
			}
			i += size
			start = i
			continue
		}
		// ASCII byte that needs an escape. DEL is only escaped when
		// ensureASCII is set, matching Python's two escape patterns.
		if c == 0x7f && !ensureASCII {
			i++
			continue
		}
		dst = append(dst, s[start:i]...)
		switch c {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		default:
			dst = appendU(dst, rune(c))
		}
		i++
		start = i
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"'), nil
}

// appendRuneWTF8 appends r as UTF-8, encoding surrogate code points as the
// three-byte WTF-8 sequence instead of U+FFFD.
func appendRuneWTF8(dst []byte, r rune) []byte {
	if r >= 0xD800 && r <= 0xDFFF {
		return append(dst, 0xE0|byte(r>>12), 0x80|byte(r>>6)&0x3F, 0x80|byte(r)&0x3F)
	}
	return utf8.AppendRune(dst, r)
}
