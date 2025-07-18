# alpacadecimal
Similar and compatible with [decimal.Decimal](https://pkg.go.dev/github.com/shopspring/decimal), but optimized for Alpaca's data sets.

### Goal
- optimize for Alpaca data sets (99% of decimals are within 10 millions with up to 12 precisions).
- compatible with [decimal.Decimal](https://pkg.go.dev/github.com/shopspring/decimal) so that it could be a drop-in replacement for current `decimal.Decimal` usage.

### Key Ideas

The original `decimal.Decimal` package has bottleneck on `big.Int` operations, e.g. sql serialization / deserialization, addition, multiplication etc. These operations took fair amount cpu and memory during
our profiling / monitoring.

![profiling result](doc/value-slowness.png)

The optimization this library is to represent most decimal numbers with `int64` instead of `big.Int`. To 
keep this library to be compatible with original `decimal.Decimal` package, we use original as a fallback
solution when `int64` is not enough (e.g. number is too big / small, too many precisions).

The core data struct is like following:

```golang
type Decimal struct {
	// represent decimal with 12 precision, 1.23 will have `fixed = 1_230_000_000_000`
	// max support decimal is 9_223_372.000_000_000_000
	// min support decimal is -9_223_372.000_000_000_000
	fixed int64

	// fallback to original decimal.Decimal if necessary
	fallback *decimal.Decimal
}
```

We pick 12 precisions because it could cover 99% of Alpaca common cases.


### Compatibility

In general, `alpacadecimal.Decimal` is fully compatible with `decimal.Decimal` package, as `decimal.Decimal` is used as 
a fallback solution for overflow cases.

There are a few special cases / APIs that `alpacadecimal.Decimal` behaves different from `decimal.Decimal` (behaviour is still
correct / valid, just different). Affected APIs:

- `Decimal.Exponent()`
- `Decimal.Coefficient()`
- `Decimal.CoefficientInt64()` 
- `Decimal.NumDigits()`

For optimized case, `alpacadecimal.Decimal` always assume that exponent is 12, which results in a valid but different decimal representation. For example,

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

### Related Issues
- `big.NewInt` optimization from [here](https://go-review.googlesource.com/c/go/+/411254) might help to speed up some `big.Int` related operations.
- `big.Int.String` slowness is tracked by [this issue](https://github.com/golang/go/issues/20906). The approach we reduce this slowness is to use int64 to represent the number if possible to avoid `big.Int` operations.

### Benchmark

Generally, for general case (99%), the speedup varies from 5x to 100x.

```
$ make bench
go test -bench=. --cpuprofile profile.out --memprofile memprofile.out
goos: darwin
goarch: arm64
pkg: github.com/alpacahq/alpacadecimal
cpu: Apple M3
BenchmarkValue/alpacadecimal.Decimal_Cached_Case-8              579633375                2.084 ns/op
BenchmarkValue/alpacadecimal.Decimal_Optimized_Case-8           33500136                35.10 ns/op
BenchmarkValue/alpacadecimal.Decimal_Fallback_Case-8            12971452                91.12 ns/op
BenchmarkValue/decimal.Decimal-8                                14983346                80.26 ns/op
BenchmarkValue/eric.Decimal-8                                   13220779                93.22 ns/op
BenchmarkAdd/alpacadecimal.Decimal-8                            863144540                1.385 ns/op
BenchmarkAdd/decimal.Decimal-8                                  34509368                35.58 ns/op
BenchmarkAdd/eric.Decimal-8                                     69539348                17.16 ns/op
BenchmarkSub/alpacadecimal.Decimal-8                            501099547                2.394 ns/op
BenchmarkSub/decimal.Decimal-8                                  40411579                28.76 ns/op
BenchmarkSub/eric.Decimal-8                                     69800077                17.12 ns/op
BenchmarkScan/alpacadecimal.Decimal-8                           122420659               10.02 ns/op
BenchmarkScan/decimal.Decimal-8                                 12091557                99.72 ns/op
BenchmarkScan/eric.Decimal-8                                    12087218                96.06 ns/op
BenchmarkMul/alpacadecimal.Decimal-8                            323009985                3.722 ns/op
BenchmarkMul/decimal.Decimal-8                                  33682357                34.52 ns/op
BenchmarkMul/eric.Decimal-8                                     91006764                12.58 ns/op
BenchmarkDiv/alpacadecimal.Decimal-8                            266056830                4.517 ns/op
BenchmarkDiv/decimal.Decimal-8                                   8536772               139.9 ns/op
BenchmarkDiv/eric.Decimal-8                                     83571278                14.34 ns/op
BenchmarkString/alpacadecimal.Decimal-8                         613455636                1.958 ns/op
BenchmarkString/decimal.Decimal-8                               15399207                77.92 ns/op
BenchmarkString/eric.Decimal-8                                  14207025                82.22 ns/op
BenchmarkRound/alpacadecimal.Decimal-8                          800181822                1.498 ns/op
BenchmarkRound/decimal.Decimal-8                                10937922               109.2 ns/op
BenchmarkRound/eric.Decimal-8                                   140659539                8.512 ns/op
BenchmarkNewFromDecimal/alpacadecimal.Decimal.NewFromDecimal-8          100000000               11.68 ns/op
BenchmarkNewFromDecimal/alpacadecimal.Decimal.RequireFromString-8       11680768               103.5 ns/op
BenchmarkNewFromDecimal/alpacadecimal.Decimal.New-8                     645217516                1.865 ns/op
PASS
ok      github.com/alpacahq/alpacadecimal       40.632s
```
