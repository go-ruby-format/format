package format

import (
	"fmt"
	"math"
	"math/big"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// corpus is the differential corpus: every (format, args) case is run through
// both this package and MRI (when ruby is on PATH) and the outputs — or the
// raised exception class+message — compared byte-for-byte. The same corpus
// drives the deterministic golden test (golden_test.go), which embeds the
// expected results so the no-ruby CI lanes still exercise every path.
var corpus = buildCorpus()

// caseT is one differential case: a format string and its Go arguments.
type caseT struct {
	format string
	args   []any
}

func buildCorpus() []caseT {
	bigPow := new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)
	negBigPow := new(big.Int).Neg(bigPow)
	big64 := new(big.Int).Lsh(big.NewInt(1), 64) // 2^64
	negBig64 := new(big.Int).Neg(big64)
	hash := map[string]any{"x": 42, "name": "Ada", "f": 3.5}
	inf := math.Inf(1)
	ninf := math.Inf(-1)
	nan := math.NaN()

	c := func(f string, a ...any) caseT { return caseT{f, a} }
	return []caseT{
		// Plain text and the %% literal.
		c("hello world"),
		c("100%% done"),
		c("%d%%", 50),
		c("a%%b%%c"),

		// Integer: d/i/u and flags.
		c("%d", 42), c("%i", 42), c("%u", 42),
		c("%d", -42), c("%5d", 42), c("%-5d|", 42), c("%05d", 42),
		c("%+d", 42), c("% d", 42), c("%+d", -5), c("% d", -5),
		c("%5d", -42), c("%05d", -42), c("%+05d", -3), c("%05.3d", -7),
		c("%5.3d", 7), c("%-5.3d|", 7), c("%+.3d", 7), c("% 5.3d", 7),
		c("%.0d", 0), c("%-05d", 3), c("%+ d", 5), c("% +d", 5), c("%0-5d|", 5),
		c("%#d", 42), c("%#10.5d", 42), c("%.3d", 0), c("%.3x", 0),

		// Hex/octal/binary, positive.
		c("%x", 255), c("%X", 255), c("%#x", 255), c("%#X", 255),
		c("%o", 8), c("%#o", 8), c("%b", 5), c("%#b", 5), c("%B", 5), c("%#B", 5),
		c("%+x", 255), c("% x", 255), c("%08x", 255), c("%#08x", 255),
		c("%+#x", 255), c("%010X", -1), c("%.1d", 42), c("%.1x", -1),
		c("%#.4x", 255), c("%#.3o", 8), c("%#.1o", 8), c("%x", 0), c("%#x", 0),
		c("%#o", 0), c("%#b", 0), c("%.0o", 0), c("%.0x", 0), c("%#.0o", 0),
		c("%#.0x", 0),

		// Negative two's-complement bases.
		c("%x", -1), c("%X", -255), c("%#x", -255), c("%o", -8), c("%b", -5),
		c("%x", -256), c("%x", -16), c("%o", -64), c("%b", -8), c("%b", -2),
		c("%10x", -1), c("%-10x|", -1), c("%010x", -1), c("%#10x", -255),
		c("%.5x", -1), c("%.5o", -1), c("%.5b", -1), c("%08x", -1),
		c("%5b", -5), c("%-8b|", -5), c("%+x", -1), c("%#x", -1), c("%#o", -8),
		c("%X", -255),

		// Bignum.
		c("%d", bigPow), c("%x", bigPow), c("%d", negBigPow), c("%+d", bigPow),
		c("%020d", big.NewInt(1000000000000000000)), c("%d", big64),
		c("%x", negBig64), c("%b", new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), 32))),

		// Float: f/e/E/g/G.
		c("%f", 3.14), c("%.2f", 3.14159), c("%10.2f", 3.14), c("%-10.2f|", 3.14),
		c("%e", 12345.678), c("%E", 12345.678), c("%g", 0.0001), c("%g", 1000000.0),
		c("%g", 100000.0), c("%G", 0.00001), c("%.3g", 3.14159), c("%g", 1.5),
		c("%g", 0.0), c("%g", 123456789.0), c("%.10g", 1.0/3.0), c("%#g", 1.5),
		c("%#.0f", 5.0), c("%#.0f", 2.0), c("%.0f", 2.5), c("%.0f", 3.5),
		c("%08.2f", 3.14), c("%+08.2f", 3.14), c("%5.2f", -3.14159),
		c("%+.0e", 0.0), c("%g", -0.0), c("%f", -0.0), c("%010.3f", -1.5),
		c("%-10.3f|", -1.5), c("%e", 0.0), c("%.0e", 9.99), c("%.3f", 0.0),
		c("% .3f", 1.5), c("%g", 1e-5), c("%g", 1e20), c("%g", 1e21),
		c("%g", 1e100), c("%g", 1e-100), c("%.0g", 123.0), c("%g", 100.0),
		c("%#g", 100.0), c("%#G", 0.0001), c("%G", 1234567.0), c("%.3g", 0.0001234),
		c("%#G", 1e20), c("%#.0e", 5.0), c("%#.0E", 5.0),
		c("%#.1g", 1e20), c("%#g", 123456.789),

		// Float special values.
		c("%f", inf), c("%f", ninf), c("%f", nan), c("%e", inf), c("%g", inf),
		c("%+f", inf), c("% f", inf), c("%10f", inf), c("%-10f|", inf),
		c("%010f", inf), c("%g", nan), c("%+e", ninf),

		// Hex float a/A.
		c("%a", 1.0), c("%A", 255.5), c("%a", 0.5), c("%a", -1.5), c("%a", 0.0),
		c("%a", 3.14), c("%a", 1024.0), c("%.2a", 3.14), c("%a", 2.0),
		c("%.0a", 3.14), c("%a", inf), c("%a", nan),

		// String: s and precision/width.
		c("%s", "hi"), c("%5s", "hi"), c("%-5s|", "hi"), c("%.3s", "hello"),
		c("%5.2s", "hello"), c("%.3s", "héllo"), c("%.10s", "hi"),
		c("%s", true), c("%s", false),
		c("%s", nil), c("%s", Symbol("sym")), c("%s", []any{1, 2}),
		c("%10s", "x"),

		// Inspect: p.
		c("%p", "hi"), c("%p", Symbol("sym")), c("%p", nil), c("%p", []any{1, 2}),
		c("%p", true), c("%p", false),
		c("%p", 1.5), c("%p", 42), c("%p", "a\nb"), c("%p", "a\tb\"c"),
		c("%p", 3.0), c("%p", inf), c("%10p", "hi"), c("%-10p|", "hi"),
		c("%.3p", "hello"), c("%p", "a#{x}"),

		// Char: c.
		c("%c", 65), c("%c", 0x3b1), c("%3c", 65),
		c("%c", 256), c("%c", 0x10FFFF), c("%c", "abc"), c("%c", ""),
		c("%c%c", 104, 105), c("%3.1c", 65), c("%-3c|", 65),

		// Named references.
		c("%<x>d", hash), c("%<x>05d", map[string]any{"x": 7}),
		c("%{x}", map[string]any{"x": "hi"}), c("val=%{x}!", map[string]any{"x": 99}),
		c("%<name>s says %<x>d", hash), c("%<f>.1f", hash),
		c("%{name}", hash), c("%<name>-10s|", hash),

		// Argument indexing and width/precision from args.
		c("%d %d", 1, 2), c("%2$s %1$s", "a", "b"), c("%*d", 5, 42),
		c("%-*d|", 5, 42), c("%.*f", 2, 3.14159), c("%*d", -5, 42),
		c("%.*f", -1, 3.14159), c("%*.*f", 8, 2, 3.14159), c("%2$04d|%1$+d", 5, 7),
		c("%2$d %2$d", 1, 2), c("%1$*2$d", 5, 3),

		// String coercion for numeric verbs.
		c("%d", "  42  "), c("%d", "0x1A"), c("%d", "0b101"), c("%d", "1_000"),
		c("%f", "1.5"), c("%f", "  2.5 "),

		// Float→int and int→float truncation/widening.
		c("%d", 3.9), c("%f", 42), c("%c", 1.9),
	}
}

// errCorpus holds cases that must raise; each records the expected MRI exception
// class and message so the deterministic test can assert them without ruby.
var errCorpus = []struct {
	format string
	args   []any
	class  string
	msg    string
}{
	{"%d", nil, "ArgumentError", "too few arguments"},
	{"%z", []any{1}, "ArgumentError", "malformed format string - %z"},
	{"%", []any{1}, "ArgumentError", "incomplete format specifier; use %% (double %) instead"},
	{"%5", []any{1}, "ArgumentError", "malformed format string - %*[0-9]"},
	{"%+", []any{1}, "ArgumentError", "malformed format string"},
	{"%.", []any{1}, "ArgumentError", "malformed format string - %*[0-9]"},
	{"%1$", []any{1}, "ArgumentError", "malformed format string"},
	{"%<x", []any{map[string]any{"x": 1}}, "ArgumentError", "malformed name - unmatched parenthesis"},
	{"%{x", []any{map[string]any{"x": 1}}, "ArgumentError", "malformed name - unmatched parenthesis"},
	{"%1$s %s", []any{"a", "b"}, "ArgumentError", "unnumbered(1) mixed with numbered"},
	{"%s %1$s", []any{"a", "b"}, "ArgumentError", "numbered(1) after unnumbered(1)"},
	{"%2$1$d", []any{1, 2}, "ArgumentError", "value given twice - 1$"},
	{"%<x>d", []any{map[string]any{"y": 1}}, "KeyError", "key<x> not found"},
	{"%{x}", []any{map[string]any{"y": 1}}, "KeyError", "key{x} not found"},
	{"%<x>d", []any{1}, "ArgumentError", "one hash required"},
	{"%{x}", []any{1}, "ArgumentError", "one hash required"},
	{"%d", []any{"notnum"}, "ArgumentError", `invalid value for Integer(): "notnum"`},
	{"%f", []any{"notnum"}, "ArgumentError", `invalid value for Float(): "notnum"`},
	{"%d", []any{nil}, "TypeError", "can't convert nil into Integer"},
	{"%f", []any{nil}, "TypeError", "can't convert nil into Float"},
	{"%d", []any{true}, "TypeError", "can't convert true into Integer"},
	{"%c", []any{-1}, "ArgumentError", "invalid character"},
	{"%c", []any{0x110000}, "ArgumentError", "invalid character"},
	{"%c", []any{[]any{1}}, "TypeError", "no implicit conversion of Array into Integer"},
	{"%*d", []any{nil, 5}, "TypeError", "no implicit conversion from nil to integer"},
	{"%c", []any{Symbol("z")}, "TypeError", "no implicit conversion of Symbol into Integer"},
	{"%s%s", []any{"a"}, "ArgumentError", "too few arguments"},
	{"%2$s", []any{"a"}, "ArgumentError", "too few arguments"},
	{"%*d", nil, "ArgumentError", "too few arguments"},
}

// rubyAvailable reports whether the ruby oracle can be invoked.
func rubyAvailable() bool {
	_, err := exec.LookPath("ruby")
	return err == nil
}

// rubyLiteral renders a Go argument as the Ruby source literal the oracle uses.
func rubyLiteral(a any) string {
	switch x := a.(type) {
	case nil:
		return "nil"
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case *big.Int:
		return x.String()
	case float64:
		switch {
		case math.IsInf(x, 1):
			return "(1.0/0.0)"
		case math.IsInf(x, -1):
			return "(-1.0/0.0)"
		case math.IsNaN(x):
			return "(0.0/0.0)"
		}
		s := strconv.FormatFloat(x, 'g', -1, 64)
		// Keep the value a Float in Ruby source: append .0 when it would
		// otherwise read as an integer literal.
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return s
	case string:
		return rubyStrLit(x)
	case Symbol:
		return ":" + string(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = rubyLiteral(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = ":" + k + " => " + rubyLiteral(x[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	panic(fmt.Sprintf("rubyLiteral: unsupported %T", a))
}

// rubyStrLit renders a Go string as a Ruby double-quoted literal.
func rubyStrLit(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '#':
			b.WriteString(`\#`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// rubyArgs renders the Go args as the comma-separated Ruby argument list passed
// to sprintf.
func rubyArgs(args []any) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = rubyLiteral(a)
	}
	return strings.Join(parts, ", ")
}

// mriResult runs `sprintf(fmt, *args)` under MRI, returning either ("", output)
// on success or (class+message, "") on a raised exception, distinguished by the
// leading tag the oracle script prints.
func mriResult(t *testing.T, format string, args []any) (errTag, out string) {
	t.Helper()
	script := `$stdout.binmode
fmt = $stdin.binmode.read.force_encoding("UTF-8")
begin
  print "OK\t" + sprintf(fmt, ` + rubyArgs(args) + `)
rescue => e
  print "ERR\t" + e.class.to_s + "\t" + e.message
end`
	cmd := exec.Command("ruby", "-e", script)
	cmd.Stdin = strings.NewReader(format)
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("ruby sprintf(%q, %s): %v", format, rubyArgs(args), err)
	}
	s := string(raw)
	switch {
	case strings.HasPrefix(s, "OK\t"):
		return "", s[3:]
	case strings.HasPrefix(s, "ERR\t"):
		rest := s[4:]
		i := strings.IndexByte(rest, '\t')
		return rest[:i] + ": " + rest[i+1:], ""
	}
	t.Fatalf("unexpected oracle output %q", s)
	return "", ""
}

// TestDifferentialAgainstMRI compares every success and error case against the
// MRI oracle. It self-skips when ruby is absent (qemu/Windows lanes), where the
// deterministic golden test still provides full coverage.
func TestDifferentialAgainstMRI(t *testing.T) {
	if !rubyAvailable() {
		t.Skip("ruby not on PATH; the golden test covers these cases")
	}
	for _, tc := range corpus {
		errTag, want := mriResult(t, tc.format, tc.args)
		got, gerr := Sprintf(tc.format, tc.args...)
		if errTag != "" {
			t.Errorf("case %q args=%s: MRI raised %q but ours did not (got %q, err %v)",
				tc.format, rubyArgs(tc.args), errTag, got, gerr)
			continue
		}
		if gerr != nil {
			t.Errorf("case %q args=%s: ours errored %v; MRI gave %q",
				tc.format, rubyArgs(tc.args), gerr, want)
			continue
		}
		if got != want {
			t.Errorf("MISMATCH %q args=%s\n ours=%q\n  mri=%q",
				tc.format, rubyArgs(tc.args), got, want)
		}
	}
	for _, tc := range errCorpus {
		errTag, _ := mriResult(t, tc.format, tc.args)
		want := tc.class + ": " + tc.msg
		if errTag != want {
			t.Errorf("ERR case %q args=%s: MRI gave %q want %q",
				tc.format, rubyArgs(tc.args), errTag, want)
		}
	}
}
