package alpacadecimal_test

import (
	"database/sql/driver"
	"testing"

	"github.com/alpacahq/alpacadecimal"
	"github.com/shopspring/decimal"
)

func BenchmarkValue(b *testing.B) {
	b.Run("AlpacaDecimal Cached Case", func(b *testing.B) {
		d := alpacadecimal.NewFromInt(123)

		var result driver.Value

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})

	b.Run("AlpacaDecimal Optimized Case", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234567.12)

		var result driver.Value

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})

	b.Run("AlpacaDecimal Fallback Case", func(b *testing.B) {
		d := alpacadecimal.NewFromDecimal(decimal.NewFromFloat(123.123))

		var result driver.Value

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})

	b.Run("Decimal", func(b *testing.B) {
		d := decimal.NewFromInt(123)

		var result driver.Value

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})
}

func BenchmarkAdd(b *testing.B) {
	b.Run("AlpacaDecimal", func(b *testing.B) {
		d1 := alpacadecimal.NewFromInt(1)
		d2 := alpacadecimal.NewFromInt(2)

		var result alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Add(d2)
		}
		_ = result
	})

	b.Run("Decimal", func(b *testing.B) {
		d1 := decimal.NewFromInt(1)
		d2 := decimal.NewFromInt(2)

		var result decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Add(d2)
		}
		_ = result

	})
}

func BenchmarkScan(b *testing.B) {
	source := any("12345.123456789")

	b.Run("AlpacaDecimal", func(b *testing.B) {
		var err error
		for n := 0; n < b.N; n++ {
			var d alpacadecimal.Decimal
			err = d.Scan(source)
		}
		_ = err
	})

	b.Run("Decimal", func(b *testing.B) {
		var err error
		for n := 0; n < b.N; n++ {
			var d decimal.Decimal
			err = d.Scan(source)
		}
		_ = err
	})
}

func BenchmarkMul(b *testing.B) {
	x := 1.23
	y := 2.0

	b.Run("AlpacaDecimal", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x)
		d2 := alpacadecimal.NewFromFloat(y)

		var result alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Mul(d2)
		}
		_ = result
	})

	b.Run("Decimal", func(b *testing.B) {
		d1 := decimal.NewFromFloat(x)
		d2 := decimal.NewFromFloat(y)

		var result decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Mul(d2)
		}
		_ = result

	})
}
