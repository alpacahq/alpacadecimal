# alpacadecimal
Similar and compatible with decimal.Decimal, but optimized for Alpaca's data sets.

### Goal
- optimize for Alpaca data sets.
- compatible with `decimal.Decimal` so that it could be a drop-in replacement for current `decimal.Decimal` usage.

### Design Doc

https://alpaca.atlassian.net/wiki/spaces/ENG/pages/1752891395/Decimal+golang+package+for+Alpaca+Data+Sets

### Key Ideas

The original `decimal.Decimal` package has bottleneck on `big.Int` operations, e.g. sql serialization / deserialization, addition, multiplication etc. These operations took fair amount cpu and memory during
our profiling / monitoring.

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

We pick 12 precisions because it could cover 99% of Alpaca common cases as indicated in design doc.


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
goarch: amd64
pkg: github.com/alpacahq/alpacadecimal
cpu: Intel(R) Core(TM) i9-9880H CPU @ 2.30GHz
BenchmarkValue/alpacadecimal.Decimal_Cached_Case-16             336750202                3.687 ns/op
BenchmarkValue/alpacadecimal.Decimal_Optimized_Case-16          14973528                74.76 ns/op
BenchmarkValue/alpacadecimal.Decimal_Fallback_Case-16            5258898               232.7 ns/op
BenchmarkValue/decimal.Decimal-16                                5769552               202.2 ns/op
BenchmarkValue/eric.Decimal-16                                   6294133               182.0 ns/op

BenchmarkAdd/alpacadecimal.Decimal-16                           514034338                2.362 ns/op
BenchmarkAdd/decimal.Decimal-16                                 15149514                74.97 ns/op
BenchmarkAdd/eric.Decimal-16                                    24288189                44.76 ns/op

BenchmarkSub/alpacadecimal.Decimal-16                           242750554                5.179 ns/op
BenchmarkSub/decimal.Decimal-16                                 19062336                60.58 ns/op
BenchmarkSub/eric.Decimal-16                                    24667969                46.34 ns/op

BenchmarkScan/alpacadecimal.Decimal-16                          77829289                16.21 ns/op
BenchmarkScan/decimal.Decimal-16                                 5272404               203.6 ns/op
BenchmarkScan/eric.Decimal-16                                    6435286               182.5 ns/op

BenchmarkMul/alpacadecimal.Decimal-16                           151763162                7.943 ns/op
BenchmarkMul/decimal.Decimal-16                                 15458967                68.01 ns/op
BenchmarkMul/eric.Decimal-16                                    37150664                27.99 ns/op

BenchmarkDiv/alpacadecimal.Decimal-16                           138624043                8.418 ns/op
BenchmarkDiv/decimal.Decimal-16                                  4009129               285.0 ns/op
BenchmarkDiv/eric.Decimal-16                                    35730601                32.89 ns/op

BenchmarkString/alpacadecimal.Decimal-16                        355172980                3.346 ns/op
BenchmarkString/decimal.Decimal-16                               6924613               165.4 ns/op
BenchmarkString/eric.Decimal-16                                  6880526               170.6 ns/op
PASS
ok      github.com/alpacahq/alpacadecimal       35.016s
```
