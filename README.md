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

Run against shopspring/decimal v1.4.0, quagmt/udecimal v1.10.0, AlexandrosKyriakakis/zerodecimal (the raw fallback engine, standalone) and govalues/decimal v0.1.36 on an Apple M1 Pro (`make bench`). "optimized" is the int64 fast path (the 99% case), "fallback" is the zerodecimal path. Bold marks the fastest library for the operation:

| operation | alpacadecimal (optimized) | alpacadecimal (fallback) | shopspring | udecimal | zerodecimal | govalues |
|---|---|---|---|---|---|---|
| Abs | **0.31 ns** | 12.96 ns | 28.21 ns | 2.03 ns | **0.31 ns** | **0.31 ns** |
| Add | **2.03 ns** | 18.28 ns | 39.40 ns | 4.41 ns | **2.03 ns** | 5.29 ns |
| Avg | **10.76 ns** | 74.83 ns | 290.50 ns | 30.79 ns | 16.84 ns | 47.80 ns |
| BigFloat | **221.20 ns** | 256.55 ns | 347.45 ns | — | — | — |
| BigInt | **26.72 ns** | 68.92 ns | 112.55 ns | — | — | — |
| Ceil | **2.14 ns** | 19.56 ns | 139.35 ns | 5.73 ns | 4.52 ns | 2.90 ns |
| Cmp | **2.03 ns** | 6.12 ns | 70.01 ns | 6.28 ns | 4.36 ns | 6.23 ns |
| Coefficient | **26.84 ns** | 27.75 ns | 27.76 ns | — | — | — |
| CoefficientInt64 | 2.03 ns | 2.10 ns | 0.41 ns | — | — | **0.31 ns** |
| Copy | **0.31 ns** | **0.31 ns** | 27.34 ns | — | — | — |
| Div | **5.87 ns** | 36.40 ns | 188.60 ns | 13.48 ns | 11.19 ns | 37.16 ns |
| DivRound | **7.19 ns** | 36.35 ns | 171.45 ns | 22.42 ns | — | 289.55 ns |
| Equal | **2.03 ns** | 6.23 ns | 69.93 ns | 5.99 ns | 4.71 ns | 6.55 ns |
| Exponent | **0.31 ns** | **0.31 ns** | **0.31 ns** | — | — | **0.31 ns** |
| Float64 | **1.93 ns** | 217.25 ns | 232.70 ns | — | — | 48.26 ns |
| Floor | **2.05 ns** | 19.77 ns | 118.50 ns | 5.01 ns | 4.40 ns | 2.84 ns |
| GobDecode | **7.49 ns** | 21.14 ns | 32.50 ns | — | — | — |
| GobEncode | **17.82 ns** | 21.61 ns | 30.06 ns | — | — | — |
| GreaterThan | **2.04 ns** | 5.93 ns | 70.78 ns | 6.28 ns | 4.36 ns | — |
| GreaterThanOrEqual | **2.03 ns** | 5.92 ns | 70.01 ns | 6.24 ns | 4.37 ns | — |
| InexactFloat64 | **2.03 ns** | **2.03 ns** | 232.45 ns | 68.45 ns | 43.73 ns | — |
| IntPart | **2.05 ns** | 4.05 ns | 113.95 ns | 6.23 ns | 3.04 ns | 3.74 ns |
| IsInteger | **0.31 ns** | 2.50 ns | — | — | — | 0.78 ns |
| IsNegative | **0.31 ns** | **0.31 ns** | **0.32 ns** | 2.09 ns | **0.31 ns** | **0.31 ns** |
| IsPositive | **0.31 ns** | **0.31 ns** | **0.31 ns** | 2.03 ns | **0.31 ns** | **0.31 ns** |
| IsZero | **0.31 ns** | **0.31 ns** | **0.31 ns** | 2.03 ns | **0.31 ns** | **0.31 ns** |
| LessThan | **2.04 ns** | 5.93 ns | 69.58 ns | 5.93 ns | 4.51 ns | 6.74 ns |
| LessThanOrEqual | **2.03 ns** | 5.92 ns | 69.13 ns | 5.92 ns | 4.36 ns | — |
| MarshalBinary | 17.83 ns | 21.66 ns | 29.88 ns | 17.31 ns | **11.71 ns** | 26.88 ns |
| MarshalJSON | 181.10 ns | 204.05 ns | 307.90 ns | 194.15 ns | **173.00 ns** | 178.15 ns |
| MarshalText | 26.38 ns | 35.63 ns | 124.80 ns | 40.55 ns | **23.84 ns** | 26.94 ns |
| Max | **5.25 ns** | 10.59 ns | 12.21 ns | 16.69 ns | 8.72 ns | 13.71 ns |
| Min | **5.28 ns** | 10.60 ns | 11.79 ns | 17.22 ns | 9.33 ns | 13.45 ns |
| Mod | **2.04 ns** | 16.11 ns | 131.95 ns | 14.88 ns | 11.99 ns | 9.05 ns |
| Mul | 4.91 ns | 19.16 ns | 40.36 ns | 5.95 ns | **2.04 ns** | 5.29 ns |
| Neg | **0.31 ns** | 13.23 ns | 26.95 ns | 2.03 ns | **0.31 ns** | **0.31 ns** |
| New | 2.11 ns | 18.58 ns | 25.57 ns | 2.82 ns | **1.88 ns** | 2.19 ns |
| NewFromBigInt | **3.11 ns** | 19.94 ns | 27.15 ns | — | — | — |
| NewFromFloat | 63.09 ns | 114.05 ns | 368.25 ns | 62.49 ns | **51.90 ns** | 70.20 ns |
| NewFromFloat32 | 182.85 ns | 174.00 ns | 181.65 ns | 54.23 ns | **39.55 ns** | 54.45 ns |
| NewFromFloatWithExponent | **146.75 ns** | 180.40 ns | 174.70 ns | — | — | — |
| NewFromFormattedString | **129.45 ns** | 248.50 ns | 269.30 ns | — | — | — |
| NewFromInt | **0.31 ns** | 12.89 ns | 26.14 ns | 2.82 ns | **0.31 ns** | 2.18 ns |
| NewFromInt32 | **0.31 ns** | 12.84 ns | 25.03 ns | 2.77 ns | **0.31 ns** | 2.18 ns |
| NewFromString | **6.78 ns** | 73.50 ns | 101.05 ns | 20.89 ns | 19.63 ns | 50.90 ns |
| NumDigits | 2.51 ns | **2.27 ns** | 6.14 ns | — | — | 3.74 ns |
| Pow | **17.18 ns** | 112.70 ns | 244.30 ns | 37.16 ns | — | 197.20 ns |
| QuoRem | **5.37 ns** | 35.62 ns | 131.05 ns | 14.79 ns | 11.98 ns | 9.04 ns |
| Rat | **118.20 ns** | 153.55 ns | — | — | — | — |
| Round | **2.06 ns** | 19.33 ns | 151.80 ns | 5.30 ns | 4.05 ns | 3.12 ns |
| RoundBank | **2.59 ns** | 19.51 ns | 456.10 ns | 5.30 ns | 4.36 ns | 2.83 ns |
| RoundCash | **14.53 ns** | 99.36 ns | 545.25 ns | — | — | — |
| RoundCeil | **2.05 ns** | 20.50 ns | 278.35 ns | — | 4.39 ns | 2.82 ns |
| RoundDown | 192.50 ns | **145.55 ns** | 12432.50 ns | 306.85 ns | 167.90 ns | 164.70 ns |
| RoundFloor | **2.00 ns** | 19.97 ns | 253.05 ns | — | 4.39 ns | 2.82 ns |
| RoundUp | 178.90 ns | **84.22 ns** | 8447.00 ns | 231.40 ns | — | 179.80 ns |
| Scan | **10.48 ns** | 72.32 ns | 114.35 ns | 21.84 ns | 20.05 ns | 50.57 ns |
| Shift | **2.19 ns** | 51.36 ns | 28.20 ns | — | — | — |
| Sign | **0.31 ns** | 0.47 ns | **0.31 ns** | 2.03 ns | **0.31 ns** | **0.31 ns** |
| String | **2.27 ns** | 23.14 ns | 112.45 ns | 39.05 ns | 27.16 ns | 25.56 ns |
| StringFixed | **17.96 ns** | 44.63 ns | 275.40 ns | 52.03 ns | 28.26 ns | 25.69 ns |
| StringFixedBank | **18.10 ns** | 45.56 ns | 584.10 ns | — | 30.45 ns | 25.65 ns |
| StringFixedCash | **29.83 ns** | 147.80 ns | 648.15 ns | — | — | — |
| Sub | **2.04 ns** | 19.47 ns | 32.89 ns | 6.61 ns | 3.13 ns | 6.44 ns |
| Sum | **4.88 ns** | 37.90 ns | 82.89 ns | 20.62 ns | 5.61 ns | 23.96 ns |
| Truncate | **2.04 ns** | 19.31 ns | 112.15 ns | 4.83 ns | 4.05 ns | 2.49 ns |
| UnmarshalBinary | 7.48 ns | 22.04 ns | 34.58 ns | 2.19 ns | **2.02 ns** | 37.83 ns |
| UnmarshalJSON | **106.40 ns** | 188.85 ns | 240.35 ns | 128.55 ns | 127.30 ns | 147.10 ns |
| UnmarshalText | **10.61 ns** | 74.55 ns | 99.06 ns | 17.77 ns | 15.89 ns | 37.83 ns |
| Value | 40.73 ns | **38.79 ns** | 125.55 ns | 53.80 ns | 43.94 ns | 40.38 ns |

Notes:
- sub-nanosecond entries are fully inlined and partially dead-code-eliminated by the benchmark loop; treat them as "free".
- `NewFromFloat32` deliberately uses shopspring's exact shortest-representation algorithm (which differs from strconv's in rare 1-ulp cases), trading speed for digit parity.
- `UnmarshalBinary`/`GobDecode` pay for decoding shopspring's wire format, which udecimal's own format-specific decoder doesn't.
- requires Go 1.26+ (the zerodecimal engine's floor).
- the `zerodecimal` column is the raw fallback engine used standalone, on the same fallback-range inputs as the other competitor columns. Against it, alpacadecimal's optimized path is **-33% geomean** (the int64 representation is the point of this library), while the fallback path costs **+181% geomean** — the price of boxing the engine value behind a pointer (one 24 B allocation per result) plus shopspring semantics (`DivisionPrecision` rounding, shopspring wire formats). If your values routinely exceed the fixed range and you don't need shopspring compatibility, raw zerodecimal is the better tool; if 99% fit (the Alpaca case), alpacadecimal is faster where it counts.
- zerodecimal's `MarshalBinary`/`UnmarshalBinary` use its own compact wire format (like udecimal's), not shopspring's; its StringFixedBank row is emulated as `RoundBank(2).StringFixed(2)`.
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
