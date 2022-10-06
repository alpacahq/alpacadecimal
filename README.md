# alpacadecimal
Similar and compatible with decimal.Decimal, but optimized for Alpaca's data sets.

### Goal
- optimize for Alpaca data sets.
- compatible with `decimal.Decimal` so that it could be a drop-in replacement for current `decimal.Decimal` usage.

### Design Doc

https://alpaca.atlassian.net/wiki/spaces/ENG/pages/1752891395/Decimal+golang+package+for+Alpaca+Data+Sets

### Benchmark

```
$ make bench
go test -bench=. --cpuprofile profile.out --memprofile memprofile.out
goos: darwin
goarch: amd64
pkg: github.com/alpacahq/alpacadecimal
cpu: Intel(R) Core(TM) i9-9880H CPU @ 2.30GHz
BenchmarkValue/AlpacaDecimal_Cached_Case-16             355753014                3.382 ns/op
BenchmarkValue/AlpacaDecimal_Optimized_Case-16          17469913                67.09 ns/op
BenchmarkValue/AlpacaDecimal_Fallback_Case-16            5666762               203.0 ns/op
BenchmarkValue/decimal.Decimal-16                        6021271               201.3 ns/op
BenchmarkValue/eric.Decimal-16                           5783232               173.9 ns/op
BenchmarkAdd/AlpacaDecimal-16                           523703056                2.221 ns/op
BenchmarkAdd/decimal.Decimal-16                         15251074                74.35 ns/op
BenchmarkAdd/eric.Decimal-16                            24815637                43.63 ns/op
BenchmarkScan/AlpacaDecimal-16                          77938342                15.11 ns/op
BenchmarkScan/decimal.Decimal-16                         5867258               194.6 ns/op
BenchmarkScan/eric.Decimal-16                            5484034               188.7 ns/op
BenchmarkMul/AlpacaDecimal-16                           162763900                7.288 ns/op
BenchmarkMul/decimal.Decimal-16                         14980779                71.57 ns/op
BenchmarkMul/eric.Decimal-16                            43237760                26.84 ns/op
PASS
ok      github.com/alpacahq/alpacadecimal       22.036s
```
