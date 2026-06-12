# alpacadecimal
Similar and compatible with [decimal.Decimal](https://pkg.go.dev/github.com/shopspring/decimal), but optimized for Alpaca's data sets.

### Goal
- optimize for Alpaca data sets (99% of decimals are within 10 millions with up to 12 precisions).
- compatible with [decimal.Decimal](https://pkg.go.dev/github.com/shopspring/decimal) so that it could be a drop-in replacement for current `decimal.Decimal` usage.

### Key Ideas

The original `decimal.Decimal` package has bottleneck on `big.Int` operations, e.g. sql serialization / deserialization, addition, multiplication etc. These operations took fair amount cpu and memory during our profiling / monitoring.

The optimization this library is to represent most decimal numbers with `int64` instead of `big.Int`. To keep this library compatible with the original `decimal.Decimal` package, [AlexandrosKyriakakis/zerodecimal](https://github.com/AlexandrosKyriakakis/zerodecimal) (a zero-allocation, panic-free 128-bit decimal engine) is used as a fallback when `int64` is not enough (number is too big / small, or has too many digits).

The core data struct is:

```golang
type Decimal struct {
	// represent decimal with 12 fractional digits; 1.23 is stored as
	// fixed = 1_230_000_000_000
	// max value: 9_223_372.000_000_000_000
	// min value: -9_223_372.000_000_000_000
	fixed int64

	// fallback for out-of-range values; nil means fixed is authoritative
	fallback *zerodecimal.Decimal
}
```

The struct is 16 bytes (same as shopspring's), the zero value is `0`, the fixed path never allocates, and the hot methods are small enough to inline. Arithmetic on the fixed path uses 64/128-bit integer math (`math/bits`); division by the constant 10^12 uses a precomputed Möller–Granlund reciprocal.

We pick 12 fractional digits because it covers 99% of Alpaca's common cases.

### Compatibility

`alpacadecimal.Decimal` is designed as a drop-in replacement for `decimal.Decimal`: the full API surface (including `Sin`/`Cos`/`Tan`/`Atan`, `Ln`, `ExpTaylor`, `ExpHullAbrham`, the `Pow` family, `RescalePair`, and package variables like `DivisionPrecision`, `PowPrecisionNegativeExponent` and `ExpMaxIterations`), the same parsing acceptance (including scientific notation), the same rounding semantics in every mode and at any `places` value, digit-exact native ports of shopspring's math (`Ln`/`ExpTaylor`/trig — verified value-identical against shopspring v1.4.0), and a binary/gob wire format that is byte-compatible with shopspring's in both directions.

Verified by a fuzzing suite (`fuzz/`) that compares every operation against shopspring/decimal as the reference, plus a parity unit-test suite (`parity_test.go`) whose expected values were generated with shopspring v1.4.0.

There are a few documented differences:

- **Precision is capped at 19 fractional digits** (the zerodecimal engine's limit). Digits beyond 19 are truncated by default; call `SetDefaultParseModeError()` to reject such inputs instead. shopspring keeps arbitrary precision. The same convention applies to results of `Div`/`Pow`/etc. that would need more than 19 fractional digits.
- **The coefficient is bounded to 128 bits** (zerodecimal has no `big.Int` escape hatch, by design). Values up to ±2^128 (~3.4e38, i.e. up to 39 significant digits) are representable. When an exact result's coefficient at 19 fractional digits would exceed 2^128, fractional digits are truncated until it fits (an extension of the 19-digit convention above). Values whose **integer part** alone exceeds 2^128 are out of domain: parse and decode paths (`NewFromString`, `UnmarshalJSON`/`Text`/`Binary`, `Scan`) return an error, and infallible API paths (`Add`, `Mul`, `Pow`, `New`, ...) panic with `alpacadecimal: value exceeds the 128-bit decimal domain (|value| >= 2^128)`. shopspring is unbounded. For Alpaca-style data (prices, quantities) this boundary is ~10^31 away from real values.
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

- Scientific-notation exponents are capped at ±10000 when parsing, and binary (gob) exponents likewise (a DoS guard: shopspring stores huge exponents lazily, we materialize them). Internal exact big.Int intermediates are additionally capped at 10^100000.
- shopspring's accidental acceptance of a sign after a leading dot (`".-5"` parses as `-0.05` there) is deliberately not replicated; such input is rejected.

### Benchmarks

Run against shopspring/decimal v1.4.0, quagmt/udecimal v1.10.0 and govalues/decimal v0.1.36 on an Apple M1 Pro (`make bench`). "optimized" is the int64 fast path (the 99% case), "fallback" is the zerodecimal path. Bold marks the fastest library for the operation:

| operation | alpacadecimal (optimized) | alpacadecimal (fallback) | shopspring | udecimal | govalues |
|---|---|---|---|---|---|
| Abs | **0.31 ns** | 13.02 ns | 28.50 ns | 2.03 ns | **0.31 ns** |
| Add | **2.03 ns** | 18.02 ns | 39.91 ns | 4.41 ns | 5.29 ns |
| Avg | **10.77 ns** | 74.09 ns | 288.65 ns | 31.02 ns | 45.55 ns |
| BigFloat | **222.75 ns** | 256.55 ns | 348.25 ns | — | — |
| BigInt | **26.98 ns** | 68.91 ns | 113.15 ns | — | — |
| Ceil | **2.14 ns** | 19.58 ns | 139.00 ns | 5.68 ns | 2.80 ns |
| Cmp | **2.03 ns** | 7.17 ns | 69.66 ns | 6.27 ns | 6.23 ns |
| Coefficient | **26.76 ns** | 28.00 ns | 27.46 ns | — | — |
| CoefficientInt64 | 2.09 ns | 2.03 ns | 0.39 ns | — | **0.31 ns** |
| Copy | **0.31 ns** | 0.32 ns | 28.90 ns | — | — |
| Div | **5.90 ns** | 36.58 ns | 189.05 ns | 14.14 ns | 32.53 ns |
| DivRound | **7.18 ns** | 36.37 ns | 174.25 ns | 25.22 ns | 281.65 ns |
| Equal | **2.03 ns** | 6.23 ns | 69.60 ns | 5.97 ns | 6.54 ns |
| Exponent | **0.31 ns** | **0.31 ns** | **0.31 ns** | — | **0.31 ns** |
| Float64 | **1.93 ns** | 215.85 ns | 233.25 ns | — | 48.27 ns |
| Floor | **2.02 ns** | 19.48 ns | 116.65 ns | 4.98 ns | 2.80 ns |
| GobDecode | **7.48 ns** | 20.92 ns | 32.91 ns | — | — |
| GobEncode | **18.18 ns** | 21.61 ns | 30.22 ns | — | — |
| GreaterThan | **2.03 ns** | 5.92 ns | 69.75 ns | 6.28 ns | — |
| GreaterThanOrEqual | **2.03 ns** | 5.92 ns | 69.71 ns | 6.26 ns | — |
| InexactFloat64 | **2.02 ns** | **2.03 ns** | 231.35 ns | 68.31 ns | — |
| IntPart | **2.05 ns** | 4.05 ns | 113.30 ns | 6.23 ns | 3.74 ns |
| IsInteger | **0.31 ns** | 2.50 ns | — | — | 0.78 ns |
| IsNegative | **0.31 ns** | **0.31 ns** | **0.31 ns** | 2.02 ns | **0.31 ns** |
| IsPositive | **0.31 ns** | **0.31 ns** | **0.31 ns** | 2.03 ns | **0.31 ns** |
| IsZero | **0.31 ns** | **0.31 ns** | **0.31 ns** | 2.03 ns | **0.31 ns** |
| LessThan | **2.03 ns** | 5.91 ns | 69.31 ns | 5.97 ns | 6.54 ns |
| LessThanOrEqual | **2.03 ns** | 5.92 ns | 69.52 ns | 6.27 ns | — |
| MarshalBinary | 18.12 ns | 21.71 ns | 30.06 ns | **17.34 ns** | 26.99 ns |
| MarshalJSON | 181.70 ns | 205.45 ns | 316.70 ns | 193.75 ns | **171.90 ns** |
| MarshalText | **26.49 ns** | 35.58 ns | 135.45 ns | 40.67 ns | **26.88 ns** |
| Max | **5.25 ns** | 10.59 ns | 11.63 ns | 16.69 ns | 13.70 ns |
| Min | **5.25 ns** | 10.59 ns | 11.68 ns | 16.62 ns | 13.39 ns |
| Mod | **2.03 ns** | 16.04 ns | 131.40 ns | 14.89 ns | 9.03 ns |
| Mul | **4.77 ns** | 19.29 ns | 40.79 ns | 5.97 ns | 5.31 ns |
| Neg | **0.31 ns** | 13.16 ns | 27.04 ns | 2.04 ns | **0.31 ns** |
| New | **2.09 ns** | 18.26 ns | 25.59 ns | 2.82 ns | 2.19 ns |
| NewFromBigInt | **3.12 ns** | 19.46 ns | 26.98 ns | — | — |
| NewFromFloat | **63.75 ns** | 115.15 ns | 373.15 ns | **63.37 ns** | 70.75 ns |
| NewFromFloat32 | 184.30 ns | 175.70 ns | 183.30 ns | **54.28 ns** | **54.40 ns** |
| NewFromFloatWithExponent | **143.75 ns** | 179.45 ns | 174.10 ns | — | — |
| NewFromFormattedString | **127.30 ns** | 249.45 ns | 269.00 ns | — | — |
| NewFromInt | **0.31 ns** | 12.78 ns | 25.10 ns | 2.76 ns | 2.18 ns |
| NewFromInt32 | **0.31 ns** | 12.86 ns | 25.18 ns | 2.77 ns | 2.25 ns |
| NewFromString | **6.58 ns** | 73.01 ns | 102.70 ns | 21.30 ns | 52.70 ns |
| NumDigits | 2.50 ns | **2.26 ns** | 6.04 ns | — | 3.74 ns |
| Pow | **16.98 ns** | 107.55 ns | 236.85 ns | 36.94 ns | 196.90 ns |
| QuoRem | **5.37 ns** | 35.57 ns | 131.85 ns | 14.75 ns | 9.04 ns |
| Rat | **117.65 ns** | 155.20 ns | — | — | — |
| Round | **1.98 ns** | 18.61 ns | 150.20 ns | 5.30 ns | 2.81 ns |
| RoundBank | **2.59 ns** | 19.52 ns | 454.40 ns | 5.30 ns | 2.80 ns |
| RoundCash | **14.47 ns** | 97.63 ns | 537.55 ns | — | — |
| RoundCeil | **2.04 ns** | 19.52 ns | 272.90 ns | — | 2.80 ns |
| RoundDown | 177.05 ns | **145.10 ns** | 12123.50 ns | 303.10 ns | 163.65 ns |
| RoundFloor | **1.96 ns** | 19.56 ns | 248.35 ns | — | 2.80 ns |
| RoundUp | 177.60 ns | **82.33 ns** | 8305.50 ns | 230.05 ns | 178.45 ns |
| Scan | **10.16 ns** | 72.33 ns | 115.70 ns | 21.81 ns | 50.61 ns |
| Shift | **2.19 ns** | 50.75 ns | 27.68 ns | — | — |
| Sign | **0.31 ns** | 0.47 ns | **0.31 ns** | 2.03 ns | **0.31 ns** |
| String | **2.26 ns** | 22.93 ns | 122.95 ns | 38.27 ns | 25.50 ns |
| StringFixed | **17.66 ns** | 45.79 ns | 276.35 ns | 52.08 ns | 25.59 ns |
| StringFixedBank | **17.98 ns** | 47.02 ns | 565.95 ns | — | 25.60 ns |
| StringFixedCash | **29.76 ns** | 148.75 ns | 663.60 ns | — | — |
| Sub | **2.03 ns** | 19.52 ns | 33.11 ns | 6.59 ns | 6.23 ns |
| Sum | **4.88 ns** | 37.35 ns | 81.64 ns | 20.62 ns | 24.02 ns |
| Truncate | **2.05 ns** | 19.20 ns | 111.80 ns | 4.82 ns | 2.49 ns |
| UnmarshalBinary | 7.48 ns | 20.81 ns | 32.43 ns | **2.49 ns** | 37.82 ns |
| UnmarshalJSON | **104.40 ns** | 187.70 ns | 236.55 ns | 129.05 ns | 150.70 ns |
| UnmarshalText | **10.59 ns** | 74.03 ns | 98.92 ns | 17.74 ns | 37.83 ns |
| Value | 40.54 ns | **38.77 ns** | 135.80 ns | 53.86 ns | 40.35 ns |

Notes:
- sub-nanosecond entries are fully inlined and partially dead-code-eliminated by the benchmark loop; treat them as "free".
- `NewFromFloat32` deliberately uses shopspring's exact shortest-representation algorithm (which differs from strconv's in rare 1-ulp cases), trading speed for digit parity.
- `UnmarshalBinary`/`GobDecode` pay for decoding shopspring's wire format, which udecimal's own format-specific decoder doesn't.
- requires Go 1.26+ (the zerodecimal engine's floor).
- govalues clamps negative rounding scales to zero, so `RoundUp`/`RoundDown` loops are not fully comparable.

### Profile-guided optimization (PGO)

The repo ships a representative CPU profile at `default.pgo`, generated by
`make pgo-profile` from a workload modeled on the package's design target
(fixed-path prices/quantities through parsing, arithmetic, rounding,
comparison and serialization, with ~1% fallback traffic).

Go applies PGO when building **binaries**, so to benefit in your service,
point your build at the shipped profile (or merge it into your own profile —
see `go tool pprof -proto`):

```
go build -pgo=$(go list -m -f '{{.Dir}}' github.com/alpacahq/alpacadecimal)/default.pgo ./cmd/yourservice
```

Measured effect on this library's own benchmark suite (Apple M1 Pro,
`make bench-pgo`, benchstat over 6 runs): **-2.0% geomean** across the hot
operations, with larger wins where PGO unlocks cross-function inlining into
the zerodecimal fallback — Mod -11%, Div -9.5%, Avg -9.3%, Pow -8.8%,
mixed-representation Cmp -8.5%. (The gain is smaller than it was with the
previous fallback engine because zerodecimal's hot paths are already written
PGO-friendly and the baseline is faster.)

```
make pgo-profile   # regenerate default.pgo
make bench-pgo     # benchstat comparison: -pgo=off vs the shipped profile
```

### Fuzzing

```
make fuzz-seeds   # run all fuzz targets against their seed corpora
make fuzz         # continuous fuzzing against shopspring as reference
```

### Related Issues
- `big.NewInt` optimization from [here](https://go-review.googlesource.com/c/go/+/411254) might help to speed up some `big.Int` related operations.
- `big.Int.String` slowness is tracked by [this issue](https://github.com/golang/go/issues/20906). The approach we reduce this slowness is to use int64 to represent the number if possible to avoid `big.Int` operations.
- the fallback engine is [AlexandrosKyriakakis/zerodecimal](https://github.com/AlexandrosKyriakakis/zerodecimal); see `benchmarks/bench-zerodecimal-vs-udecimal.txt` for the engine-swap comparison against the previous fallback (quagmt/udecimal): -18.9% time geomean across this suite, -25% bytes/op on fallback values.
- upstream optimizations contributed to the previous fallback engine: [quagmt/udecimal#45](https://github.com/quagmt/udecimal/pull/45).
