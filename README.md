<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-format/brand/main/social/go-ruby-format.png" alt="go-ruby-format/format" width="720"></p>

# format — go-ruby-format

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-format.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of Ruby's format-string engine** — the
computation behind `Kernel#sprintf`, `Kernel#format`, and `String#%` — matching
MRI (CRuby) **4.0** byte-for-byte across every conversion, flag, width/precision
form, named/numbered reference, and the exact `ArgumentError` / `KeyError` /
`TypeError` messages.

It is the formatting backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime —
mirroring [go-ruby-regexp](https://github.com/go-ruby-regexp/regexp) (Onigmo)
and [go-ruby-erb](https://github.com/go-ruby-erb/erb) (ERB).

## Install

```sh
go get github.com/go-ruby-format/format
```

## Usage

```go
package main

import (
	"fmt"
	"math/big"

	"github.com/go-ruby-format/format"
)

func main() {
	// Plain Go values: int, int64, *big.Int, float64, string, bool, nil,
	// []any, format.Symbol.
	out, _ := format.Sprintf("%05.2f -> 0x%x", 3.14159, 255)
	fmt.Println(out) // 03.14 -> 0xff

	// Arbitrary-precision Bignum, full width.
	z := new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)
	out, _ = format.Sprintf("%d", z)
	fmt.Println(out) // 1000000000000000000000000000000

	// Ruby's two's-complement notation for negative non-decimal bases.
	out, _ = format.Sprintf("%#x", -255)
	fmt.Println(out) // 0x..f01

	// Named references from a hash; KeyError on a missing key.
	out, _ = format.Sprintf("%<name>s is %<age>d", map[string]any{
		"name": "Ada", "age": 36,
	})
	fmt.Println(out) // Ada is 36

	// MRI-matching errors.
	_, err := format.Sprintf("%d")
	fmt.Println(err) // ArgumentError: too few arguments
}
```

## Conversions

| Verb              | Meaning                                                    |
| ----------------- | ---------------------------------------------------------- |
| `d` `i` `u`       | signed decimal integer (Bignum-aware)                      |
| `f`               | fixed-point float                                          |
| `e` `E`           | scientific float                                           |
| `g` `G`           | shortest float (trailing zeros trimmed; kept under `#`)    |
| `a` `A`           | hexadecimal float                                          |
| `s`               | `to_s`                                                      |
| `p`               | `inspect`                                                  |
| `x` `X` `o` `b` `B` | hex / octal / binary integer (`..` two's-complement form)|
| `c`               | character (code point or one-character string)             |
| `%%`              | a literal percent                                          |

**Flags** `-` `+` *space* `0` `#` · **width/precision** numeric, `*` (from args),
and named · **`%n$`** absolute argument references · **`%<name>`** and
**`%{name}`** hash references · Bignum at full precision · the `Inf` / `-Inf` /
`NaN` spellings.

## Argument model

`Sprintf(format string, args ...any) (string, error)` accepts plain Go values
and adapts them internally. A host such as `rbgo` that already holds typed value
objects implements the small `Value` interface and calls
`Format(format string, args []Value, named *NamedArgs) (string, error)`, so its
objects are formatted directly with no intermediate copy. A `*NamedArgs`,
`map[string]Value`, or `map[string]any` supplies the hash for named references.

## Tests & coverage

The package is verified two ways:

- a **differential** test runs a broad corpus through both this package and the
  live **MRI `ruby` oracle**, comparing output — and raised exception class and
  message — byte-for-byte (it self-skips where `ruby` is absent, e.g. the qemu
  and Windows lanes);
- a **deterministic golden** test embeds the MRI-captured results so the
  oracle-free lanes still exercise the entire surface and hold the **100.0%
  coverage** gate.

```sh
go test ./...
```

CI enforces 100% coverage, `go vet`, and **CGO=0** builds on all six 64-bit Go
targets: `amd64`, `arm64`, `riscv64`, `loong64`, `ppc64le`, `s390x`.

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-format/format
authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
