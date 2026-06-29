package format

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// rubyFloatToS renders a float as Ruby's Float#to_s, which differs from Go's
// strconv in its special values and its exponent form: Infinity/-Infinity/NaN
// in full, a mandatory ".0" on integral values, and a two-digit exponent with
// an explicit sign (1.0e+20, 1.0e-05).
func rubyFloatToS(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case math.IsNaN(f):
		return "NaN"
	}
	// Go's 'g' with -1 precision gives the shortest round-tripping form.
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if e := strings.IndexAny(s, "eE"); e >= 0 {
		// Normalize "1e+20" / "1e-05" to Ruby's "1.0e+20" form: ensure the
		// mantissa has a decimal point and the exponent is at least two digits.
		mant := s[:e]
		exp := s[e+1:]
		if !strings.ContainsRune(mant, '.') {
			mant += ".0"
		}
		sign := "+"
		if exp[0] == '-' {
			sign = "-"
		}
		exp = exp[1:] // Go's 'g' always emits an explicit exponent sign.
		return mant + "e" + sign + exp
	}
	// No exponent: Ruby always shows a fractional part.
	if !strings.ContainsRune(s, '.') {
		s += ".0"
	}
	return s
}

// rubyInspectString renders a string as Ruby's String#inspect: wrapped in
// double quotes with the standard escapes (\n \t \r \e \a \b \f \v \\ \"),
// \u00XX for other C0/DEL control bytes, and printable runes (including
// multibyte UTF-8) left intact.
func rubyInspectString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	runes := []rune(s)
	for idx, r := range runes {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case 0x1b:
			b.WriteString(`\e`)
		case 0x07:
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\v':
			b.WriteString(`\v`)
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '#':
			// Ruby escapes '#' only when it starts an interpolation sigil
			// (#{ , #@ , #$ ); a bare '#' is left alone.
			if idx+1 < len(runes) && (runes[idx+1] == '{' || runes[idx+1] == '@' || runes[idx+1] == '$') {
				b.WriteString(`\#`)
			} else {
				b.WriteByte('#')
			}
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
