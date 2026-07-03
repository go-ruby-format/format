package format

import (
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// Value is the argument model for the formatter. It is the bridge between a
// host language's value space (most notably rbgo's Ruby objects) and what the
// conversions need: an integer (possibly a Bignum), a float, a string, a
// symbol, nil, or a composite (array/hash) that the %s/%p conversions render.
//
// Callers may pass plain Go values to Sprintf/Format — int, int64, *big.Int,
// float64, string, bool, nil, []any, NamedArgs/map — which are adapted to this
// interface internally (see toValue). Implementing Value directly lets a host
// (rbgo) format its own objects without an intermediate copy: a Ruby object
// wrapper need only report its kind plus a to_s and an inspect string and, for
// numbers, an integer/float view.
type Value interface {
	// Kind reports which family of conversions the value natively satisfies.
	Kind() Kind
	// ToS is the Ruby to_s rendering, used by %s and as the textual value of
	// %{name} references.
	ToS() string
	// Inspect is the Ruby inspect rendering, used by %p.
	Inspect() string
	// ClassName names the value's Ruby class for TypeError messages
	// ("Integer", "Float", "String", "Symbol", "Array", "Hash", "nil", ...).
	ClassName() string
	// Int returns the value as an arbitrary-precision integer for the integer
	// conversions (d/i/u/x/X/o/b/B/c). ok is false when the value cannot be
	// interpreted as an integer at all (e.g. a Hash); a String that is not a
	// valid Integer() literal reports ok true with a non-nil parse error so the
	// caller can raise ArgumentError rather than TypeError.
	Int() (z *big.Int, err error, ok bool)
	// Float returns the value as a float64 for the float conversions
	// (f/e/E/g/G/a/A). The contract mirrors Int.
	Float() (f float64, err error, ok bool)
}

// Kind enumerates the value families the conversions dispatch on.
type Kind int

// The value kinds. Composite kinds (Array/Hash/Other) are rendered through ToS
// and Inspect; only Integer/Float/String/Symbol/Nil carry conversion-specific
// behavior.
const (
	KindNil Kind = iota
	KindInteger
	KindFloat
	KindString
	KindSymbol
	KindArray
	KindHash
	KindOther
)

// NamedArgs supplies the hash backing %<name>s and %{name} references. It is an
// ordered name->Value map; callers that do not care about iteration order may
// pass a plain map[string]any to Sprintf, which is wrapped automatically.
type NamedArgs struct {
	keys   []string
	values map[string]Value
}

// NewNamedArgs builds a NamedArgs from a name->Value map. Iteration order is
// the sorted key order, which only affects ToS/Inspect of the hash itself.
func NewNamedArgs(m map[string]Value) *NamedArgs {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return &NamedArgs{keys: keys, values: m}
}

// get returns the Value for key and whether it is present.
func (n *NamedArgs) get(key string) (Value, bool) {
	v, ok := n.values[key]
	return v, ok
}

// Kind reports KindHash.
func (n *NamedArgs) Kind() Kind { return KindHash }

// ClassName reports "Hash".
func (n *NamedArgs) ClassName() string { return "Hash" }

// ToS renders the hash like Ruby's Hash#to_s (== inspect).
func (n *NamedArgs) ToS() string { return n.Inspect() }

// Inspect renders the hash like Ruby's Hash#inspect, e.g. {x: 1, y: "a"}.
func (n *NamedArgs) Inspect() string {
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range n.keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(n.values[k].Inspect())
	}
	b.WriteByte('}')
	return b.String()
}

// Int reports that a Hash is not an integer.
func (n *NamedArgs) Int() (*big.Int, error, bool) { return nil, nil, false }

// Float reports that a Hash is not a float.
func (n *NamedArgs) Float() (float64, error, bool) { return 0, nil, false }

// goValue adapts a plain Go value (int/int64/*big.Int/float64/string/bool/nil)
// to the Value interface. It is the bridge used by Sprintf so Go callers need
// not construct Values by hand.
type goValue struct {
	kind  Kind
	s     string  // for KindString/KindSymbol/KindOther: the textual value
	i64   int64   // for KindInteger when i == nil: the small-integer value
	i     *big.Int // for KindInteger: set only for a magnitude exceeding int64
	f     float64
	cls   string
	elems []Value // for KindArray
}

func (g goValue) Kind() Kind        { return g.kind }
func (g goValue) ClassName() string { return g.cls }

func (g goValue) ToS() string {
	switch g.kind {
	case KindNil:
		return ""
	case KindInteger:
		return g.intString()
	case KindFloat:
		return rubyFloatToS(g.f)
	case KindArray:
		return g.Inspect()
	default: // KindString, KindSymbol, KindOther
		return g.s
	}
}

func (g goValue) Inspect() string {
	switch g.kind {
	case KindNil:
		return "nil"
	case KindInteger:
		return g.intString()
	case KindFloat:
		return rubyFloatToS(g.f)
	case KindString:
		return rubyInspectString(g.s)
	case KindArray:
		var b strings.Builder
		b.WriteByte('[')
		for i, e := range g.elems {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(e.Inspect())
		}
		b.WriteByte(']')
		return b.String()
	case KindSymbol:
		return ":" + g.s
	default: // KindOther (e.g. bool)
		return g.s
	}
}

// intString renders a KindInteger's textual value, using the allocation-free
// int64 form when the value fits (i == nil) and the *big.Int form otherwise.
func (g goValue) intString() string {
	if g.i != nil {
		return g.i.String()
	}
	return strconv.FormatInt(g.i64, 10)
}

// Int64Fast reports a genuine int64-range integer without allocating a *big.Int,
// letting the formatter's integer conversions skip math/big. Bignums (i != nil),
// floats, strings, and non-numeric values report ok=false.
func (g goValue) Int64Fast() (int64, bool) {
	if g.kind == KindInteger && g.i == nil {
		return g.i64, true
	}
	return 0, false
}

func (g goValue) Int() (*big.Int, error, bool) {
	switch g.kind {
	case KindInteger:
		if g.i != nil {
			return new(big.Int).Set(g.i), nil, true
		}
		return big.NewInt(g.i64), nil, true
	case KindFloat:
		bi, _ := big.NewFloat(g.f).Int(nil)
		return bi, nil, true
	case KindString:
		z, err := parseRubyInteger(g.s)
		return z, err, true
	default:
		return nil, nil, false
	}
}

func (g goValue) Float() (float64, error, bool) {
	switch g.kind {
	case KindInteger:
		if g.i == nil {
			return float64(g.i64), nil, true
		}
		f := new(big.Float).SetInt(g.i)
		v, _ := f.Float64()
		return v, nil, true
	case KindFloat:
		return g.f, nil, true
	case KindString:
		f, err := parseRubyFloat(g.s)
		return f, err, true
	default:
		return 0, nil, false
	}
}

// toValue wraps a plain Go argument as a Value. A value already implementing
// Value is returned unchanged so hosts can pass their own objects.
func toValue(a any) Value {
	switch x := a.(type) {
	case Value:
		return x
	case nil:
		return goValue{kind: KindNil, cls: "nil"}
	case bool:
		return goValue{kind: KindOther, s: strconv.FormatBool(x), cls: boolClass(x)}
	case int:
		return goValue{kind: KindInteger, i64: int64(x), cls: "Integer"}
	case int64:
		return goValue{kind: KindInteger, i64: x, cls: "Integer"}
	case *big.Int:
		if x.IsInt64() {
			return goValue{kind: KindInteger, i64: x.Int64(), cls: "Integer"}
		}
		return goValue{kind: KindInteger, i: new(big.Int).Set(x), cls: "Integer"}
	case float64:
		return goValue{kind: KindFloat, f: x, cls: "Float"}
	case string:
		return goValue{kind: KindString, s: x, cls: "String"}
	case Symbol:
		return goValue{kind: KindSymbol, s: string(x), cls: "Symbol"}
	case []any:
		elems := make([]Value, len(x))
		for i, e := range x {
			elems[i] = toValue(e)
		}
		return goValue{kind: KindArray, elems: elems, cls: "Array"}
	default:
		return goValue{kind: KindOther, s: toStringFallback(x), cls: "Object"}
	}
}

// Symbol is the plain-Go representation of a Ruby Symbol argument, so a Go
// caller can pass format.Symbol("name") and get :name from %p and name from %s.
type Symbol string

func boolClass(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func toStringFallback(a any) string {
	if s, ok := a.(interface{ String() string }); ok {
		return s.String()
	}
	return ""
}

// parseRubyInteger parses a string as Ruby's Integer() does for sprintf: it
// trims surrounding whitespace, accepts underscores between digits, and honors
// 0x/0o/0b/0 radix prefixes (base 0). A malformed value yields a non-nil error.
func parseRubyInteger(s string) (*big.Int, error) {
	t := strings.TrimSpace(s)
	clean := strings.ReplaceAll(t, "_", "")
	z, ok := new(big.Int).SetString(clean, 0)
	if !ok {
		return nil, &argError{"invalid value for Integer(): " + rubyInspectString(s)}
	}
	return z, nil
}

// parseRubyFloat parses a string as Ruby's Float() does for sprintf: it trims
// surrounding whitespace. A malformed value yields a non-nil error.
func parseRubyFloat(s string) (float64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, &argError{"invalid value for Float(): " + rubyInspectString(s)}
	}
	return f, nil
}
