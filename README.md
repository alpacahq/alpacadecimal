# alpacadecimal
Similar and compatible with [decimal.Decimal](https://pkg.go.dev/github.com/shopspring/decimal), but optimized for Alpaca's data sets.

### Goal
- optimize for Alpaca data sets (99% of decimals are within 10 millions with up to 12 precisions).
- compatible with [decimal.Decimal](https://pkg.go.dev/github.com/shopspring/decimal) so that it could be a drop-in replacement for current `decimal.Decimal` usage.

### Key Ideas

The original `decimal.Decimal` package has bottleneck on `big.Int` operations, e.g. sql serialization / deserialization, addition, multiplication etc. These operations took fair amount cpu and memory during our profiling / monitoring.

The optimization this library is to represent most decimal numbers with `int64` instead of `big.Int`. To keep this library compatible with the original `decimal.Decimal` package, [quagmt/udecimal](https://github.com/quagmt/udecimal) (a 128-bit decimal engine) is used as a fallback when `int64` is not enough (number is too big / small, or has too many digits).

The core data struct is:

```golang
type Decimal struct {
	// represent decimal with 12 fractional digits; 1.23 is stored as
	// fixed = 1_230_000_000_000
	// max value: 9_223_372.000_000_000_000
	// min value: -9_223_372.000_000_000_000
	fixed int64

	// fallback for out-of-range values; nil means fixed is authoritative
	fallback *udecimal.Decimal
}
```

The struct is 16 bytes (same as shopspring's), the zero value is `0`, the fixed path never allocates, and the hot methods are small enough to inline. Arithmetic on the fixed path uses 64/128-bit integer math (`math/bits`); division by the constant 10^12 uses a precomputed Möller–Granlund reciprocal.

We pick 12 fractional digits because it covers 99% of Alpaca's common cases.

### Compatibility

`alpacadecimal.Decimal` is designed as a drop-in replacement for `decimal.Decimal`: the full API surface (including `Sin`/`Cos`/`Tan`/`Atan`, `Ln`, `ExpTaylor`, `ExpHullAbrham`, the `Pow` family, `RescalePair`, and package variables like `DivisionPrecision`, `PowPrecisionNegativeExponent` and `ExpMaxIterations`), the same parsing acceptance (including scientific notation), the same rounding semantics in every mode and at any `places` value, digit-exact native ports of shopspring's math (`Ln`/`ExpTaylor`/trig — verified value-identical against shopspring v1.4.0), and a binary/gob wire format that is byte-compatible with shopspring's in both directions.

Verified by a fuzzing suite (`fuzz/`) that compares every operation against shopspring/decimal as the reference, plus a parity unit-test suite (`parity_test.go`) whose expected values were generated with shopspring v1.4.0.

There are a few documented differences:

- **Precision is capped at 19 fractional digits** (the udecimal engine's limit). Digits beyond 19 are truncated by default; call `SetDefaultParseModeError()` to reject such inputs instead. shopspring keeps arbitrary precision. The same convention applies to results of `Div`/`Pow`/etc. that would need more than 19 fractional digits.
- **Representation introspection differs**: `Exponent()`, `Coefficient()`, `CoefficientInt64()` and `NumDigits()` return different (but valid) representations of equal values. For optimized values the exponent is always -12:

```golang
x := alpacadecimal.NewFromInt(123)
require.Equal(t, int32(-12), x.Exponent())
require.Equal(t, "123000000000000", x.Coefficient().String())
require.Equal(t, int64(123000000000000), x.CoefficientInt64())
require.Equal(t, 15, x.NumDigits())

y := decimal.NewFromInt(123)
require.Equal(t, int32(0), y.Exponent())
require.Equal(t, "123", y.Coefficient().String())
require.Equal(t, int64(123), y.CoefficientInt64())
require.Equal(t, 3, y.NumDigits())
```

- Scientific-notation exponents are capped at ±10000 when parsing, and binary (gob) exponents likewise (a DoS guard: shopspring stores huge exponents lazily, we materialize them). Programmatic construction (`New`, `Shift`) is capped at 10^100000 and panics with a clear message beyond it instead of pinning a CPU.
- shopspring's accidental acceptance of a sign after a leading dot (`".-5"` parses as `-0.05` there) is deliberately not replicated; such input is rejected.

### Benchmarks

Run against shopspring/decimal v1.4.0, quagmt/udecimal v1.10.0 and govalues/decimal v0.1.36 on an Apple M1 Pro (`make bench`). "optimized" is the int64 fast path (the 99% case), "fallback" is the udecimal path. Bold marks the fastest library for the operation:

| operation | alpacadecimal (optimized) | alpacadecimal (fallback) | shopspring | udecimal | govalues |
|---|---|---|---|---|---|
| Abs | **0.33 ns** | 22.72 ns | 32.18 ns | 2.14 ns | **0.33 ns** |
| Add | **2.13 ns** | 26.67 ns | 44.73 ns | 4.56 ns | 5.55 ns |
| Avg | **12.22 ns** | 117.50 ns | 315.00 ns | 31.81 ns | 49.11 ns |
| BigFloat | **234.90 ns** | 273.80 ns | 367.20 ns | — | — |
| BigInt | **29.11 ns** | 72.38 ns | 120.00 ns | — | — |
| Ceil | **2.23 ns** | 27.41 ns | 152.50 ns | 5.80 ns | 2.91 ns |
| Cmp | **2.14 ns** | 7.95 ns | 75.62 ns | 6.47 ns | 6.44 ns |
| Coefficient | **29.48 ns** | 29.34 ns | **29.76 ns** | — | — |
| CoefficientInt64 | 2.12 ns | 2.12 ns | 0.41 ns | — | **0.32 ns** |
| Copy | **0.32 ns** | 0.32 ns | 30.20 ns | — | — |
| Div | **6.15 ns** | 60.98 ns | 203.00 ns | 13.96 ns | 33.83 ns |
| DivRound | **10.09 ns** | 58.29 ns | 184.70 ns | 23.46 ns | 291.90 ns |
| Equal | **2.11 ns** | 7.71 ns | 72.92 ns | 6.12 ns | 6.75 ns |
| Exponent | **0.32 ns** | 0.32 ns | **0.32 ns** | — | **0.32 ns** |
| Float64 | **2.01 ns** | 229.30 ns | 247.30 ns | — | 50.74 ns |
| Floor | **2.09 ns** | 27.00 ns | 126.50 ns | 5.17 ns | 2.89 ns |
| GobDecode | **8.12 ns** | 27.01 ns | 35.38 ns | — | — |
| GobEncode | **19.51 ns** | 24.58 ns | 32.25 ns | — | — |
| GreaterThan | **2.10 ns** | 8.07 ns | 74.94 ns | 6.44 ns | — |
| GreaterThanOrEqual | **2.11 ns** | 7.80 ns | 74.27 ns | 6.47 ns | — |
| InexactFloat64 | **2.10 ns** | 2.11 ns | 245.70 ns | 71.11 ns | — |
| IntPart | **2.12 ns** | 7.46 ns | 121.40 ns | 6.49 ns | 3.87 ns |
| IsInteger | **0.32 ns** | 2.58 ns | — | — | 0.81 ns |
| IsNegative | 2.11 ns | 2.26 ns | **0.32 ns** | 2.10 ns | **0.32 ns** |
| IsPositive | 2.11 ns | 2.30 ns | **0.32 ns** | 2.10 ns | **0.32 ns** |
| IsZero | **0.32 ns** | 2.13 ns | **0.32 ns** | 2.11 ns | **0.33 ns** |
| LessThan | **2.11 ns** | 7.80 ns | 75.48 ns | 6.18 ns | 6.80 ns |
| LessThanOrEqual | **2.12 ns** | 7.84 ns | 75.12 ns | 6.47 ns | — |
| MarshalBinary | 18.91 ns | 24.28 ns | 31.53 ns | **18.31 ns** | 28.79 ns |
| MarshalJSON | 192.40 ns | 216.90 ns | 327.20 ns | 207.80 ns | **185.00 ns** |
| MarshalText | 32.27 ns | 38.85 ns | 132.60 ns | 43.49 ns | **28.67 ns** |
| Max | **5.47 ns** | 16.31 ns | 12.14 ns | 17.51 ns | 14.32 ns |
| Min | **5.49 ns** | 16.38 ns | 12.06 ns | 17.40 ns | 13.92 ns |
| Mod | **2.12 ns** | 25.71 ns | 140.70 ns | 15.51 ns | 9.46 ns |
| Mul | **5.05 ns** | 29.94 ns | 43.59 ns | 6.22 ns | 5.51 ns |
| Neg | **0.32 ns** | 21.55 ns | 29.40 ns | 2.12 ns | **0.33 ns** |
| New | **2.20 ns** | 25.02 ns | 27.55 ns | 2.96 ns | 2.27 ns |
| NewFromBigInt | **3.33 ns** | 27.39 ns | 31.01 ns | — | — |
| NewFromFloat | **65.45 ns** | 118.40 ns | 387.90 ns | **65.77 ns** | 72.73 ns |
| NewFromFloat32 | 188.10 ns | 187.80 ns | 196.80 ns | **57.52 ns** | **56.98 ns** |
| NewFromFloatWithExponent | **158.80 ns** | 199.90 ns | 191.10 ns | — | — |
| NewFromFormattedString | **137.00 ns** | 267.00 ns | 289.00 ns | — | — |
| NewFromInt | **2.07 ns** | 23.12 ns | 58.75 ns | 4.40 ns | 2.46 ns |
| NewFromInt32 | **2.13 ns** | 23.98 ns | 27.92 ns | 3.07 ns | 2.40 ns |
| NewFromString | **6.85 ns** | 74.06 ns | 106.30 ns | 21.55 ns | 52.94 ns |
| NumDigits | **2.60 ns** | 2.55 ns | 6.30 ns | — | 3.90 ns |
| Pow | **18.53 ns** | 120.70 ns | 258.60 ns | 38.26 ns | 203.00 ns |
| QuoRem | **5.58 ns** | 83.45 ns | 141.00 ns | 15.52 ns | 9.40 ns |
| Rat | **125.50 ns** | 167.00 ns | — | — | — |
| Round | **2.06 ns** | 28.99 ns | 160.10 ns | 5.49 ns | 2.91 ns |
| RoundBank | **2.67 ns** | 26.42 ns | 482.90 ns | 5.47 ns | 2.88 ns |
| RoundCash | **15.80 ns** | 153.20 ns | 577.90 ns | — | — |
| RoundCeil | **2.12 ns** | 41.77 ns | 286.50 ns | — | 2.90 ns |
| RoundDown | 183.30 ns | 177.10 ns | 12991.00 ns | 313.30 ns | **169.00 ns** |
| RoundFloor | **2.06 ns** | 34.88 ns | 270.40 ns | — | 2.99 ns |
| RoundUp | **183.60 ns** | 90.75 ns | 8954.00 ns | 238.50 ns | **185.50 ns** |
| Scan | **10.52 ns** | 74.04 ns | 121.30 ns | 22.63 ns | 52.79 ns |
| Shift | **2.59 ns** | 62.97 ns | 30.69 ns | — | — |
| Sign | 2.12 ns | 2.92 ns | **0.32 ns** | 2.12 ns | **0.32 ns** |
| String | **2.34 ns** | 25.63 ns | 119.20 ns | 41.23 ns | 26.61 ns |
| StringFixed | **18.58 ns** | 64.14 ns | 301.50 ns | 55.45 ns | 28.03 ns |
| StringFixedBank | **18.96 ns** | 58.10 ns | 602.00 ns | — | 26.99 ns |
| StringFixedCash | **31.93 ns** | 206.60 ns | 702.80 ns | — | — |
| Sub | **2.15 ns** | 29.14 ns | 35.98 ns | 6.95 ns | 6.54 ns |
| Sum | **4.95 ns** | 55.93 ns | 92.68 ns | 21.87 ns | 26.91 ns |
| Truncate | **2.12 ns** | 26.66 ns | 120.50 ns | 5.01 ns | 2.58 ns |
| UnmarshalBinary | 8.04 ns | 26.52 ns | 36.09 ns | **2.60 ns** | 39.13 ns |
| UnmarshalJSON | **114.00 ns** | 198.10 ns | 253.40 ns | 139.50 ns | 158.80 ns |
| UnmarshalText | **10.99 ns** | 78.66 ns | 105.10 ns | 18.37 ns | 39.20 ns |
| Value | **43.15 ns** | 42.52 ns | 134.00 ns | 56.53 ns | 44.67 ns |

Notes:
- sub-nanosecond entries are fully inlined and partially dead-code-eliminated by the benchmark loop; treat them as "free".
- `NewFromFloat32` deliberately uses shopspring's exact shortest-representation algorithm (which differs from strconv's in rare 1-ulp cases), trading speed for digit parity.
- `UnmarshalBinary`/`GobDecode` pay for decoding shopspring's wire format, which udecimal's own format-specific decoder doesn't.
- govalues clamps negative rounding scales to zero, so `RoundUp`/`RoundDown` loops are not fully comparable.

### Fuzzing

```
make fuzz-seeds   # run all fuzz targets against their seed corpora
make fuzz         # continuous fuzzing against shopspring as reference
```

### Related Issues
- `big.NewInt` optimization from [here](https://go-review.googlesource.com/c/go/+/411254) might help to speed up some `big.Int` related operations.
- `big.Int.String` slowness is tracked by [this issue](https://github.com/golang/go/issues/20906). The approach we reduce this slowness is to use int64 to represent the number if possible to avoid `big.Int` operations.
- upstream udecimal optimizations for the fallback path: [quagmt/udecimal#45](https://github.com/quagmt/udecimal/pull/45).
