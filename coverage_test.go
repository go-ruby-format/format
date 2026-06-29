package format

import (
	"math/big"
	"testing"
)

// TestStarArgErrors covers the argument-mixing and out-of-range error paths of
// '*' width/precision references, which the differential corpus does not reach.
func TestStarArgErrors(t *testing.T) {
	cases := []struct {
		format string
		args   []any
		want   string
	}{
		// A numbered '*' reference after an auto positional is rejected.
		{"%d %*2$d", []any{1, 5, 9}, "ArgumentError: numbered(2) after unnumbered(1)"},
		// An auto '*' reference after a numbered one is rejected.
		{"%2$d %*d", []any{1, 2}, "ArgumentError: unnumbered(1) mixed with numbered"},
		// A numbered '*' index out of range is "too few arguments".
		{"%*9$d", []any{5}, "ArgumentError: too few arguments"},
		// A bare '*' with no value left is "too few arguments".
		{"%*d", []any{5}, "ArgumentError: too few arguments"},
	}
	for _, c := range cases {
		_, err := Sprintf(c.format, c.args...)
		if err == nil || err.Error() != c.want {
			t.Errorf("Sprintf(%q,%v) err=%v want %q", c.format, c.args, err, c.want)
		}
	}
}

// TestAltFormCorners covers the alternate-form (#) branches the corpus misses:
// signed hex with a prefix, uppercase two's-complement precision, a forced
// decimal point on an exponent form, octal precision of zero, and #G of zero.
func TestAltFormCorners(t *testing.T) {
	cases := []struct {
		format, want string
		arg          any
	}{
		{"%#x", "0xff", 255},
		{"%#.5X", "0X..FFF", -1},
		{"%#e", "5.000000e+00", 5.0},
		{"%#.3o", "000", 0},
		{"%#G", "0.00000", 0.0},
		{"%.0s|", "|", "hi"},
		{"%#g", "0.00000", 0.0},
	}
	for _, c := range cases {
		got, err := Sprintf(c.format, c.arg)
		if err != nil || got != c.want {
			t.Errorf("Sprintf(%q,%v) = %q,%v want %q", c.format, c.arg, got, err, c.want)
		}
	}
}

// TestPanicPassthrough proves Format re-panics a non-*Error panic rather than
// swallowing it, so a genuine bug is not masked as a format error.
func TestPanicPassthrough(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected re-panic")
		}
		if s, ok := r.(string); !ok || s != "boom" {
			t.Fatalf("unexpected panic value %v", r)
		}
	}()
	panicValue := panicker{}
	_, _ = Format("%d", []Value{panicValue}, nil)
}

// panicker is a Value whose Int panics with a non-*Error value.
type panicker struct{}

func (panicker) Kind() Kind        { return KindInteger }
func (panicker) ToS() string       { return "" }
func (panicker) Inspect() string   { return "" }
func (panicker) ClassName() string { return "X" }
func (panicker) Int() (*big.Int, error, bool) {
	panic("boom")
}
func (panicker) Float() (float64, error, bool) { return 0, nil, false }
