//go:build go1.23

package benchmarks

import (
	"database/sql/driver"
	"encoding/json"
	"math/big"
	"regexp"
	"testing"

	"github.com/quagmt/udecimal"
	"github.com/shopspring/decimal"

	"github.com/alpacahq/alpacadecimal"
)

// ---------------------------------------------------------------------------
// Test values
// ---------------------------------------------------------------------------
//
// Optimized path: integer part < ~9,223,372 AND ≤12 fractional digits.
// Fallback path:  integer part ≥ ~9,223,372 OR >12 fractional digits.
//
// optSmall:  small value, often cached               "1.23"
// optLarge:  near upper end of optimized range        "1234567.123456789"
// fbInt:     integer part exceeds optimized maxInt    "123456789.123"
// fbPrec:    >12 fractional digits                   "1.0000000000001"

// ---------------------------------------------------------------------------
// Constructors
// ---------------------------------------------------------------------------

func BenchmarkNew(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.New(12345, -2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.New(123456789123, -3)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		var result decimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = decimal.New(123456789123, -3)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		var result udecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = udecimal.MustFromInt64(123456789123, 3)
		}
		_ = result
	})
}

func BenchmarkNewFromString(b *testing.B) {
	b.Run("alpacadecimal/optimized_small", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = alpacadecimal.NewFromString("1.23")
		}
		_ = result
	})

	b.Run("alpacadecimal/optimized_large", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = alpacadecimal.NewFromString("1234567.123456789")
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback_bigint", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = alpacadecimal.NewFromString("123456789.123456789")
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback_highprec", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = alpacadecimal.NewFromString("1.0000000000001")
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		var result decimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = decimal.NewFromString("123456789.123456789")
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		var result udecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = udecimal.Parse("123456789.123456789")
		}
		_ = result
	})
}

func BenchmarkNewFromFloat(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromFloat(1234567.12)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromFloat(123456789.123)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		var result decimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = decimal.NewFromFloat(123456789.123)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		var result udecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = udecimal.NewFromFloat64(123456789.123)
		}
		_ = result
	})
}

func BenchmarkNewFromFloat32(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromFloat32(1234.56)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromFloat32(123456789.0)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		var result decimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = decimal.NewFromFloat32(123456789.0)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		var result udecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = udecimal.NewFromFloat64(float64(float32(123456789.0)))
		}
		_ = result
	})
}

func BenchmarkNewFromFloatWithExponent(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromFloatWithExponent(1234.56, -2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromFloatWithExponent(123456789.123, -3)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		var result decimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = decimal.NewFromFloatWithExponent(123456789.123, -3)
		}
		_ = result
	})
}

func BenchmarkNewFromInt(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromInt(12345)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromInt(123456789)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		var result decimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = decimal.NewFromInt(123456789)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		var result udecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = udecimal.MustFromInt64(123456789, 0)
		}
		_ = result
	})
}

func BenchmarkNewFromInt32(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromInt32(12345)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromInt32(123456789)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		var result decimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = decimal.NewFromInt32(123456789)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		var result udecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = udecimal.MustFromInt64(123456789, 0)
		}
		_ = result
	})
}

func BenchmarkNewFromBigInt(b *testing.B) {
	smallBI := big.NewInt(12345)
	largeBI := big.NewInt(123456789123)

	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromBigInt(smallBI, -2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.NewFromBigInt(largeBI, -3)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		var result decimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = decimal.NewFromBigInt(largeBI, -3)
		}
		_ = result
	})
}

func BenchmarkNewFromFormattedString(b *testing.B) {
	re := regexp.MustCompile(",")

	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = alpacadecimal.NewFromFormattedString("1,234.56", re)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		var result alpacadecimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = alpacadecimal.NewFromFormattedString("123,456,789.123", re)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		var result decimal.Decimal
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = decimal.NewFromFormattedString("123,456,789.123", re)
		}
		_ = result
	})
}

// ---------------------------------------------------------------------------
// Aggregate Functions
// ---------------------------------------------------------------------------

func BenchmarkSum(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1.23)
		d2 := alpacadecimal.NewFromFloat(4.56)
		d3 := alpacadecimal.NewFromFloat(7.89)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.Sum(d1, d2, d3)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("223456789.123")
		d3 := alpacadecimal.RequireFromString("323456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.Sum(d1, d2, d3)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("223456789.123")
		d3 := decimal.RequireFromString("323456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = decimal.Sum(d1, d2, d3)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("223456789.123")
		d3 := udecimal.MustParse("323456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			for _, item := range []udecimal.Decimal{d1, d2, d3} {
				result = result.Add(item)
			}
		}
		_ = result
	})
}

func BenchmarkMin(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1.23)
		d2 := alpacadecimal.NewFromFloat(4.56)
		d3 := alpacadecimal.NewFromFloat(7.89)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.Min(d1, d2, d3)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("223456789.123")
		d3 := alpacadecimal.RequireFromString("323456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.Min(d1, d2, d3)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("223456789.123")
		d3 := decimal.RequireFromString("323456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = decimal.Min(d1, d2, d3)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("223456789.123")
		d3 := udecimal.MustParse("323456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = udecimal.Min(d1, d2, d3)
		}
		_ = result
	})
}

func BenchmarkMax(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1.23)
		d2 := alpacadecimal.NewFromFloat(4.56)
		d3 := alpacadecimal.NewFromFloat(7.89)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.Max(d1, d2, d3)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("223456789.123")
		d3 := alpacadecimal.RequireFromString("323456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.Max(d1, d2, d3)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("223456789.123")
		d3 := decimal.RequireFromString("323456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = decimal.Max(d1, d2, d3)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("223456789.123")
		d3 := udecimal.MustParse("323456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = udecimal.Max(d1, d2, d3)
		}
		_ = result
	})
}

func BenchmarkAvg(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1.23)
		d2 := alpacadecimal.NewFromFloat(4.56)
		d3 := alpacadecimal.NewFromFloat(7.89)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.Avg(d1, d2, d3)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("223456789.123")
		d3 := alpacadecimal.RequireFromString("323456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = alpacadecimal.Avg(d1, d2, d3)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("223456789.123")
		d3 := decimal.RequireFromString("323456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = decimal.Avg(d1, d2, d3)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("223456789.123")
		d3 := udecimal.MustParse("323456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			for _, item := range []udecimal.Decimal{d1, d2, d3} {
				result = result.Add(item)
			}
			result, _ = result.Div(udecimal.MustFromInt64(3, 0))
		}
		_ = result
	})
}

// ---------------------------------------------------------------------------
// Arithmetic
// ---------------------------------------------------------------------------

func BenchmarkAdd(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1234.56)
		d2 := alpacadecimal.NewFromFloat(7890.12)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Add(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("223456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Add(d2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("223456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Add(d2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("223456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Add(d2)
		}
		_ = result
	})
}

func BenchmarkSub(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(7890.12)
		d2 := alpacadecimal.NewFromFloat(1234.56)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Sub(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("223456789.123")
		d2 := alpacadecimal.RequireFromString("123456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Sub(d2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("223456789.123")
		d2 := decimal.RequireFromString("123456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Sub(d2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("223456789.123")
		d2 := udecimal.MustParse("123456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Sub(d2)
		}
		_ = result
	})
}

func BenchmarkMul(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1.23)
		d2 := alpacadecimal.NewFromFloat(2.0)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Mul(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("3.14")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Mul(d2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("3.14")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Mul(d2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("3.14")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Mul(d2)
		}
		_ = result
	})
}

func BenchmarkDiv(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1.23)
		d2 := alpacadecimal.NewFromFloat(2.0)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Div(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("3.0")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Div(d2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("3.0")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Div(d2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("3.0")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d1.Div(d2)
		}
		_ = result
	})
}

func BenchmarkDivRound(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(123.456)
		d2 := alpacadecimal.NewFromFloat(7.0)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.DivRound(d2, 4)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("7.0")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.DivRound(d2, 4)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("7.0")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.DivRound(d2, 4)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("7.0")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d1.Div(d2)
			result = result.RoundHAZ(4)
		}
		_ = result
	})
}

func BenchmarkMod(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(123.456)
		d2 := alpacadecimal.NewFromFloat(7.0)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Mod(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("7.0")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Mod(d2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("7.0")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Mod(d2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("7.0")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d1.Mod(d2)
		}
		_ = result
	})
}

func BenchmarkPow(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.5)
		exp := alpacadecimal.NewFromInt(3)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Pow(exp)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		exp := alpacadecimal.NewFromInt(2)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Pow(exp)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		exp := decimal.NewFromInt(2)
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Pow(exp)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.PowInt32(2)
		}
		_ = result
	})
}

func BenchmarkQuoRem(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(123.456)
		d2 := alpacadecimal.NewFromFloat(7.0)
		var q, r alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			q, r = d1.QuoRem(d2, 4)
		}
		_ = q
		_ = r
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("7.0")
		var q, r alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			q, r = d1.QuoRem(d2, 4)
		}
		_ = q
		_ = r
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("7.0")
		var q, r decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			q, r = d1.QuoRem(d2, 4)
		}
		_ = q
		_ = r
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("7.0")
		var q, r udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			q, r, _ = d1.QuoRem(d2)
		}
		_ = q
		_ = r
	})
}

func BenchmarkShift(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Shift(2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Shift(2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Shift(2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Mul(udecimal.MustFromUint64(1, 2))
		}
		_ = result
	})
}

func BenchmarkNeg(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Neg()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Neg()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Neg()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Neg()
		}
		_ = result
	})
}

func BenchmarkAbs(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(-1234.56)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Abs()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("-123456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Abs()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("-123456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Abs()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("-123456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Abs()
		}
		_ = result
	})
}

// ---------------------------------------------------------------------------
// Comparison
// ---------------------------------------------------------------------------

func BenchmarkCmp(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1234.56)
		d2 := alpacadecimal.NewFromFloat(7890.124)
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Cmp(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("223456789.1234")
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Cmp(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/mixed", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1234.56)
		d2 := alpacadecimal.RequireFromString("123456789.1234")
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Cmp(d2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("22345678.1234")
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Cmp(d2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("22345678.1234")
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Cmp(d2)
		}
		_ = result
	})
}

func BenchmarkEqual(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1234.56)
		d2 := alpacadecimal.NewFromFloat(1234.56)
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Equal(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("123456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Equal(d2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("123456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Equal(d2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("123456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.Equal(d2)
		}
		_ = result
	})
}

func BenchmarkGreaterThan(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(7890.12)
		d2 := alpacadecimal.NewFromFloat(1234.56)
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.GreaterThan(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("223456789.123")
		d2 := alpacadecimal.RequireFromString("123456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.GreaterThan(d2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("223456789.123")
		d2 := decimal.RequireFromString("123456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.GreaterThan(d2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("223456789.123")
		d2 := udecimal.MustParse("123456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.GreaterThan(d2)
		}
		_ = result
	})
}

func BenchmarkGreaterThanOrEqual(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1234.56)
		d2 := alpacadecimal.NewFromFloat(1234.56)
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.GreaterThanOrEqual(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("123456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.GreaterThanOrEqual(d2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("123456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.GreaterThanOrEqual(d2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("223456789.123")
		d2 := udecimal.MustParse("123456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.GreaterThanOrEqual(d2)
		}
		_ = result
	})
}

func BenchmarkLessThan(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1234.56)
		d2 := alpacadecimal.NewFromFloat(7890.12)
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.LessThan(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("223456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.LessThan(d2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("223456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.LessThan(d2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("223456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.LessThan(d2)
		}
		_ = result
	})
}

func BenchmarkLessThanOrEqual(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(1234.56)
		d2 := alpacadecimal.NewFromFloat(1234.56)
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.LessThanOrEqual(d2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d1 := alpacadecimal.RequireFromString("123456789.123")
		d2 := alpacadecimal.RequireFromString("123456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.LessThanOrEqual(d2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.RequireFromString("123456789.123")
		d2 := decimal.RequireFromString("123456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.LessThanOrEqual(d2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustParse("123456789.123")
		d2 := udecimal.MustParse("223456789.1234")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.LessThanOrEqual(d2)
		}
		_ = result
	})
}

// ---------------------------------------------------------------------------
// Introspection
// ---------------------------------------------------------------------------

func BenchmarkSign(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(-1234.56)
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Sign()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("-123456789.123")
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Sign()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("-123456789.123")
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Sign()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("-123456789.123")
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Sign()
		}
		_ = result
	})
}

func BenchmarkIsZero(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsZero()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsZero()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsZero()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsZero()
		}
		_ = result
	})
}

func BenchmarkIsPositive(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsPositive()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsPositive()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsPositive()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsPos()
		}
		_ = result
	})
}

func BenchmarkIsNegative(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(-1234.56)
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsNegative()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("-123456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsNegative()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("-123456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsNegative()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("-123456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsNeg()
		}
		_ = result
	})
}

func BenchmarkIsInteger(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsInteger()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IsInteger()
		}
		_ = result
	})
}

func BenchmarkIntPart(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result int64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IntPart()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result int64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IntPart()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result int64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.IntPart()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result int64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.Int64()
		}
		_ = result
	})
}

func BenchmarkNumDigits(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.NumDigits()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.NumDigits()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.NumDigits()
		}
		_ = result
	})
}

func BenchmarkExponent(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result int32
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Exponent()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result int32
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Exponent()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result int32
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Exponent()
		}
		_ = result
	})
}

func BenchmarkCoefficient(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result *big.Int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Coefficient()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result *big.Int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Coefficient()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result *big.Int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Coefficient()
		}
		_ = result
	})
}

func BenchmarkCoefficientInt64(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result int64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.CoefficientInt64()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result int64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.CoefficientInt64()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result int64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.CoefficientInt64()
		}
		_ = result
	})
}

func BenchmarkFloat64(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result float64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.Float64()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result float64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.Float64()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result float64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.Float64()
		}
		_ = result
	})
}

func BenchmarkInexactFloat64(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result float64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.InexactFloat64()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result float64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.InexactFloat64()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result float64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.InexactFloat64()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result float64
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.InexactFloat64()
		}
		_ = result
	})
}

func BenchmarkBigInt(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result *big.Int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.BigInt()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result *big.Int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.BigInt()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result *big.Int
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.BigInt()
		}
		_ = result
	})
}

func BenchmarkBigFloat(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result *big.Float
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.BigFloat()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result *big.Float
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.BigFloat()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result *big.Float
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.BigFloat()
		}
		_ = result
	})
}

func BenchmarkRat(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result *big.Rat
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Rat()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result *big.Rat
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Rat()
		}
		_ = result
	})
}

func BenchmarkCopy(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Copy()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Copy()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Copy()
		}
		_ = result
	})
}

// ---------------------------------------------------------------------------
// Rounding
// ---------------------------------------------------------------------------

func BenchmarkRound(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23456)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Round(2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("10000000.123456")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Round(2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("10000000.123456")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Round(2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("10000000.123456")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundAwayFromZero(2)
		}
		_ = result
	})
}

func BenchmarkRoundBank(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23456)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundBank(2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("10000000.123456")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundBank(2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("10000000.123456")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundBank(2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("10000000.123456")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundBank(2)
		}
		_ = result
	})
}

func BenchmarkRoundCash(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23456)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundCash(5)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("10000000.123456")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundCash(5)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("10000000.123456")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundCash(5)
		}
		_ = result
	})
}

func BenchmarkTruncate(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23456)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Truncate(2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("10000000.123456")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Truncate(2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("10000000.123456")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Truncate(2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("10000000.123456")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Trunc(2)
		}
		_ = result
	})
}

func BenchmarkCeil(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Ceil()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Ceil()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Ceil()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Ceil()
		}
		_ = result
	})
}

func BenchmarkFloor(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Floor()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Floor()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Floor()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.Floor()
		}
		_ = result
	})
}

func BenchmarkRoundUp(b *testing.B) {
	x1 := 1.23456789
	x2 := 123456.123456789
	x3 := 9000000.0
	// fallback case: RoundUp threshold exceeded (fixed > 9e18)
	x4 := 9000000.1

	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x1)
		d2 := alpacadecimal.NewFromFloat(x2)
		d3 := alpacadecimal.NewFromFloat(x3)
		var result1, result2, result3 alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
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

	b.Run("alpacadecimal/with_fallback", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x1)
		d2 := alpacadecimal.NewFromFloat(x2)
		d3 := alpacadecimal.NewFromFloat(x3)
		d4 := alpacadecimal.NewFromFloat(x4)
		var result1, result2, result3, result4 alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
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

	b.Run("alpacadecimal/fallback_only", func(b *testing.B) {
		d4 := alpacadecimal.NewFromFloat(x4)
		var result4 alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result4 = d4.RoundUp(i)
			}
		}
		_ = result4
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.NewFromFloat(x1)
		d2 := decimal.NewFromFloat(x2)
		d3 := decimal.NewFromFloat(x3)
		var result1, result2, result3 decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
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

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustFromFloat64(x1)
		d2 := udecimal.MustFromFloat64(x2)
		d3 := udecimal.MustFromFloat64(x3)
		d4 := udecimal.MustFromFloat64(x4)
		var result1, result2, result3, result4 udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result1 = d1.RoundHAZ(uint8(i))
				result2 = d2.RoundHAZ(uint8(i))
				result3 = d3.RoundHAZ(uint8(i))
				result4 = d4.RoundHAZ(uint8(i))
			}
		}
		_ = result1
		_ = result2
		_ = result3
		_ = result4
	})
}

func BenchmarkRoundDown(b *testing.B) {
	x1 := 1.23456789
	x2 := 123456.123456789
	x3 := 1234567.123456789
	// fallback case: RoundDown threshold exceeded
	x4 := 9999999.0

	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x1)
		d2 := alpacadecimal.NewFromFloat(x2)
		d3 := alpacadecimal.NewFromFloat(x3)
		var result1, result2, result3 alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
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

	b.Run("alpacadecimal/with_fallback", func(b *testing.B) {
		d1 := alpacadecimal.NewFromFloat(x1)
		d2 := alpacadecimal.NewFromFloat(x2)
		d3 := alpacadecimal.NewFromFloat(x3)
		d4 := alpacadecimal.NewFromFloat(x4)
		var result1, result2, result3, result4 alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
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

	b.Run("alpacadecimal/fallback_only", func(b *testing.B) {
		d4 := alpacadecimal.NewFromFloat(x4)
		var result4 alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result4 = d4.RoundDown(i)
			}
		}
		_ = result4
	})

	b.Run("shopspring", func(b *testing.B) {
		d1 := decimal.NewFromFloat(x1)
		d2 := decimal.NewFromFloat(x2)
		d3 := decimal.NewFromFloat(x3)
		var result1, result2, result3 decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
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

	b.Run("udecimal", func(b *testing.B) {
		d1 := udecimal.MustFromFloat64(x1)
		d2 := udecimal.MustFromFloat64(x2)
		d3 := udecimal.MustFromFloat64(x3)
		d4 := udecimal.MustFromFloat64(x4)
		var result1, result2, result3, result4 udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			for i := int32(-6); i <= 12; i++ {
				result1 = d1.RoundHTZ(uint8(i))
				result2 = d2.RoundHTZ(uint8(i))
				result3 = d3.RoundHTZ(uint8(i))
				result4 = d4.RoundHTZ(uint8(i))
			}
		}
		_ = result1
		_ = result2
		_ = result3
		_ = result4
	})
}

func BenchmarkRoundCeil(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23456)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundCeil(2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("10000000.123456")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundCeil(2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.NewFromFloat(1.23456)
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundCeil(2)
		}
		_ = result
	})
}

func BenchmarkRoundFloor(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23456)
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundFloor(2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("10000000.123456")
		var result alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundFloor(2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.NewFromFloat(1.23456)
		var result decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.RoundFloor(2)
		}
		_ = result
	})
}

// ---------------------------------------------------------------------------
// String / Formatting
// ---------------------------------------------------------------------------

func BenchmarkString(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23)
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.String()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.String()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.String()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.String()
		}
		_ = result
	})
}

func BenchmarkStringFixed(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23456)
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.StringFixed(2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("10000000.123456")
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.StringFixed(2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("10000000.123456")
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.StringFixed(2)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("10000000.123456")
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.StringFixed(2)
		}
		_ = result
	})
}

func BenchmarkStringFixedBank(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23456)
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.StringFixedBank(2)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("10000000.123456")
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.StringFixedBank(2)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("10000000.123456")
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.StringFixedBank(2)
		}
		_ = result
	})
}

func BenchmarkStringFixedCash(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1.23456)
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.StringFixedCash(5)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("10000000.123456")
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.StringFixedCash(5)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("10000000.123456")
		var result string
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d.StringFixedCash(5)
		}
		_ = result
	})
}

// ---------------------------------------------------------------------------
// SQL / Serialization
// ---------------------------------------------------------------------------

func BenchmarkValue(b *testing.B) {
	b.Run("alpacadecimal/cached", func(b *testing.B) {
		d := alpacadecimal.NewFromInt(123)
		var result driver.Value
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})

	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234567.12)
		var result driver.Value
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result driver.Value
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result driver.Value
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result driver.Value
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.Value()
		}
		_ = result
	})
}

func BenchmarkScan(b *testing.B) {
	small := any("123.45")
	large := any("12345678.123456789")

	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		var err error
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d alpacadecimal.Decimal
			err = d.Scan(small)
		}
		_ = err
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		var err error
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d alpacadecimal.Decimal
			err = d.Scan(large)
		}
		_ = err
	})

	b.Run("shopspring", func(b *testing.B) {
		var err error
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d decimal.Decimal
			err = d.Scan(small)
		}
		_ = err
	})

	b.Run("udecimal", func(b *testing.B) {
		var err error
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d udecimal.Decimal
			err = d.Scan(small)
		}
		_ = err
	})
}

func BenchmarkMarshalJSON(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = json.Marshal(d)
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = json.Marshal(d)
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = json.Marshal(d)
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = json.Marshal(d)
		}
		_ = result
	})
}

func BenchmarkUnmarshalJSON(b *testing.B) {
	small := []byte(`"1234.56"`)
	large := []byte(`"123456789.123"`)

	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		var d alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			_ = json.Unmarshal(small, &d)
		}
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		var d alpacadecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			_ = json.Unmarshal(large, &d)
		}
	})

	b.Run("shopspring", func(b *testing.B) {
		var d decimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			_ = json.Unmarshal(large, &d)
		}
	})

	b.Run("udecimal", func(b *testing.B) {
		var d udecimal.Decimal
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			_ = json.Unmarshal(large, &d)
		}
	})
}

func BenchmarkMarshalText(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.MarshalText()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.MarshalText()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.MarshalText()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.MarshalText()
		}
		_ = result
	})
}

func BenchmarkUnmarshalText(b *testing.B) {
	small := []byte("1234.56")
	large := []byte("123456789.123")

	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d alpacadecimal.Decimal
			_ = d.UnmarshalText(small)
		}
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d alpacadecimal.Decimal
			_ = d.UnmarshalText(large)
		}
	})

	b.Run("shopspring", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d decimal.Decimal
			_ = d.UnmarshalText(large)
		}
	})

	b.Run("udecimal", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d udecimal.Decimal
			_ = d.UnmarshalText(large)
		}
	})
}

func BenchmarkMarshalBinary(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.MarshalBinary()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.MarshalBinary()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.MarshalBinary()
		}
		_ = result
	})

	b.Run("udecimal", func(b *testing.B) {
		d := udecimal.MustParse("123456789.123")
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.MarshalBinary()
		}
		_ = result
	})
}

func BenchmarkUnmarshalBinary(b *testing.B) {
	optData, _ := alpacadecimal.NewFromFloat(1234.56).MarshalBinary()
	fbData, _ := alpacadecimal.RequireFromString("123456789.123").MarshalBinary()
	ssData, _ := decimal.RequireFromString("123456789.123").MarshalBinary()
	udData, _ := udecimal.MustParse("123456789.123").MarshalBinary()

	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d alpacadecimal.Decimal
			_ = d.UnmarshalBinary(optData)
		}
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d alpacadecimal.Decimal
			_ = d.UnmarshalBinary(fbData)
		}
	})

	b.Run("shopspring", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d decimal.Decimal
			_ = d.UnmarshalBinary(ssData)
		}
	})

	b.Run("udecimal", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d udecimal.Decimal
			_ = d.UnmarshalBinary(udData)
		}
	})
}

func BenchmarkGobEncode(b *testing.B) {
	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		d := alpacadecimal.NewFromFloat(1234.56)
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.GobEncode()
		}
		_ = result
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		d := alpacadecimal.RequireFromString("123456789.123")
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.GobEncode()
		}
		_ = result
	})

	b.Run("shopspring", func(b *testing.B) {
		d := decimal.RequireFromString("123456789.123")
		var result []byte
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result, _ = d.GobEncode()
		}
		_ = result
	})
}

func BenchmarkGobDecode(b *testing.B) {
	optData, _ := alpacadecimal.NewFromFloat(1234.56).GobEncode()
	fbData, _ := alpacadecimal.RequireFromString("123456789.123").GobEncode()
	ssData, _ := decimal.RequireFromString("123456789.123").GobEncode()

	b.Run("alpacadecimal/optimized", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d alpacadecimal.Decimal
			_ = d.GobDecode(optData)
		}
	})

	b.Run("alpacadecimal/fallback", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d alpacadecimal.Decimal
			_ = d.GobDecode(fbData)
		}
	})

	b.Run("shopspring", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d decimal.Decimal
			_ = d.GobDecode(ssData)
		}
	})
}
