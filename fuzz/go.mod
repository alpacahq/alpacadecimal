module github.com/alpacahq/alpacadecimal/fuzz

go 1.26

require (
	github.com/AlexandrosKyriakakis/zerodecimal v0.0.0-00010101000000-000000000000
	github.com/alpacahq/alpacadecimal v0.0.0
	github.com/shopspring/decimal v1.4.0
)

replace github.com/alpacahq/alpacadecimal => ../

replace github.com/AlexandrosKyriakakis/zerodecimal => /Users/alexandros_kyriakakis/go/src/github.com/AlexandrosKyriakakis/decimal-workspace/zero-decimal
