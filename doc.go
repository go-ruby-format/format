/*
Package format is a pure-Go (no cgo) implementation of Ruby's format-string
engine — the computation behind Kernel#sprintf, Kernel#format, and String#% —
compatible with MRI (CRuby) 4.0 byte-for-byte.

It is the formatting backend for go-embedded-ruby (rbgo) but is a standalone,
reusable module with no dependency on the Ruby runtime, mirroring the way
go-ruby-regexp provides Onigmo and go-ruby-erb provides ERB.

# Entry points

Sprintf is the convenience form taking plain Go values:

	out, err := format.Sprintf("%05.2f -> 0x%x", 3.14159, 255)
	// out == "03.14 -> 0xff"

Format is the explicit form for hosts (rbgo) that already hold typed values:

	out, err := format.Format("%<name>s is %<age>d", nil, named)

# Argument model

A plain Go caller may pass int, int64, *big.Int, float64, string, bool, nil,
[]any, and Symbol; a *NamedArgs, map[string]Value, or map[string]any supplies
the hash backing %<name>s and %{name} references. A host that holds its own
value objects (an interpreter) implements the Value interface so its objects are
formatted directly, without an intermediate copy.

# Coverage

Every MRI conversion is implemented:

  - d, i, u — signed decimal integer
  - f — fixed-point float
  - e, E — scientific float
  - g, G — shortest float, trailing zeros trimmed (kept under #)
  - a, A — hexadecimal float
  - s — to_s
  - p — inspect
  - x, X — hexadecimal integer
  - o — octal integer
  - b, B — binary integer
  - c — character (code point or one-character string)
  - %% — a literal percent

with the -, +, space, 0, and # flags; numeric, *, and named width/precision;
%n$ absolute argument references; %<name> and %{name} hash references; arbitrary-
precision Bignum integers; Ruby's two's-complement ".." notation for negative
x/o/b values; the Inf/-Inf/NaN spellings; and the MRI ArgumentError, KeyError,
and TypeError messages — all validated against the live CRuby oracle by a broad
differential test, with a deterministic golden corpus so the oracle-free CI
lanes still exercise the full surface.
*/
package format
