# format examples

Runnable pure-Ruby usage of the `format` engine — the computation behind
`Kernel#format`, `Kernel#sprintf`, and `String#%` — verified under the rbgo
interpreter.

```sh
rbgo examples/format_usage.rb
```

| File               | Shows                                                                   |
| ------------------ | ----------------------------------------------------------------------- |
| `format_usage.rb`  | `format`/`sprintf`/`%` with width, precision, flags, and zero-padding   |
|                    | Integer conversions (`%d %b %o %x %X`) and arbitrary-precision Bignum    |
|                    | Named (`%<name>s`) and absolute (`%3$s`) argument references             |
|                    | Literal percent (`%%`) and MRI-matching `ArgumentError` handling         |
