package format

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// twosComplementDigits renders a negative integer in Ruby's two's-complement
// "dot" notation for an unsigned base conversion (x/o/b): the leading-ones
// continuation is abbreviated as ".." followed by the digit string of
// b^k - |z| for the smallest k whose top digit is (base-1). For example -255 in
// base 16 is "..f01" (16^3-255 = 0xf01). The returned string includes the ".."
// prefix.
func twosComplementDigits(z *big.Int, base int) string {
	v := new(big.Int).Abs(z)
	bigBase := big.NewInt(int64(base))
	// Find the smallest k with |z| <= base^(k-1); then base^k - |z| has exactly
	// k digits with a leading (base-1) digit.
	k := 1
	pow := new(big.Int).Set(bigBase) // base^k, starts at base^1
	for {
		// base^(k-1) >= v ?
		prev := new(big.Int).Quo(pow, bigBase) // base^(k-1)
		if prev.Cmp(v) >= 0 {
			break
		}
		k++
		pow.Mul(pow, bigBase)
	}
	rep := new(big.Int).Sub(pow, v) // base^k - v
	digits := rep.Text(base)
	// Guarantee exactly k digits (Text never adds leading zeros, and by
	// construction the top digit is base-1, so len == k already).
	return ".." + digits
}

// twosComplementPrecision applies a precision (minimum displayed-digit count,
// not counting the "..") to a two's-complement rendering, padding by repeating
// the leading (base-1) digit. digits includes the ".." prefix.
func twosComplementPrecision(digits string, base, prec int) string {
	body := strings.TrimPrefix(digits, "..")
	// MRI counts the two ".." characters toward the precision, so the displayed
	// digit run is padded to (prec-2) digits.
	want := prec - 2
	if len(body) >= want {
		return digits
	}
	lead := string(lowDigit(base - 1))
	return ".." + strings.Repeat(lead, want-len(body)) + body
}

// leadDigit is the fill digit used when zero-padding a two's-complement value:
// the all-ones digit (base-1), in the case matching the conversion.
func leadDigit(base int, upper bool) string {
	d := string(lowDigit(base - 1))
	if upper {
		return strings.ToUpper(d)
	}
	return d
}

// lowDigit maps a 0..15 digit value to its lowercase base-36 character.
func lowDigit(n int) byte {
	if n < 10 {
		return byte('0' + n)
	}
	return byte('a' + n - 10)
}

// altPrefix returns the alternate-form (# flag) radix prefix for a base, or ""
// when the value is zero (MRI omits 0x/0b for a zero magnitude; octal's "0" is
// supplied by the digits themselves). upper selects the uppercase X/B prefix.
func altPrefix(base int, upper bool, nonzero bool) string {
	if !nonzero {
		return ""
	}
	switch base {
	case 16:
		if upper {
			return "0X"
		}
		return "0x"
	case 8:
		return "0"
	case 2:
		if upper {
			return "0B"
		}
		return "0b"
	}
	return ""
}

// formatG renders a float with the g/G conversion at the given precision,
// matching MRI: trailing zeros are stripped unless the # flag is set, and the
// exponent has the C two-digit-minimum form. Go's strconv 'g'/'G' already
// strips trailing zeros and uses the C exponent form, so the non-# path is a
// direct call; the # path re-adds the zeros C/Ruby keep.
func formatG(x float64, verb byte, prec int, hash bool) string {
	s := strconv.FormatFloat(x, verb, prec, 64)
	if !hash {
		return s
	}
	// Alternate form: keep trailing zeros and a decimal point to the full
	// precision of significant digits.
	return formatGAlt(x, verb, prec)
}

// formatGAlt produces the # (alternate) form of g/G: a decimal point is always
// present and trailing zeros are retained out to prec significant digits.
func formatGAlt(x float64, verb byte, prec int) string {
	// Decide e vs f exponent the way C does for %g, then format with that verb
	// keeping all digits.
	exp := 0
	if x != 0 {
		exp = int(math.Floor(math.Log10(math.Abs(x))))
	}
	useExp := exp < -4 || exp >= prec
	var s string
	if useExp {
		ev := byte('e')
		if verb == 'G' {
			ev = 'E'
		}
		s = strconv.FormatFloat(x, ev, prec-1, 64)
	} else {
		s = strconv.FormatFloat(x, 'f', prec-1-exp, 64)
	}
	// The alternate form always shows a decimal point, even when no fractional
	// digit remains (e.g. "%#g" of 123456.789 -> "123457.").
	return ensureDecimalPoint(s)
}

// ensureDecimalPoint inserts a '.' into a float rendering that lacks one,
// placing it before any exponent marker, as the alternate (#) form requires.
func ensureDecimalPoint(s string) string {
	if strings.ContainsRune(s, '.') {
		return s
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		return s[:i] + "." + s[i:]
	}
	return s + "."
}

// formatHexFloat renders a float with the a/A hexadecimal-float conversion,
// matching MRI/C: 0x1.<hex>p±d (lowercase) or 0X1.<HEX>P±D (uppercase), with an
// optional precision limiting the fractional hex digits.
func formatHexFloat(x float64, verb byte, hasPrec bool, prec int) string {
	p := -1
	if hasPrec {
		p = prec
	}
	v := byte('x')
	if verb == 'A' {
		v = 'X'
	}
	s := strconv.FormatFloat(x, v, p, 64)
	// Go pads the binary exponent to two digits (p+00); MRI/C use the minimum
	// digit count (p+0, p+7, p+10), so strip a single redundant leading zero.
	// A finite float always carries a 'p'/'P' exponent here (Inf/NaN are handled
	// before this point), so the index is always valid.
	i := strings.IndexAny(s, "pP")
	mant, pc, sign, exp := s[:i], s[i], s[i+1], s[i+2:]
	for len(exp) > 1 && exp[0] == '0' {
		exp = exp[1:]
	}
	return mant + string(pc) + string(sign) + exp
}

// truncRunes returns the first n runes of s (n>=0); used by %s/%p precision.
func truncRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// runeLen counts runes for width comparisons (Ruby measures width in
// characters, not bytes).
func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
