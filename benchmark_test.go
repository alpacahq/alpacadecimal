package alpacadecimal_test

import (
	"database/sql/driver"
	"testing"

	ericdecimal "github.com/ericlagergren/decimal"
	ericpostgres "github.com/ericlagergren/decimal/sql/postgres"
	"github.com/shopspring/decimal"

	"github.com/alpacahq/alpacadecimal"
)

func BenchmarkValue(b *testing.B) {
	b.Run("alpacadecimal.Decimal Cached Case", func(b *testing.B) {
		d := alpacadecimal.NewFromInt(123)

		var result driver.Value

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})

	b.Run("alpacadecimal.Decimal Optimized Case", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234567.12)

		var result driver.Value

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})

	b.Run("alpacadecimal.Decimal Fallback Case", func(b *testing.B) {
		d := alpacadecimal.NewFromInt(123456789) // this larger than max supported optimized value.

		var result driver.Value

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})

	b.Run("decimal.Decimal", func(b *testing.B) {
		d := decimal.NewFromInt(123)

		var result driver.Value

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})

	b.Run("eric.Decimal", func(b *testing.B) {
		v := ericdecimal.New(123, 0)
		d := ericpostgres.Decimal{V: v}

		var result driver.Value

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})
}

func BenchmarkAdd(b *testing.B) {
	b.Run("alpacadecimal.Decimal", func(b *testing.B) {
		d1 := alpacadecimal.NewFromInt(1)
		d2 := alpacadecimal.NewFromInt(2)

		var result alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Add(d2)
		}
		_ = result
	})

	b.Run("decimal.Decimal", func(b *testing.B) {
		d1 := decimal.NewFromInt(1)
		d2 := decimal.NewFromInt(2)

		var result decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Add(d2)
		}
		_ = result
	})

	b.Run("eric.Decimal", func(b *testing.B) {
		d1 := ericdecimal.New(1, 0)
		d2 := ericdecimal.New(2, 0)

		result := ericdecimal.New(0, 0)

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = result.Add(d1, d2)
		}
		_ = result
	})
}

func BenchmarkSub(b *testing.B) {
	b.Run("alpacadecimal.Decimal", func(b *testing.B) {
		d1 := alpacadecimal.NewFromInt(1)
		d2 := alpacadecimal.NewFromInt(2)

		var result alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Sub(d2)
		}
		_ = result
	})

	b.Run("decimal.Decimal", func(b *testing.B) {
		d1 := decimal.NewFromInt(1)
		d2 := decimal.NewFromInt(2)

		var result decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Sub(d2)
		}
		_ = result
	})

	b.Run("eric.Decimal", func(b *testing.B) {
		d1 := ericdecimal.New(1, 0)
		d2 := ericdecimal.New(2, 0)

		result := ericdecimal.New(0, 0)

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = result.Sub(d1, d2)
		}
		_ = result
	})
}

func BenchmarkScan(b *testing.B) {
	source := any("12345.123456789")

	b.Run("alpacadecimal.Decimal", func(b *testing.B) {
		var err error
		for n := 0; n < b.N; n++ {
			var d alpacadecimal.Decimal
			err = d.Scan(source)
		}
		_ = err
	})

	b.Run("decimal.Decimal", func(b *testing.B) {
		var err error
		for n := 0; n < b.N; n++ {
			var d decimal.Decimal
			err = d.Scan(source)
		}
		_ = err
	})

	b.Run("eric.Decimal", func(b *testing.B) {
		var err error
		for n := 0; n < b.N; n++ {
			var d ericpostgres.Decimal
			err = d.Scan(source)
		}
		_ = err
	})
}

func BenchmarkMul(b *testing.B) {
	x := 1.23
	y := 2.0

	b.Run("alpacadecimal.Decimal", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x)
		d2 := alpacadecimal.NewFromFloat(y)

		var result alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Mul(d2)
		}
		_ = result
	})

	b.Run("decimal.Decimal", func(b *testing.B) {
		d1 := decimal.NewFromFloat(x)
		d2 := decimal.NewFromFloat(y)

		var result decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Mul(d2)
		}
		_ = result
	})

	b.Run("eric.Decimal", func(b *testing.B) {
		d1 := ericdecimal.New(123, 2)
		d2 := ericdecimal.New(2, 0)

		result := ericdecimal.New(0, 0)

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = result.Mul(d1, d2)
		}
		_ = result
	})
}

func BenchmarkDiv(b *testing.B) {
	x := 1.23
	y := 2.0

	b.Run("alpacadecimal.Decimal", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x)
		d2 := alpacadecimal.NewFromFloat(y)

		var result alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Div(d2)
		}
		_ = result
	})

	b.Run("decimal.Decimal", func(b *testing.B) {
		d1 := decimal.NewFromFloat(x)
		d2 := decimal.NewFromFloat(y)

		var result decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Div(d2)
		}
		_ = result
	})

	b.Run("eric.Decimal", func(b *testing.B) {
		d1 := ericdecimal.New(123, 2)
		d2 := ericdecimal.New(2, 0)

		result := ericdecimal.New(0, 0)

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = result.Quo(d1, d2)
		}
		_ = result
	})
}

func BenchmarkString(b *testing.B) {
	x := 1.23

	b.Run("alpacadecimal.Decimal", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x)

		var result string

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.String()
		}
		_ = result
	})

	b.Run("decimal.Decimal", func(b *testing.B) {
		d1 := decimal.NewFromFloat(x)

		var result string

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.String()
		}
		_ = result
	})

	b.Run("eric.Decimal", func(b *testing.B) {
		d1 := ericdecimal.New(123, 2)

		var result string

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.String()
		}
		_ = result
	})
}

func BenchmarkRound(b *testing.B) {
	x := 1.23456

	b.Run("alpacadecimal.Decimal", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x)

		var result alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Round(2)
		}
		_ = result
	})

	b.Run("decimal.Decimal", func(b *testing.B) {
		d1 := decimal.NewFromFloat(x)

		var result decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Round(2)
		}
		_ = result
	})

	b.Run("eric.Decimal", func(b *testing.B) {
		d1 := ericdecimal.New(123456, 5)

		var result *ericdecimal.Big

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = d1.Round(2)
		}
		_ = result
	})
}

func BenchmarkNewFromDecimal(b *testing.B) {
	b.Run("alpacadecimal.Decimal.NewFromDecimal", func(b *testing.B) {
		d := decimal.New(123, -12)

		var result alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromDecimal(d)
		}
		_ = result
	})

	b.Run("alpacadecimal.Decimal.RequireFromString", func(b *testing.B) {
		d := decimal.New(123, -12)

		var result alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.RequireFromString(d.String())
		}
		_ = result
	})

	b.Run("alpacadecimal.Decimal.New", func(b *testing.B) {
		var result alpacadecimal.Decimal
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.New(123, -12)
		}
		_ = result
	})
}

func BenchmarkRoundUp(b *testing.B) {
	x1 := 1.23456789
	x2 := 123456.123456789
	x3 := 9000000.0
	// fallback case: slower than shopspring/decimal as overhead is added
	x4 := 9000000.1

	b.Run("alpacadecimal.Decimal", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x1)
		d2 := alpacadecimal.NewFromFloat(x2)
		d3 := alpacadecimal.NewFromFloat(x3)

		var result1, result2, result3 alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result1 = d1.RoundUp(i)
				result2 = d2.RoundUp(i)
				result3 = d3.RoundUp(i)
			}
		}
		_ = result1
		_ = result2
		_ = result3
	})

	b.Run("alpacadecimal.Decimal with fallback", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x1)
		d2 := alpacadecimal.NewFromFloat(x2)
		d3 := alpacadecimal.NewFromFloat(x3)
		d4 := alpacadecimal.NewFromFloat(x4)

		var result1, result2, result3, result4 alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result1 = d1.RoundUp(i)
				result2 = d2.RoundUp(i)
				result3 = d3.RoundUp(i)
				result4 = d4.RoundUp(i)
			}
		}
		_ = result1
		_ = result2
		_ = result3
		_ = result4
	})

	b.Run("alpacadecimal.Decimal fallback only", func(b *testing.B) {
		d4 := alpacadecimal.NewFromFloat(x4)

		var result4 alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result4 = d4.RoundUp(i)
			}
		}
		_ = result4
	})

	b.Run("decimal.Decimal", func(b *testing.B) {
		d1 := decimal.NewFromFloat(x1)
		d2 := decimal.NewFromFloat(x2)
		d3 := decimal.NewFromFloat(x3)

		var result1, result2, result3 decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result1 = d1.RoundUp(i)
				result2 = d2.RoundUp(i)
				result3 = d3.RoundUp(i)
			}
		}
		_ = result1
		_ = result2
		_ = result3
	})

	b.Run("decimal.Decimal with fallback", func(b *testing.B) {
		d1 := decimal.NewFromFloat(x1)
		d2 := decimal.NewFromFloat(x2)
		d3 := decimal.NewFromFloat(x3)
		d4 := decimal.NewFromFloat(x4)

		var result1, result2, result3, result4 decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result1 = d1.RoundUp(i)
				result2 = d2.RoundUp(i)
				result3 = d3.RoundUp(i)
				result4 = d4.RoundUp(i)
			}
		}
		_ = result1
		_ = result2
		_ = result3
		_ = result4
	})

	b.Run("decimal.Decimal fallback only", func(b *testing.B) {
		d4 := decimal.NewFromFloat(x4)

		var result4 decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result4 = d4.RoundUp(i)
			}
		}
		_ = result4
	})
}

func BenchmarkRoundDown(b *testing.B) {
	x1 := 1.23456789
	x2 := 123456.123456789
	x3 := 1234567.123456789
	// fallback case: slower than shopspring/decimal as overhead is added
	x4 := 9999999.0

	b.Run("alpacadecimal.Decimal", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x1)
		d2 := alpacadecimal.NewFromFloat(x2)
		d3 := alpacadecimal.NewFromFloat(x3)

		var result1, result2, result3 alpacadecimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result1 = d1.RoundDown(i)
				result2 = d2.RoundDown(i)
				result3 = d3.RoundDown(i)
			}
		}
		_ = result1
		_ = result2
		_ = result3
	})

	b.Run("alpacadecimal.Decimal with fallback", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x1)
		d2 := alpacadecimal.NewFromFloat(x2)
		d3 := alpacadecimal.NewFromFloat(x3)
		d4 := alpacadecimal.NewFromFloat(x4)
		var result1, result2, result3, result4 alpacadecimal.Decimal
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result1 = d1.RoundDown(i)
				result2 = d2.RoundDown(i)
				result3 = d3.RoundDown(i)
				result4 = d4.RoundDown(i)
			}
		}
		_ = result1
		_ = result2
		_ = result3
		_ = result4
	})

	b.Run("alpacadecimal.Decimal fallback only", func(b *testing.B) {
		d4 := alpacadecimal.NewFromFloat(x4)
		var result4 alpacadecimal.Decimal
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result4 = d4.RoundDown(i)
			}
		}
		_ = result4
	})

	b.Run("decimal.Decimal", func(b *testing.B) {
		d1 := decimal.NewFromFloat(x1)
		d2 := decimal.NewFromFloat(x2)
		d3 := decimal.NewFromFloat(x3)

		var result1, result2, result3 decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result1 = d1.RoundDown(i)
				result2 = d2.RoundDown(i)
				result3 = d3.RoundDown(i)
			}
		}
		_ = result1
		_ = result2
		_ = result3
	})

	b.Run("decimal.Decimal with fallback", func(b *testing.B) {
		d1 := decimal.NewFromFloat(x1)
		d2 := decimal.NewFromFloat(x2)
		d3 := decimal.NewFromFloat(x3)
		d4 := decimal.NewFromFloat(x4)

		var result1, result2, result3, result4 decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result1 = d1.RoundDown(i)
				result2 = d2.RoundDown(i)
				result3 = d3.RoundDown(i)
				result4 = d4.RoundDown(i)
			}
		}
		_ = result1
		_ = result2
		_ = result3
		_ = result4
	})

	b.Run("decimal.Decimal fallback only", func(b *testing.B) {
		d4 := decimal.NewFromFloat(x4)

		var result4 decimal.Decimal

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result4 = d4.RoundDown(i)
			}
		}
		_ = result4
	})
}
