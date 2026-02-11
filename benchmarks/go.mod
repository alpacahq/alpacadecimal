module github.com/alpacahq/alpacadecimal/benchmarks

go 1.23

toolchain go1.24.5

require (
	github.com/alpacahq/alpacadecimal v0.0.8
	github.com/ericlagergren/decimal v0.0.0-20240411145413-00de7ca16731
	github.com/quagmt/udecimal v1.9.0
	github.com/shopspring/decimal v1.4.0
)

replace github.com/alpacahq/alpacadecimal => ../
