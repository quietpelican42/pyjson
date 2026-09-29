package pyjson

import (
	"fmt"
	"math/big"
	"strconv"
	"unicode/utf8"
)

// Loads parses JSON text the way Python's json.loads does and returns a
// Value. It accepts NaN, Infinity and -Infinity, keeps big integers exact,
// preserves object key order, and lets a later duplicate key replace the
// value of an earlier one while keeping the earlier position.
//
// The input must be UTF-8. A byte order mark, UTF-16 and UTF-32 input are
// rejected. Nesting deeper than 100000 levels is an error; CPython's own limit
// depends on the platform C stack (100000 worked and 200000 failed on the
// machine used for the tests).
func Loads(data []byte) (Value, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: input is not valid UTF-8", ErrSyntax)
	}
	p := &parser{s: data}
	p.ws()
	v, err := p.value(0)
	if err != nil {
		return nil, err
	}
	p.ws()
	if p.i != len(p.s) {
		return nil, p.errf("extra data")
	}
	return v, nil
}

type parser struct {
	s []byte
	i int
}

const loadsMaxDepth = defaultMaxDepth

func (p *parser) errf(format string, a ...any) error {
	return fmt.Errorf("%w: %s at offset %d", ErrSyntax, fmt.Sprintf(format, a...), p.i)
}

func (p *parser) ws() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) lit(word string) bool {
	if len(p.s)-p.i >= len(word) && string(p.s[p.i:p.i+len(word)]) == word {
		p.i += len(word)
		return true
	}
	return false
}

func (p *parser) value(depth int) (Value, error) {
	if p.i >= len(p.s) {
		return nil, p.errf("expecting value")
	}
	switch c := p.s[p.i]; {
	case c == '"':
		s, err := p.str()
		return s, err
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == 'n' && p.lit("null"):
		return nil, nil
	case c == 't' && p.lit("true"):
		return true, nil
	case c == 'f' && p.lit("false"):
		return false, nil
	case c == 'N' && p.lit("NaN"):
		return nan(), nil
	case c == 'I' && p.lit("Infinity"):
		return inf(1), nil
	case c == '-' && p.lit("-Infinity"):
		return inf(-1), nil
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	}
	return nil, p.errf("expecting value")
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func (p *parser) number() (Value, error) {
	start := p.i
	i := p.i
	if p.s[i] == '-' {
		i++
	}
	if i >= len(p.s) || !isDigit(p.s[i]) {
		return nil, p.errf("expecting value")
	}
	if p.s[i] == '0' {
		i++
	} else {
		for i < len(p.s) && isDigit(p.s[i]) {
			i++
		}
	}
	intEnd := i
	isFloat := false
	if i+1 < len(p.s) && p.s[i] == '.' && isDigit(p.s[i+1]) {
		i += 2
		for i < len(p.s) && isDigit(p.s[i]) {
			i++
		}
		isFloat = true
	}
	if i < len(p.s) && (p.s[i] == 'e' || p.s[i] == 'E') {
		j := i + 1
		if j < len(p.s) && (p.s[j] == '+' || p.s[j] == '-') {
			j++
		}
		if j < len(p.s) && isDigit(p.s[j]) {
			for j < len(p.s) && isDigit(p.s[j]) {
				j++
			}
			i = j
			isFloat = true
		}
	}
	p.i = i
	text := string(p.s[start:i])
	if isFloat {
		return parseFloat(text), nil
	}
	_ = intEnd
	if n, err := strconv.ParseInt(text, 10, 64); err == nil {
		return n, nil
	}
	digits := len(text)
	if text[0] == '-' {
		digits--
	}
	if digits > maxIntDigits {
		return nil, fmt.Errorf("%w: %w", ErrSyntax, ErrIntTooLarge)
	}
	z, _ := new(big.Int).SetString(text, 10)
	return z, nil
}

// parseFloat converts text already validated as a JSON number. Overflow
// yields an infinity, as float() does in Python.
func parseFloat(text string) float64 {
	f, _ := strconv.ParseFloat(text, 64)
	return f
}

func hex4(b []byte) (rune, bool) {
	if len(b) < 4 {
		return 0, false
	}
	var r rune
	for _, c := range b[:4] {
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, false
		}
		r = r<<4 | rune(d)
	}
	return r, true
}

func (p *parser) str() (string, error) {
	p.i++ // opening quote
	var out []byte
	start := p.i
	for {
		if p.i >= len(p.s) {
			return "", p.errf("unterminated string")
		}
		c := p.s[p.i]
		switch {
		case c == '"':
			if out == nil {
				s := string(p.s[start:p.i])
				p.i++
				return s, nil
			}
			out = append(out, p.s[start:p.i]...)
			p.i++
			return string(out), nil
		case c < 0x20:
			return "", p.errf("invalid control character")
		case c != '\\':
			p.i++
			continue
		}
		out = append(out, p.s[start:p.i]...)
		p.i++
		if p.i >= len(p.s) {
			return "", p.errf("unterminated string")
		}
		e := p.s[p.i]
		p.i++
		switch e {
		case '"', '\\', '/':
			out = append(out, e)
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			r, ok := hex4(p.s[p.i:])
			if !ok {
				return "", p.errf("invalid \\u escape")
			}
			p.i += 4
			if r >= 0xD800 && r <= 0xDBFF && p.i+1 < len(p.s) && p.s[p.i] == '\\' && p.s[p.i+1] == 'u' {
				if lo, ok := hex4(p.s[p.i+2:]); ok && lo >= 0xDC00 && lo <= 0xDFFF {
					r = 0x10000 + (r-0xD800)<<10 + (lo - 0xDC00)
					p.i += 6
				}
			}
			out = appendRuneWTF8(out, r)
		default:
			return "", p.errf("invalid escape")
		}
		start = p.i
		if out == nil {
			out = []byte{}
		}
	}
}

func (p *parser) array(depth int) (Value, error) {
	if depth >= loadsMaxDepth {
		return nil, fmt.Errorf("%w: %w", ErrSyntax, ErrDepth)
	}
	p.i++
	list := []any{}
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == ']' {
		p.i++
		return list, nil
	}
	for {
		p.ws()
		v, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		list = append(list, v)
		p.ws()
		if p.i >= len(p.s) {
			return nil, p.errf("expecting ',' delimiter")
		}
		switch p.s[p.i] {
		case ',':
			p.i++
		case ']':
			p.i++
			return list, nil
		default:
			return nil, p.errf("expecting ',' delimiter")
		}
	}
}

func (p *parser) object(depth int) (Value, error) {
	if depth >= loadsMaxDepth {
		return nil, fmt.Errorf("%w: %w", ErrSyntax, ErrDepth)
	}
	p.i++
	d := NewDict()
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == '}' {
		p.i++
		return d, nil
	}
	for {
		p.ws()
		if p.i >= len(p.s) || p.s[p.i] != '"' {
			return nil, p.errf("expecting property name enclosed in double quotes")
		}
		k, err := p.str()
		if err != nil {
			return nil, err
		}
		p.ws()
		if p.i >= len(p.s) || p.s[p.i] != ':' {
			return nil, p.errf("expecting ':' delimiter")
		}
		p.i++
		p.ws()
		v, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		if err := d.Set(k, v); err != nil {
			return nil, err
		}
		p.ws()
		if p.i >= len(p.s) {
			return nil, p.errf("expecting ',' delimiter")
		}
		switch p.s[p.i] {
		case ',':
			p.i++
		case '}':
			p.i++
			return d, nil
		default:
			return nil, p.errf("expecting ',' delimiter")
		}
	}
}

// validNumber reports whether s matches the JSON number grammar.
func validNumber(s string) bool {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	if i >= len(s) || !isDigit(s[i]) {
		return false
	}
	if s[i] == '0' {
		i++
	} else {
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	if i < len(s) && s[i] == '.' {
		i++
		if i >= len(s) || !isDigit(s[i]) {
			return false
		}
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if i >= len(s) || !isDigit(s[i]) {
			return false
		}
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	return i == len(s)
}
