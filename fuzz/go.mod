module github.com/alpacahq/alpacadecimal/fuzz

go 1.23

require (
	github.com/alpacahq/alpacadecimal v0.0.0
	github.com/shopspring/decimal v1.4.0
)

require github.com/quagmt/udecimal v1.10.0

replace github.com/alpacahq/alpacadecimal => ../
