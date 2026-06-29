package format

import (
	"errors"
	"math"
	"math/big"
	"testing"
)

// hostValue is a minimal third-party Value implementation, standing in for an
// rbgo object, used to prove the Value interface is sufficient for a host to
// format its own values without going through the plain-Go adapters.
type hostValue struct {
	kind    Kind
	s, insp string
	cls     string
	i       *big.Int
	f       float64
	ierr    error
	iok     bool
	ferr    error
	fok     bool
}

func (h hostValue) Kind() Kind        { return h.kind }
func (h hostValue) ToS() string       { return h.s }
func (h hostValue) Inspect() string   { return h.insp }
func (h hostValue) ClassName() string { return h.cls }
func (h hostValue) Int() (*big.Int, error, bool) {
	return h.i, h.ierr, h.iok
}
func (h hostValue) Float() (float64, error, bool) { return h.f, h.ferr, h.fok }

func TestHostValueBinding(t *testing.T) {
	// A host Integer value formats through %d and %x.
	iv := hostValue{kind: KindInteger, s: "7", insp: "7", cls: "Integer", i: big.NewInt(7), iok: true}
	got, err := Format("%d/%x", []Value{iv, iv}, nil)
	if err != nil || got != "7/7" {
		t.Fatalf("host int: got %q err %v", got, err)
	}
	// A host Float value formats through %.1f.
	fv := hostValue{kind: KindFloat, cls: "Float", f: 2.5, fok: true}
	got, err = Format("%.1f", []Value{fv}, nil)
	if err != nil || got != "2.5" {
		t.Fatalf("host float: got %q err %v", got, err)
	}
	// A host String value formats through %s and %p (inspect).
	sv := hostValue{kind: KindString, s: "hi", insp: `"hi"`, cls: "String"}
	got, _ = Format("%s %p", []Value{sv, sv}, nil)
	if got != `hi "hi"` {
		t.Fatalf("host string: %q", got)
	}
	// A Value passed directly to Sprintf is used as-is (toValue passthrough).
	got, _ = Sprintf("%d", iv)
	if got != "7" {
		t.Fatalf("Sprintf with Value arg: %q", got)
	}
}

func TestHostValueErrors(t *testing.T) {
	// Int() reporting a parse error becomes an ArgumentError.
	pv := hostValue{kind: KindString, cls: "String", iok: true, ierr: errors.New("invalid value for Integer(): \"x\"")}
	_, err := Format("%d", []Value{pv}, nil)
	if err == nil || err.(*Error).Class != "ArgumentError" {
		t.Fatalf("want ArgumentError, got %v", err)
	}
	// Int() reporting not-ok becomes a TypeError.
	nv := hostValue{kind: KindArray, cls: "Array"}
	_, err = Format("%d", []Value{nv}, nil)
	if err == nil || err.(*Error).Class != "TypeError" {
		t.Fatalf("want TypeError, got %v", err)
	}
	// Float() parse error and not-ok.
	fpv := hostValue{kind: KindString, cls: "String", fok: true, ferr: errors.New("invalid value for Float(): \"x\"")}
	_, err = Format("%f", []Value{fpv}, nil)
	if err == nil || err.(*Error).Class != "ArgumentError" {
		t.Fatalf("want ArgumentError, got %v", err)
	}
	_, err = Format("%f", []Value{nv}, nil)
	if err == nil || err.(*Error).Class != "TypeError" {
		t.Fatalf("want TypeError, got %v", err)
	}
	// %c with a host non-string, non-integer is a TypeError.
	_, err = Format("%c", []Value{nv}, nil)
	if err == nil || err.(*Error).Class != "TypeError" {
		t.Fatalf("%%c host: want TypeError, got %v", err)
	}
	// A '*' width drawn from a host non-integer is a TypeError.
	_, err = Format("%*d", []Value{nv, iVal(3)}, nil)
	if err == nil || err.(*Error).Class != "TypeError" {
		t.Fatalf("star host: want TypeError, got %v", err)
	}
}

func iVal(n int) Value { return toValue(n) }

func TestErrorString(t *testing.T) {
	e := &Error{Class: "ArgumentError", Message: "too few arguments"}
	if e.Error() != "ArgumentError: too few arguments" {
		t.Fatalf("Error(): %q", e.Error())
	}
}

func TestToValueBranches(t *testing.T) {
	cases := []struct {
		arg  any
		fmt  string
		want string
	}{
		{int64(9), "%d", "9"},
		{big.NewInt(11), "%d", "11"},
		{true, "%s", "true"},
		{false, "%s", "false"},
		{Symbol("ok"), "%p", ":ok"},
		{[]any{1, "a", nil}, "%p", `[1, "a", nil]`},
		{nil, "%s", ""},
		{nil, "%p", "nil"},
		{3.5, "%s", "3.5"},
		{[]any{}, "%s", "[]"},
	}
	for _, c := range cases {
		got, err := Sprintf(c.fmt, c.arg)
		if err != nil || got != c.want {
			t.Errorf("Sprintf(%q,%#v) = %q,%v want %q", c.fmt, c.arg, got, err, c.want)
		}
	}
}

// stringer exercises the toValue default/toStringFallback path for an arbitrary
// type implementing String().
type stringer struct{ v string }

func (s stringer) String() string { return s.v }

func TestToValueFallback(t *testing.T) {
	got, _ := Sprintf("%s", stringer{"custom"})
	if got != "custom" {
		t.Fatalf("stringer fallback: %q", got)
	}
	// A type without String() yields the empty fallback.
	type opaque struct{}
	got, _ = Sprintf("%s", opaque{})
	if got != "" {
		t.Fatalf("opaque fallback: %q", got)
	}
}

func TestNamedArgsAPI(t *testing.T) {
	na := NewNamedArgs(map[string]Value{
		"a": toValue(1),
		"b": toValue("two"),
	})
	if k := na.Kind(); k != KindHash {
		t.Fatalf("Kind = %v", k)
	}
	if na.ClassName() != "Hash" {
		t.Fatalf("ClassName = %q", na.ClassName())
	}
	if na.Inspect() != `{a: 1, b: "two"}` {
		t.Fatalf("Inspect = %q", na.Inspect())
	}
	if na.ToS() != na.Inspect() {
		t.Fatalf("ToS != Inspect")
	}
	if _, _, ok := na.Int(); ok {
		t.Fatal("Hash.Int ok")
	}
	if _, _, ok := na.Float(); ok {
		t.Fatal("Hash.Float ok")
	}
	// Passing a *NamedArgs to Sprintf supplies the named hash.
	got, err := Sprintf("%<a>d/%{b}", na)
	if err != nil || got != "1/two" {
		t.Fatalf("named via *NamedArgs: %q %v", got, err)
	}
	// Passing a map[string]Value also works.
	got, _ = Sprintf("%<a>d", map[string]Value{"a": toValue(5)})
	if got != "5" {
		t.Fatalf("map[string]Value: %q", got)
	}
}

func TestRubyFloatToS(t *testing.T) {
	cases := map[float64]string{
		3.0:          "3.0",
		0.1:          "0.1",
		1e20:         "1.0e+20",
		1e-5:         "1.0e-05",
		1e17:         "1.0e+17",
		math.Inf(1):  "Infinity",
		math.Inf(-1): "-Infinity",
		math.NaN():   "NaN",
	}
	for f, want := range cases {
		got := rubyFloatToS(f)
		if math.IsNaN(f) {
			got = rubyFloatToS(math.NaN())
		}
		if got != want {
			t.Errorf("rubyFloatToS(%v) = %q want %q", f, got, want)
		}
	}
	// %s of these floats goes through rubyFloatToS too.
	got, _ := Sprintf("%s", 1e20)
	if got != "1.0e+20" {
		t.Fatalf("%%s float: %q", got)
	}
}

func TestRubyInspectEscapes(t *testing.T) {
	got, _ := Sprintf("%p", "a\nb\tc\r\x1b\a\b\f\v\\\"'#x")
	want := `"a\nb\tc\r\e\a\b\f\v\\\"'#x"`
	if got != want {
		t.Fatalf("inspect escapes:\n got=%q\nwant=%q", got, want)
	}
	// Control bytes render as \u00XX; '#' is escaped only before {, @, $.
	got, _ = Sprintf("%p", "\x00\x1f\x7f a#b#{c#@d#$e#")
	want = "\"\\u0000\\u001F\\u007F a#b\\#{c\\#@d\\#$e#\""
	if got != want {
		t.Fatalf("inspect controls:\n got=%q\nwant=%q", got, want)
	}
}

func TestStringCoercionRadix(t *testing.T) {
	got, _ := Sprintf("%d", "0o17")
	if got != "15" {
		t.Fatalf("octal string: %q", got)
	}
	got, _ = Sprintf("%x", "255")
	if got != "ff" {
		t.Fatalf("hex of decimal string: %q", got)
	}
	// A huge decimal string parses as a Bignum, not int64.
	got, _ = Sprintf("%d", "123456789012345678901234567890")
	if got != "123456789012345678901234567890" {
		t.Fatalf("bignum string: %q", got)
	}
}

func TestHashGFormatAlt(t *testing.T) {
	// Exercise the # alternate g/G exponent and fixed branches.
	for _, c := range []struct{ f, w string }{
		{"%#g", "1.50000"},
		{"%#g", "100.000"},
		{"%#G", "0.000100000"},
		{"%#.3g", "1.00"},
		{"%#g", "1.00000e+20"},
	} {
		_ = c
	}
	got, _ := Sprintf("%#g", 1e20)
	if got != "1.00000e+20" {
		t.Fatalf("#g exp: %q", got)
	}
	got, _ = Sprintf("%#.3g", 1.0)
	if got != "1.00" {
		t.Fatalf("#.3g: %q", got)
	}
	got, _ = Sprintf("%#g", 0.0)
	if got != "0.00000" {
		t.Fatalf("#g zero: %q", got)
	}
}
