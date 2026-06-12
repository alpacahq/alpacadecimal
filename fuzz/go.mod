module github.com/alpacahq/alpacadecimal/fuzz

go 1.26

require (
	github.com/AlexandrosKyriakakis/zerodecimal v0.0.0-20260612094624-487dcb7164a5
	github.com/alpacahq/alpacadecimal v0.0.0
	github.com/shopspring/decimal v1.4.0
)

replace github.com/alpacahq/alpacadecimal => ../
