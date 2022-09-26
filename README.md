# alpacadecimal
Similar and compatible with decimal.Decimal, but optimized for Alpaca's data sets.

### Goal
- optimize for Alpaca data sets.
- compatible with `decimal.Decimal` so that it could be a drop-in replacement for current `decimal.Decimal` usage.

### Design Doc

https://alpaca.atlassian.net/wiki/spaces/ENG/pages/1752891395/Decimal+golang+package+for+Alpaca+Data+Sets

### Benchmark

```
$ go test -bench=. --cpuprofile profile.out -memprofile memprofile.out
goos: darwin
goarch: amd64
pkg: github.com/alpacahq/alpacadecimal
cpu: Intel(R) Core(TM) i9-9880H CPU @ 2.30GHz
BenchmarkDecimal-16                      5347078               193.4 ns/op
BenchmarkAlpacaDecimalBestCase-16       167308377                7.099 ns/op
BenchmarkAlpacaDecimalBetterCase-16     18409668                67.17 ns/op
BenchmarkAlpacaDecimalRestCase-16        5546788               204.7 ns/op
BenchmarkAlpacaDecimalAdd-16            519596881                2.295 ns/op
BenchmarkDecimalAdd-16                  15247545                74.09 ns/op
PASS
ok      github.com/alpacahq/alpacadecimal       9.373s
```
