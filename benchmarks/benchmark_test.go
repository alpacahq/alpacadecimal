//go:build go1.23

package benchmarks

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	ericdecimal "github.com/ericlagergren/decimal"
	ericpostgres "github.com/ericlagergren/decimal/sql/postgres"
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

func BenchmarkNewFromInt(b *testing.B) {
	b.Run("alpacadecimal", func(b *testing.B) {
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

	b.Run("eric", func(b *testing.B) {
		d1 := ericdecimal.New(123456789123, 3)
		d2 := ericdecimal.New(223456789123, 3)
		result := ericdecimal.New(0, 0)
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = result.Add(d1, d2)
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

	b.Run("eric", func(b *testing.B) {
		d1 := ericdecimal.New(789012, 2)
		d2 := ericdecimal.New(123456, 2)
		result := ericdecimal.New(0, 0)
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = result.Sub(d1, d2)
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

	b.Run("eric", func(b *testing.B) {
		d1 := ericdecimal.New(123, 2)
		d2 := ericdecimal.New(2, 0)
		result := ericdecimal.New(0, 0)
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = result.Mul(d1, d2)
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

	b.Run("eric", func(b *testing.B) {
		d1 := ericdecimal.New(123, 2)
		d2 := ericdecimal.New(2, 0)
		result := ericdecimal.New(0, 0)
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = result.Quo(d1, d2)
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
		d2 := alpacadecimal.NewFromFloat(7890.12)
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
		d2 := alpacadecimal.RequireFromString("223456789.123")
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
		d2 := alpacadecimal.RequireFromString("123456789.123")
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
		d2 := decimal.RequireFromString("223456789.123")
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
		d2 := udecimal.MustParse("223456789.123")
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
		d2 := alpacadecimal.RequireFromString("123456789.123")
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
		d2 := decimal.RequireFromString("123456789.123")
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
		d2 := udecimal.MustParse("123456789.123")
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
		d2 := alpacadecimal.RequireFromString("123456789.123")
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
		d2 := decimal.RequireFromString("123456789.123")
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
		d2 := udecimal.MustParse("123456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.GreaterThan(d2)
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
		d2 := alpacadecimal.RequireFromString("223456789.123")
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
		d2 := decimal.RequireFromString("223456789.123")
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
		d2 := udecimal.MustParse("223456789.123")
		var result bool
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			result = d1.LessThan(d2)
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

	b.Run("eric", func(b *testing.B) {
		d := ericdecimal.New(10000000123456, 6)
		var result *ericdecimal.Big
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
			result = d.RoundBank(2)
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

	b.Run("eric", func(b *testing.B) {
		d := ericdecimal.New(123, 2)
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

	b.Run("eric", func(b *testing.B) {
		v := ericdecimal.New(123, 0)
		d := ericpostgres.Decimal{V: v}
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

	b.Run("eric", func(b *testing.B) {
		var err error
		b.ResetTimer()
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			var d ericpostgres.Decimal
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
