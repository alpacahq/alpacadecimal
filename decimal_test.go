package alpacadecimal_test

import (
	"database/sql/driver"
	"testing"

	"github.com/alpacahq/alpacadecimal"
	"github.com/shopspring/decimal"
)

func BenchmarkDecimal(b *testing.B) {
	d := decimal.NewFromInt(123)

	var result driver.Value

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		result, _ = d.Value()
	}
	_ = result
}

func BenchmarkAlpacaDecimalBestCase(b *testing.B) {
	d := alpacadecimal.NewFromInt(123)

	var result driver.Value

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		result, _ = d.Value()
	}
	_ = result
}

func BenchmarkAlpacaDecimalBetterCase(b *testing.B) {
	d := alpacadecimal.NewFromInt(12346)

	var result driver.Value

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		result, _ = d.Value()
	}
	_ = result
}

func BenchmarkAlpacaDecimalRestCase(b *testing.B) {
	d := alpacadecimal.NewFromDecimal(decimal.NewFromInt(123))

	var result driver.Value

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		result, _ = d.Value()
	}
	_ = result

}

func BenchmarkAlpacaDecimalAdd(b *testing.B) {
	d1 := alpacadecimal.NewFromInt(1)
	d2 := alpacadecimal.NewFromInt(2)

	var result alpacadecimal.Decimal

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		result = d1.Add(d2)
	}
	_ = result
}

func BenchmarkDecimalAdd(b *testing.B) {
	d1 := decimal.NewFromInt(1)
	d2 := decimal.NewFromInt(2)

	var result decimal.Decimal

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		result = d1.Add(d2)
	}
	_ = result
}
