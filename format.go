// Package format implements Ruby's format-string engine — the computation
// behind Kernel#sprintf, Kernel#format, and String#% — as a standalone,
// pure-Go (no cgo) library compatible with MRI (CRuby) 4.0.
//
// It is the formatting backend for go-embedded-ruby (rbgo) but depends on
// nothing from the Ruby runtime: a host binds it by passing its own values
// through the small Value interface (see value.go), or a plain Go caller passes
// int/int64/*big.Int/float64/string/bool/nil/[]any and a NamedArgs/map for
// named references.
//
// Every conversion, flag, width/precision form, named/numbered reference, and
// the MRI ArgumentError/KeyError/TypeError messages are matched against CRuby
// byte-for-byte (see the differential test suite).
package format

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Sprintf formats per Ruby's Kernel#sprintf / Kernel#format / String#%. The
// arguments are the Ruby values to format, given either as plain Go values
// (int, int64, *big.Int, float64, string, bool, nil, []any, Symbol) or as
// Values; a *NamedArgs or map[string]any/map[string]Value supplies the hash for
// %<name>s and %{name} references. It returns the formatted string or an *Error
// carrying the Ruby exception class and message MRI would raise.
func Sprintf(formatStr string, args ...any) (string, error) {
	vals, named := splitArgs(args)
	return Format(formatStr, vals, named)
}

// Format is the explicit-arg form of Sprintf: positional Values plus the named
// hash for %<name>/%{name} references (nil when there are none). It is the
// primary entry point for hosts (rbgo) that already hold Values.
func Format(formatStr string, args []Value, named *NamedArgs) (out string, err error) {
	f := &formatter{format: formatStr, args: args, named: named}
	defer func() {
		if r := recover(); r != nil {
			if fe, ok := r.(*Error); ok {
				out, err = "", fe
				return
			}
			panic(r)
		}
	}()
	return f.run(), nil
}

// splitArgs adapts Sprintf's variadic any arguments into positional Values and
// an optional named hash. A trailing map[string]any / map[string]Value /
// *NamedArgs is treated as the named hash (as MRI treats a sole Hash argument);
// it is also retained as a positional Value so a format using neither named nor
// positional refs still sees a single hash argument.
func splitArgs(args []any) ([]Value, *NamedArgs) {
	vals := make([]Value, len(args))
	var named *NamedArgs
	for i, a := range args {
		switch m := a.(type) {
		case *NamedArgs:
			named = m
			vals[i] = m
		case map[string]Value:
			named = NewNamedArgs(m)
			vals[i] = named
		case map[string]any:
			conv := make(map[string]Value, len(m))
			for k, v := range m {
				conv[k] = toValue(v)
			}
			named = NewNamedArgs(conv)
			vals[i] = named
		default:
			vals[i] = toValue(a)
		}
	}
	return vals, named
}

// formatter holds the mutable state of a single Format call: the cursor into
// the format string, the positional argument list with its auto-advancing
// index, and the bookkeeping that detects mixing numbered and unnumbered
// references (an MRI error).
type formatter struct {
	format string
	pos    int
	args   []Value
	named  *NamedArgs
	b      strings.Builder

	argIdx     int  // next auto (unnumbered) positional argument
	usedNumber bool // a %n$ reference has been used
	usedAuto   bool // an unnumbered reference has been used
}

// run walks the format string, copying literal bytes and dispatching each
// conversion, and returns the assembled output.
func (f *formatter) run() string {
	// Pre-size the output for the common case where the result is close to the
	// format string's length, avoiding the Builder's early re-growths.
	f.b.Grow(len(f.format) + 16)
	for f.pos < len(f.format) {
		c := f.format[f.pos]
		if c != '%' {
			f.b.WriteByte(c)
			f.pos++
			continue
		}
		f.directive()
	}
	return f.b.String()
}

// spec holds the parsed pieces of one conversion directive.
type spec struct {
	minusFlag bool
	plusFlag  bool
	spaceFlag bool
	zeroFlag  bool
	hashFlag  bool

	width    int
	hasWidth bool
	prec     int
	hasPrec  bool

	argNum    int // 1-based explicit %n$ reference, 0 when absent
	hasArgNum bool

	namedRef Value // the %<name> value, when hasNamed
	hasNamed bool

	verb byte
}

// directive parses and renders the conversion that begins at f.pos (the '%'),
// advancing f.pos past it. %{name} is handled specially (no conversion letter).
func (f *formatter) directive() {
	start := f.pos
	f.pos++ // consume '%'
	if f.pos >= len(f.format) {
		panic(argumentError("incomplete format specifier; use %% (double %) instead"))
	}
	// %{name}: insert the named value's to_s, with no conversion.
	if f.format[f.pos] == '{' {
		f.bracedReference()
		return
	}
	var s spec
	sawNumeric := f.parseFlagsWidthPrec(&s)
	if s.verb == '%' {
		f.b.WriteByte('%')
		return
	}
	if s.verb == 0 {
		// The directive ran off the end of the string with no conversion
		// letter. MRI distinguishes a bare flag run ("malformed format string")
		// from one that saw a width/precision/'*'/digit token, which reports the
		// "%*[0-9]" form.
		if sawNumeric {
			panic(argumentError("malformed format string - %*[0-9]"))
		}
		panic(argumentError("malformed format string"))
	}
	_ = start
	f.render(&s)
}

// bracedReference handles %{name}: it inserts the to_s of the named value with
// no further formatting, raising KeyError when the key is absent.
func (f *formatter) bracedReference() {
	end := strings.IndexByte(f.format[f.pos:], '}')
	if end < 0 {
		panic(argumentError("malformed name - unmatched parenthesis"))
	}
	name := f.format[f.pos+1 : f.pos+end]
	v := f.namedValue(name, '{')
	f.b.WriteString(v.ToS())
	f.pos += end + 1
}

// parseFlagsWidthPrec consumes the [n$][flags][width][.prec]verb body of a
// directive starting just after '%', filling s and leaving f.pos just past the
// verb (or, for %%, past the second '%'). It also handles %<name> references,
// which carry an explicit named value rather than a positional one.
func (f *formatter) parseFlagsWidthPrec(s *spec) (sawNumeric bool) {
	// A leading number followed by '$' is an absolute argument reference; the
	// same digits without '$' are the width, so we must look ahead.
	f.maybeArgNum(s)

	for f.pos < len(f.format) {
		switch f.format[f.pos] {
		case '-':
			s.minusFlag = true
		case '+':
			s.plusFlag = true
		case ' ':
			s.spaceFlag = true
		case '0':
			s.zeroFlag = true
		case '#':
			s.hashFlag = true
		case '<':
			// %<name>: bind the conversion to the named value. Flags may appear
			// on either side of the reference, so this lives in the flag loop.
			end := strings.IndexByte(f.format[f.pos:], '>')
			if end < 0 {
				panic(argumentError("malformed name - unmatched parenthesis"))
			}
			name := f.format[f.pos+1 : f.pos+end]
			s.namedRef = f.namedValue(name, '<')
			s.hasNamed = true
			f.pos += end + 1
			f.maybeArgNum(s)
			continue
		default:
			goto flagsDone
		}
		f.pos++
		// A %n$ reference may also appear after flags in MRI; re-scan.
		f.maybeArgNum(s)
	}
flagsDone:
	// A digit run ending in '$' here is a (second) argument reference, not a
	// width — MRI rejects supplying the index twice.
	f.maybeArgNum(s)
	// Width: '*' (from args, optionally with its own n$ index) or digits.
	if f.pos < len(f.format) && f.format[f.pos] == '*' {
		f.pos++
		sawNumeric = true
		w := f.starInt(s)
		if w < 0 {
			s.minusFlag = true
			w = -w
		}
		s.width, s.hasWidth = w, true
	} else if w, ok := f.parseNumber(); ok {
		sawNumeric = true
		s.width, s.hasWidth = w, true
	}

	// Precision.
	if f.pos < len(f.format) && f.format[f.pos] == '.' {
		f.pos++
		sawNumeric = true
		if f.pos < len(f.format) && f.format[f.pos] == '*' {
			f.pos++
			p := f.starInt(s)
			if p >= 0 {
				s.prec, s.hasPrec = p, true
			}
			// A negative '.*' precision is treated as if omitted (MRI).
		} else if p, ok := f.parseNumber(); ok {
			s.prec, s.hasPrec = p, true
		} else {
			s.prec, s.hasPrec = 0, true // ".": precision zero
		}
	}

	if f.pos < len(f.format) {
		s.verb = f.format[f.pos]
		f.pos++
	}
	return sawNumeric
}

// maybeArgNum recognizes a leading 1-based argument reference of the form
// "<digits>$" at f.pos, recording it in s and advancing past the '$'. It is a
// no-op when the next token is not such a reference. Re-using a reference (two
// $-numbers) or supplying a non-positive index is an MRI error.
func (f *formatter) maybeArgNum(s *spec) {
	j := f.pos
	for j < len(f.format) && f.format[j] >= '0' && f.format[j] <= '9' {
		j++
	}
	if j == f.pos || j >= len(f.format) || f.format[j] != '$' {
		return
	}
	if s.hasArgNum {
		n := f.format[f.pos:j]
		panic(argumentError("value given twice - " + n + "$"))
	}
	n, _ := strconv.Atoi(f.format[f.pos:j])
	s.argNum, s.hasArgNum = n, true
	f.pos = j + 1
}

// parseNumber consumes a run of decimal digits at f.pos, returning its value
// and true, or 0/false when no digit is present.
func (f *formatter) parseNumber() (int, bool) {
	j := f.pos
	for j < len(f.format) && f.format[j] >= '0' && f.format[j] <= '9' {
		j++
	}
	if j == f.pos {
		return 0, false
	}
	n, _ := strconv.Atoi(f.format[f.pos:j])
	f.pos = j
	return n, true
}

// starInt fetches the integer for a '*' width or precision. A "*n$" form takes
// the value from the n-th positional argument; a bare "*" takes the next
// positional argument. MRI's TypeError for a non-integer here reads "no
// implicit conversion from X to integer" (distinct from the value path's
// "of X into Integer").
func (f *formatter) starInt(s *spec) int {
	var v Value
	if n, j, ok := f.starArgNum(); ok {
		f.pos = j
		if f.usedAuto {
			panic(argumentError("numbered(" + strconv.Itoa(n) + ") after unnumbered(" + strconv.Itoa(f.argIdx) + ")"))
		}
		f.usedNumber = true
		if n < 1 || n > len(f.args) {
			panic(argumentError("too few arguments"))
		}
		v = f.args[n-1]
	} else {
		v = f.nextPositional()
	}
	z, _, intOK := v.Int()
	if !intOK {
		panic(typeError("no implicit conversion from " + v.ClassName() + " to integer"))
	}
	return int(z.Int64())
}

// starArgNum recognizes a "n$" index immediately following a '*' (the width/
// precision argument reference), returning the 1-based index, the position just
// past the '$', and whether such a reference is present.
func (f *formatter) starArgNum() (n, next int, ok bool) {
	j := f.pos
	for j < len(f.format) && f.format[j] >= '0' && f.format[j] <= '9' {
		j++
	}
	if j == f.pos || j >= len(f.format) || f.format[j] != '$' {
		return 0, 0, false
	}
	n, _ = strconv.Atoi(f.format[f.pos:j])
	return n, j + 1, true
}

// nextPositional consumes the next auto-advancing positional argument (used for
// a bare '*'), enforcing the no-mixing rule against numbered references.
func (f *formatter) nextPositional() Value {
	if f.usedNumber {
		panic(argumentError("unnumbered(" + strconv.Itoa(f.argIdx+1) + ") mixed with numbered"))
	}
	f.usedAuto = true
	if f.argIdx >= len(f.args) {
		panic(argumentError("too few arguments"))
	}
	v := f.args[f.argIdx]
	f.argIdx++
	return v
}

// namedValue resolves a %<name>/%{name} reference against the hash argument,
// raising "one hash required" when there is no hash and KeyError when the key is
// absent. open is '<' or '{' to choose MRI's KeyError bracket style.
func (f *formatter) namedValue(name string, open byte) Value {
	if f.named == nil {
		panic(argumentError("one hash required"))
	}
	v, ok := f.named.get(name)
	if !ok {
		if open == '{' {
			panic(keyError("key{" + name + "} not found"))
		}
		panic(keyError("key<" + name + "> not found"))
	}
	return v
}

// nextArg returns the argument a directive consumes: the %<name> value when the
// directive carried one, the %n$ indexed argument, or the next auto-advancing
// positional argument. It enforces MRI's prohibition on mixing numbered and
// unnumbered references.
func (f *formatter) nextArg(s *spec) Value {
	if s.hasNamed {
		return s.namedRef
	}
	if s.hasArgNum {
		if f.usedAuto {
			panic(argumentError("numbered(" + strconv.Itoa(s.argNum) + ") after unnumbered(" + strconv.Itoa(f.argIdx) + ")"))
		}
		f.usedNumber = true
		if s.argNum < 1 || s.argNum > len(f.args) {
			panic(argumentError("too few arguments"))
		}
		return f.args[s.argNum-1]
	}
	if f.usedNumber {
		panic(argumentError("unnumbered(" + strconv.Itoa(f.argIdx+1) + ") mixed with numbered"))
	}
	f.usedAuto = true
	if f.argIdx >= len(f.args) {
		panic(argumentError("too few arguments"))
	}
	v := f.args[f.argIdx]
	f.argIdx++
	return v
}

// render dispatches a fully parsed directive to the conversion for its verb.
func (f *formatter) render(s *spec) {
	switch s.verb {
	case 'd', 'i', 'u':
		f.renderInt(s, 10, false)
	case 'x':
		f.renderInt(s, 16, false)
	case 'X':
		f.renderInt(s, 16, true)
	case 'o':
		f.renderInt(s, 8, false)
	case 'b':
		f.renderInt(s, 2, false)
	case 'B':
		f.renderInt(s, 2, true)
	case 'f', 'e', 'E', 'g', 'G', 'a', 'A':
		f.renderFloat(s)
	case 's':
		f.renderStr(s, f.nextArg(s).ToS())
	case 'p':
		f.renderStr(s, f.nextArg(s).Inspect())
	case 'c':
		f.renderChar(s)
	default:
		panic(argumentError("malformed format string - %" + string(s.verb)))
	}
}

// intFast is an optional Value fast-path: a Value that reports itself as an
// int64 without allocating a *big.Int. renderInt uses it to keep the common
// small-integer conversion out of math/big; a Value that does not implement it,
// or whose magnitude exceeds int64 (a Bignum), falls back to the Int() path.
type intFast interface {
	// Int64Fast returns the value as an int64 with ok=true only when the value
	// is a genuine integer whose magnitude fits in int64 and whose formatting is
	// byte-identical to the *big.Int path. Non-integers, Bignums, Floats, and
	// String operands report ok=false so the caller uses the precise path.
	Int64Fast() (n int64, ok bool)
}

// intArg coerces the directive's argument to an arbitrary-precision integer,
// raising the MRI TypeError/ArgumentError on a non-integer / unparsable string.
func coerceInt(v Value) *big.Int {
	z, perr, ok := v.Int()
	if !ok {
		panic(typeError("can't convert " + v.ClassName() + " into Integer"))
	}
	if perr != nil {
		panic(argumentError(perr.Error()))
	}
	return z
}

// renderInt formats an integer in the given base. Negative values use Ruby's
// two's-complement "dot" notation (..f / ..7 / ..1) unless the + or space flag
// forces a signed rendering. upper selects uppercase digits (X/B). Values that
// fit in int64 take an allocation-free path; Bignums, Floats, and String
// operands fall back to the arbitrary-precision path.
func (f *formatter) renderInt(s *spec, base int, upper bool) {
	v := f.nextArg(s)
	if iv, ok := v.(intFast); ok {
		if n, isInt := iv.Int64Fast(); isInt {
			f.renderIntFast(s, base, upper, n)
			return
		}
	}
	f.renderIntBig(s, base, upper, coerceInt(v))
}

// renderIntFast formats an int64 without touching math/big. It reproduces
// renderIntBig's output byte-for-byte, deferring the one case that genuinely
// needs arbitrary precision — a negative value in an unsigned base, whose Ruby
// two's-complement "dot" notation is computed with big arithmetic.
func (f *formatter) renderIntFast(s *spec, base int, upper bool, n int64) {
	neg := n < 0
	signed := base == 10 || s.plusFlag || s.spaceFlag
	if neg && !signed {
		f.renderIntBig(s, base, upper, big.NewInt(n))
		return
	}

	// uint64(-n) yields the correct magnitude even for math.MinInt64, where -n
	// wraps back to MinInt64 and its uint64 reinterpretation is |MinInt64|.
	mag := uint64(n)
	if neg {
		mag = uint64(-n)
	}
	digits := strconv.FormatUint(mag, base)
	var prefix, sign string
	switch {
	case neg:
		sign = "-"
	case s.plusFlag:
		sign = "+"
	case s.spaceFlag:
		sign = " "
	}
	if s.hashFlag {
		prefix = altPrefix(base, upper, mag != 0)
	}
	f.emitInt(s, base, upper, sign, prefix, digits, false, n == 0)
}

// renderIntBig is the arbitrary-precision integer path, used for Bignums, Floats
// truncated to integers, String operands, and the negative-in-unsigned-base
// two's-complement case.
func (f *formatter) renderIntBig(s *spec, base int, upper bool, z *big.Int) {
	neg := z.Sign() < 0
	signed := base == 10 || s.plusFlag || s.spaceFlag

	var digits, prefix, sign string
	dotted := false
	if signed {
		mag := new(big.Int).Abs(z)
		digits = mag.Text(base)
		switch {
		case neg:
			sign = "-"
		case s.plusFlag:
			sign = "+"
		case s.spaceFlag:
			sign = " "
		}
		if s.hashFlag {
			prefix = altPrefix(base, upper, mag.Sign() != 0)
		}
	} else if neg {
		dotted = true
		digits = twosComplementDigits(z, base) // includes the ".." lead
		if s.hashFlag && base != 8 {
			// The two's-complement ".." run already encodes octal's leading 0,
			// so the alternate-form prefix is added only for 0x/0b.
			prefix = altPrefix(base, upper, true)
		}
	} else {
		digits = z.Text(base)
		if s.hashFlag {
			prefix = altPrefix(base, upper, z.Sign() != 0)
		}
	}
	f.emitInt(s, base, upper, sign, prefix, digits, dotted, z.Sign() == 0)
}

// emitInt applies the precision, uppercasing, and width padding shared by the
// int64 and *big.Int paths, then writes the field. isZero marks a zero magnitude
// (for precision's zero special-cases) and dotted a two's-complement rendering.
func (f *formatter) emitInt(s *spec, base int, upper bool, sign, prefix, digits string, dotted, isZero bool) {
	// Precision sets the minimum digit count (excluding sign/prefix), with
	// MRI's zero-value special cases.
	if s.hasPrec {
		digits, prefix = applyIntPrecision(digits, prefix, s, base, isZero, dotted)
	}
	if upper {
		digits = strings.ToUpper(digits)
		prefix = strings.ToUpper(prefix)
	}

	f.b.WriteString(padInt(s, sign, prefix, digits, base, upper, dotted))
}

// applyIntPrecision returns the digit and prefix strings after applying a
// precision (minimum digit count) to an integer's magnitude. For a two's-
// complement (dotted) rendering the count governs the repeated leading digit
// after the "..".
func applyIntPrecision(digits, prefix string, s *spec, base int, isZero, dotted bool) (string, string) {
	if isZero {
		if s.prec == 0 {
			if base == 8 && s.hashFlag {
				return "0", ""
			}
			return "", ""
		}
		d := strings.Repeat("0", s.prec)
		if base == 8 && s.hashFlag {
			return d, ""
		}
		return d, prefix
	}
	if dotted {
		return twosComplementPrecision(digits, base, s.prec), prefix
	}
	// Octal alternate form folds its leading "0" into the precision: the digit
	// run must begin with a zero, and the separate prefix is dropped.
	if base == 8 && s.hashFlag {
		prefix = ""
		if len(digits) < s.prec {
			digits = strings.Repeat("0", s.prec-len(digits)) + digits
		}
		if !strings.HasPrefix(digits, "0") {
			digits = "0" + digits
		}
		return digits, prefix
	}
	if len(digits) < s.prec {
		return strings.Repeat("0", s.prec-len(digits)) + digits, prefix
	}
	return digits, prefix
}

// padInt applies width, zero-, and left-justification to a formatted integer,
// returning the field. dotted marks a two's-complement rendering whose zero-
// fill repeats the leading digit (after the "..") rather than '0'.
func padInt(s *spec, sign, prefix, digits string, base int, upper, dotted bool) string {
	body := prefix + digits
	full := sign + body
	if !s.hasWidth || len(full) >= s.width {
		return full
	}
	pad := s.width - len(full)
	switch {
	case s.minusFlag:
		return full + strings.Repeat(" ", pad)
	case s.zeroFlag && !s.hasPrec:
		if dotted {
			// Repeat the leading digit just after the "..".
			fill := leadDigit(base, upper)
			i := strings.Index(digits, "..")
			head := digits[:i+2]
			tail := digits[i+2:]
			return sign + prefix + head + strings.Repeat(fill, pad) + tail
		}
		return sign + prefix + strings.Repeat("0", pad) + digits
	default:
		return strings.Repeat(" ", pad) + full
	}
}

// renderFloat formats a float conversion (f/e/E/g/G/a/A) via Go's strconv,
// with Ruby's Inf/NaN spellings and the +/space/0/# flag handling.
func (f *formatter) renderFloat(s *spec) {
	v := f.nextArg(s)
	x, perr, ok := v.Float()
	if !ok {
		panic(typeError("can't convert " + v.ClassName() + " into Float"))
	}
	if perr != nil {
		panic(argumentError(perr.Error()))
	}
	if math.IsInf(x, 0) || math.IsNaN(x) {
		f.renderSpecialFloat(s, x)
		return
	}

	verb := s.verb
	prec := -1
	if s.hasPrec {
		prec = s.prec
	} else {
		prec = 6
	}
	var body string
	switch verb {
	case 'f', 'e', 'E':
		body = strconv.FormatFloat(math.Abs(x), verb, prec, 64)
	case 'g', 'G':
		gp := prec
		if !s.hasPrec {
			gp = 6
		}
		if gp == 0 {
			gp = 1
		}
		body = formatG(math.Abs(x), verb, gp, s.hashFlag)
	case 'a', 'A':
		body = formatHexFloat(math.Abs(x), verb, s.hasPrec, prec)
	}
	if s.hashFlag && (verb == 'f' || verb == 'e' || verb == 'E') && !strings.ContainsAny(body, ".") {
		// Alternate form forces a decimal point.
		if i := strings.IndexAny(body, "eE"); i >= 0 {
			body = body[:i] + "." + body[i:]
		} else {
			body += "."
		}
	}

	sign := ""
	switch {
	case math.Signbit(x):
		sign = "-"
	case s.plusFlag:
		sign = "+"
	case s.spaceFlag:
		sign = " "
	}
	f.padFloat(s, sign, body)
}

// renderSpecialFloat renders Inf/-Inf/NaN as MRI does ("Inf"/"-Inf"/"NaN"),
// honoring width, the +/space sign flags, and left-justification (but never
// zero-padding).
func (f *formatter) renderSpecialFloat(s *spec, x float64) {
	var sign, word string
	switch {
	case math.IsNaN(x):
		word = "NaN"
	case math.IsInf(x, -1):
		sign, word = "-", "Inf"
	default:
		word = "Inf"
		if s.plusFlag {
			sign = "+"
		} else if s.spaceFlag {
			sign = " "
		}
	}
	full := sign + word
	if s.hasWidth && len(full) < s.width {
		pad := strings.Repeat(" ", s.width-len(full))
		if s.minusFlag {
			full += pad
		} else {
			full = pad + full
		}
	}
	f.b.WriteString(full)
}

// padFloat applies width, zero-, and left-justification to a formatted float.
func (f *formatter) padFloat(s *spec, sign, body string) {
	full := sign + body
	if !s.hasWidth || len(full) >= s.width {
		f.b.WriteString(full)
		return
	}
	pad := s.width - len(full)
	switch {
	case s.minusFlag:
		f.b.WriteString(full + strings.Repeat(" ", pad))
	case s.zeroFlag:
		f.b.WriteString(sign + strings.Repeat("0", pad) + body)
	default:
		f.b.WriteString(strings.Repeat(" ", pad) + full)
	}
}

// renderStr formats %s/%p: width, left-justification, and precision (which
// truncates the string to that many characters).
func (f *formatter) renderStr(s *spec, str string) {
	if s.hasPrec {
		str = truncRunes(str, s.prec)
	}
	if s.hasWidth && runeLen(str) < s.width {
		pad := strings.Repeat(" ", s.width-runeLen(str))
		if s.minusFlag {
			str += pad
		} else {
			str = pad + str
		}
	}
	f.b.WriteString(str)
}

// renderChar formats %c: an integer code point or a one-character string,
// padded by width like %s. An out-of-range code point is an ArgumentError.
func (f *formatter) renderChar(s *spec) {
	v := f.nextArg(s)
	var str string
	switch v.Kind() {
	case KindString:
		for _, r := range v.ToS() {
			str = string(r)
			break
		}
	default:
		z, _, ok := v.Int()
		if !ok {
			panic(typeError("no implicit conversion of " + v.ClassName() + " into Integer"))
		}
		n := z.Int64()
		if !z.IsInt64() || n < 0 || n > 0x10FFFF {
			panic(argumentError("invalid character"))
		}
		str = string(rune(n))
	}
	if s.hasWidth && runeLen(str) < s.width {
		pad := strings.Repeat(" ", s.width-runeLen(str))
		if s.minusFlag {
			str += pad
		} else {
			str = pad + str
		}
	}
	f.b.WriteString(str)
}
