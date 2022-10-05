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
BenchmarkValue/AlpacaDecimal_Cached_Case-16             358316202                3.337 ns/op
BenchmarkValue/AlpacaDecimal_Optimized_Case-16          15720096                65.90 ns/op
BenchmarkValue/AlpacaDecimal_Fallback_Case-16            5470454               193.6 ns/op
BenchmarkValue/Decimal-16                                6017497               184.5 ns/op
BenchmarkAdd/AlpacaDecimal-16                           511924670                2.162 ns/op
BenchmarkAdd/Decimal-16                                 17032792                72.49 ns/op
BenchmarkScan/AlpacaDecimal-16                          69441972                15.21 ns/op
BenchmarkScan/Decimal-16                                 5307663               193.9 ns/op
BenchmarkMul/AlpacaDecimal-16                           166523916                7.268 ns/op
BenchmarkMul/Decimal-16                                 14445711                74.96 ns/op
PASS
ok      github.com/alpacahq/alpacadecimal       14.879s
```
