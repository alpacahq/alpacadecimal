module github.com/alpacahq/alpacadecimal/fuzz

go 1.23

toolchain go1.24.5

require (
	github.com/alpacahq/alpacadecimal v0.0.0
	github.com/shopspring/decimal v1.4.0
)

require github.com/quagmt/udecimal v1.9.0 // indirect

replace github.com/alpacahq/alpacadecimal => ../
