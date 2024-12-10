package alpacadecimal_test

import (
	"fmt"
	"math/big"
	"regexp"
	"testing"

	"github.com/alpacahq/alpacadecimal"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// helper func to format error if result not equal.
func shouldEqual(t *testing.T, left, right alpacadecimal.Decimal) {
	require.True(t, left.Equal(right), fmt.Sprintf("left (%s) should equal to right (%s)", left.String(), right.String()))
}

// test cases
var cases = []string{
	// zeros
	"0", "0.0", "0.000", "-0", "-0.0",

	// pos int
	"1", "2", "10", "100", "999", "10000", "123456", "999999999", "99999999999",

	// neg int
	"-1", "-2", "-10", "-100", "-999", "-10000", "-123456", "-999999999", "-99999999999",

	// pos decimal
	"0.1", "1.12", "0.334", "12.33345", "334.94378539458934589345", "20.0999009", "1000000000.123456", "100000000000000.01",

	// neg decimal
	"-0.1", "-1.12", "-0.334", "-12.33345", "-34.23493899450934859345304958345", "-20.0999009", "-1000000000.123456", "-100000000000000.01",
}

// helper func to check compatibility of alpacadecimal.Decimal and decimal.Decimal
func requireCompatible[T any](t *testing.T, f func(input string) (x, y T)) {
	for _, c := range cases {
		x, y := f(c)

		require.Equal(t, x, y, fmt.Sprintf("not compatible for test %s with input %s", t.Name(), c))
	}
}

// helper func to check compatibility of alpacadecimal.Decimal and decimal.Decimal with 2 inputs
func requireCompatible2[T any](t *testing.T, f func(input1, input2 string) (x, y T)) {
	for _, c := range cases {
		for _, c2 := range cases {
			x, y := f(c, c2)

			require.Equal(t, x, y, fmt.Sprintf("not compatible for test %s with input %s and %s", t.Name(), c, c2))
		}
	}
}

func TestDecimal(t *testing.T) {
	one := alpacadecimal.NewFromInt(1)
	two := alpacadecimal.NewFromInt(2)
	three := alpacadecimal.NewFromInt(3)

	t.Run("Zero", func(t *testing.T) {
		require.Equal(t, "0", alpacadecimal.Zero.String())
		require.True(t, alpacadecimal.Zero.Equal(alpacadecimal.Zero))
		require.True(t, alpacadecimal.Zero.GreaterThan(alpacadecimal.NewFromInt(-1)))
		require.True(t, alpacadecimal.Zero.LessThan(alpacadecimal.NewFromInt(1)))
	})

	t.Run("RescalePair", func(t *testing.T) {
		d1, d2 := alpacadecimal.RescalePair(one, two)
		shouldEqual(t, d1, one)
		shouldEqual(t, d2, two)
	})

	t.Run("Avg", func(t *testing.T) {
		shouldEqual(t, alpacadecimal.Avg(one, two, three), two)
	})

	t.Run("Max", func(t *testing.T) {
		require.True(t, alpacadecimal.Max(one, two, three).Equal(three))
	})

	t.Run("Min", func(t *testing.T) {
		require.True(t, alpacadecimal.Min(one, two, three).Equal(one))
	})

	t.Run("New", func(t *testing.T) {
		{
			x := alpacadecimal.New(1, -13)
			shouldEqual(t, x, alpacadecimal.RequireFromString("0.0000000000001"))
			require.False(t, x.IsOptimized())
		}

		{
			x := alpacadecimal.New(1_000_000_000_000, -12)
			shouldEqual(t, x, one)
			require.True(t, x.IsOptimized())
		}

		{
			x := alpacadecimal.New(1, -3)
			shouldEqual(t, x, alpacadecimal.NewFromFloat(0.001))
			require.True(t, x.IsOptimized())
		}

		{
			x := alpacadecimal.New(3, 0)
			shouldEqual(t, x, alpacadecimal.RequireFromString("3"))
			require.True(t, x.IsOptimized())
		}

		{
			x := alpacadecimal.New(3, 0)
			shouldEqual(t, x, alpacadecimal.RequireFromString("3"))
			require.True(t, x.IsOptimized())
		}

		{
			x := alpacadecimal.New(4, 1)
			shouldEqual(t, x, alpacadecimal.RequireFromString("40"))
			require.True(t, x.IsOptimized())
		}

		{
			x := alpacadecimal.New(5, 6)
			shouldEqual(t, x, alpacadecimal.RequireFromString("5000000"))
			require.True(t, x.IsOptimized())
		}

		{
			x := alpacadecimal.New(-9, 6)
			shouldEqual(t, x, alpacadecimal.RequireFromString("-9000000"))
			require.True(t, x.IsOptimized())
		}

		{
			x := alpacadecimal.New(1, 7)
			shouldEqual(t, x, alpacadecimal.RequireFromString("10000000"))
			require.False(t, x.IsOptimized())
		}

		{
			x := alpacadecimal.New(10_000_000, 0)
			shouldEqual(t, x, alpacadecimal.RequireFromString("10000000"))
			require.False(t, x.IsOptimized())
		}

		{
			x := alpacadecimal.New(1_000_000_000, -2)
			shouldEqual(t, x, alpacadecimal.RequireFromString("10000000"))
			require.False(t, x.IsOptimized())
		}
	})

	t.Run("NewFromBigInt", func(t *testing.T) {
		input := big.NewInt(123)

		x := alpacadecimal.NewFromBigInt(input, 2)
		y := decimal.NewFromBigInt(input, 2)

		require.Equal(t, x.String(), y.String())
	})

	t.Run("NewFromFloat", func(t *testing.T) {
		x := alpacadecimal.NewFromFloat(1.234567)
		y, err := alpacadecimal.NewFromString("1.234567")
		require.NoError(t, err)
		shouldEqual(t, x, y)
	})

	t.Run("NewFromFloat32", func(t *testing.T) {
		x := alpacadecimal.NewFromFloat32(-1.23)
		y, err := alpacadecimal.NewFromString("-1.23")
		require.NoError(t, err)
		shouldEqual(t, x, y)
	})

	t.Run("NewFromFloatWithExponent", func(t *testing.T) {
		input := 123.456

		x := alpacadecimal.NewFromFloatWithExponent(input, -2)
		y := decimal.NewFromFloatWithExponent(input, -2)

		require.Equal(t, x.String(), y.String())
	})

	t.Run("NewFromFormattedString", func(t *testing.T) {
		r := regexp.MustCompile("[$,]")

		input := "$5,125.99"

		x, err := alpacadecimal.NewFromFormattedString(input, r)
		require.NoError(t, err)

		y, err := decimal.NewFromFormattedString(input, r)
		require.NoError(t, err)

		require.Equal(t, x.String(), y.String())
	})

	t.Run("NewFromDecimal", func(t *testing.T) {
		// first, with optimized decimal
		x := alpacadecimal.NewFromDecimal(decimal.New(123, -2))
		y := alpacadecimal.New(123, -2)
		shouldEqual(t, x, y)

		// the prior means of conversion from decimal commonly used
		y = alpacadecimal.RequireFromString(decimal.New(123, -2).String())
		shouldEqual(t, x, y)

		// now, with out of optimization range decimal
		x = alpacadecimal.NewFromDecimal(decimal.New(123, -13))
		y = alpacadecimal.New(123, -13)
		shouldEqual(t, x, y)
	})

	t.Run("NewFromInt", func(t *testing.T) {
		x := alpacadecimal.NewFromInt(123)
		y, err := alpacadecimal.NewFromString("123")
		require.NoError(t, err)
		shouldEqual(t, x, y)
	})

	t.Run("NewFromInt32", func(t *testing.T) {
		x := alpacadecimal.NewFromInt32(-123)
		y, err := alpacadecimal.NewFromString("-123")
		require.NoError(t, err)
		shouldEqual(t, x, y)
	})

	t.Run("NewFromString", func(t *testing.T) {
		{
			d, err := alpacadecimal.NewFromString("2")
			require.NoError(t, err)
			require.Equal(t, "2", d.String())
			require.True(t, d.IsOptimized())

			d2, err := decimal.NewFromString("2")
			require.NoError(t, err)
			require.Equal(t, "2", d2.String())
		}

		{
			d, err := alpacadecimal.NewFromString("+2")
			require.NoError(t, err)
			require.Equal(t, "2", d.String())
			require.True(t, d.IsOptimized())

			d2, err := decimal.NewFromString("+2")
			require.NoError(t, err)
			require.Equal(t, "2", d2.String())
		}

		{
			d, err := alpacadecimal.NewFromString("-22")
			require.NoError(t, err)
			require.Equal(t, "-22", d.String())
			require.True(t, d.IsOptimized())

			d2, err := decimal.NewFromString("-22")
			require.NoError(t, err)
			require.Equal(t, "-22", d2.String())

		}

		{
			d, err := alpacadecimal.NewFromString(".123")
			require.NoError(t, err)
			require.Equal(t, "0.123", d.String())
			require.True(t, d.IsOptimized())

			d2, err := decimal.NewFromString(".123")
			require.NoError(t, err)
			require.Equal(t, "0.123", d2.String())
		}

		{
			d, err := alpacadecimal.NewFromString("-.123")
			require.NoError(t, err)
			require.Equal(t, "-0.123", d.String())
			require.True(t, d.IsOptimized())

			d2, err := decimal.NewFromString("-.123")
			require.NoError(t, err)
			require.Equal(t, "-0.123", d2.String())
		}
	})

	t.Run("RequireFromString", func(t *testing.T) {
		x := alpacadecimal.RequireFromString("1")
		shouldEqual(t, x, one)
	})

	t.Run("Sum", func(t *testing.T) {
		require.True(t, alpacadecimal.Sum(one, two).Equal(three))
	})

	t.Run("Decimal.Abs", func(t *testing.T) {
		require.True(t, alpacadecimal.NewFromInt(-1).Abs().Equal(one))
	})

	t.Run("Decimal.Add", func(t *testing.T) {
		require.True(t, one.Add(two).Equal(three))
	})

	t.Run("Decimal.Atan", func(t *testing.T) {
		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).Atan().String()
			y := decimal.RequireFromString(input).Atan().String()
			return x, y
		})
	})

	t.Run("Decimal.BigFloat", func(t *testing.T) {
		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).BigFloat().String()
			y := decimal.RequireFromString(input).BigFloat().String()
			return x, y
		})
	})

	t.Run("Decimal.BigInt", func(t *testing.T) {
		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).BigInt().String()
			y := decimal.RequireFromString(input).BigInt().String()
			return x, y
		})
	})

	t.Run("Decimal.Ceil", func(t *testing.T) {
		a1 := alpacadecimal.RequireFromString("1.234")
		b1 := alpacadecimal.RequireFromString("2")
		shouldEqual(t, a1.Ceil(), b1)

		a2 := alpacadecimal.RequireFromString("-1.234")
		b2 := alpacadecimal.RequireFromString("-1")
		shouldEqual(t, a2.Ceil(), b2)

		a3 := alpacadecimal.RequireFromString("0")
		b3 := alpacadecimal.RequireFromString("0")
		shouldEqual(t, a3.Ceil(), b3)

		a4 := alpacadecimal.RequireFromString("1")
		b4 := alpacadecimal.RequireFromString("1.0")
		shouldEqual(t, a4.Ceil(), b4)

		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).Ceil().String()
			y := decimal.RequireFromString(input).Ceil().String()
			return x, y
		})
	})

	t.Run("Decimal.Cmp", func(t *testing.T) {
		require.Equal(t, -1, one.Cmp(two))
		require.Equal(t, 0, one.Cmp(one))
		require.Equal(t, 1, three.Cmp(one))

		requireCompatible2(t, func(input1, input2 string) (int, int) {
			x := alpacadecimal.RequireFromString(input1).Cmp(alpacadecimal.RequireFromString(input2))
			y := decimal.RequireFromString(input1).Cmp(decimal.RequireFromString(input2))
			return x, y
		})
	})

	t.Run("Decimal.Coefficient", func(t *testing.T) {
		// this is not fully compatible
		//
		// requireCompatible(t, func(input string) (string, string) {
		// 	x := alpacadecimal.RequireFromString(input).Coefficient().String()
		// 	y := decimal.RequireFromString(input).Coefficient().String()
		// 	return x, y
		// })
	})

	t.Run("Decimal.CoefficientInt64", func(t *testing.T) {
		// this is not fully compatible
		//
		// requireCompatible(t, func(input string) (int64, int64) {
		// 	x := alpacadecimal.RequireFromString(input).CoefficientInt64()
		// 	y := decimal.RequireFromString(input).CoefficientInt64()
		// 	return x, y
		// })
	})

	t.Run("Decimal.Copy", func(t *testing.T) {
		{
			var a alpacadecimal.Decimal
			err := a.Scan("1")
			require.NoError(t, err)
			shouldEqual(t, a, one)

			b := a.Copy()
			err = b.Scan("2")
			require.NoError(t, err)
			shouldEqual(t, a, one)
			shouldEqual(t, b, two)
		}

		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).Copy().String()
			y := decimal.RequireFromString(input).Copy().String()
			return x, y
		})
	})

	t.Run("Decimal.Cos", func(t *testing.T) {
		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).Cos().String()
			y := decimal.RequireFromString(input).Cos().String()
			return x, y
		})
	})

	t.Run("Decimal.Div", func(t *testing.T) {
		checkIntDiv := func(a, b int64, expected string) {
			d1 := alpacadecimal.NewFromInt(a)
			d2 := alpacadecimal.NewFromInt(b)

			require.Equal(t, expected, d1.Div(d2).String())
		}

		checkIntDiv(1, 2, "0.5")
		checkIntDiv(122, 10, "12.2")

		checkFloatDiv := func(a, b float64, expected string) {
			d1 := alpacadecimal.NewFromFloat(a)
			d2 := alpacadecimal.NewFromFloat(b)

			require.Equal(t, expected, d1.Div(d2).String())
		}

		checkFloatDiv(1.1, 2.2, "0.5")
		checkFloatDiv(2.3, 0.3, "7.6666666666666667") // 16 precision
	})

	t.Run("Decimal.DivRound", func(t *testing.T) {
		// 3/4 = 0.75 => round 1 position => 0.8
		shouldEqual(t, three.DivRound(alpacadecimal.NewFromInt(4), 1), alpacadecimal.NewFromFloat(0.8))
	})

	t.Run("Decimal.Equal", func(t *testing.T) {
		shouldEqual(t, one, one)
		shouldEqual(t, two, two)
	})

	t.Run("Decimal.Equals", func(t *testing.T) {
		require.True(t, one.Equals(one))
		require.False(t, one.Equals(two))
	})

	t.Run("Decimal.ExpHullAbrham", func(t *testing.T) {
		// take too long to run
		//
		// for i := uint32(0); i < 10; i++ {
		// 	requireCompatible(t, func(input string) (string, string) {
		// 		x, err := alpacadecimal.RequireFromString(input).ExpHullAbrham(i)
		// 		require.NoError(t, err)

		// 		y, err := decimal.RequireFromString(input).ExpHullAbrham(i)
		// 		require.NoError(t, err)

		// 		return x.String(), y.String()
		// 	})
		// }
	})

	t.Run("Decimal.ExpTaylor", func(t *testing.T) {
		// take too long to run
		//
		// for i := int32(0); i < 10; i++ {
		// 	requireCompatible(t, func(input string) (string, string) {
		// 		x, err := alpacadecimal.RequireFromString(input).ExpTaylor(i)
		// 		require.NoError(t, err)

		// 		y, err := decimal.RequireFromString(input).ExpTaylor(i)
		// 		require.NoError(t, err)

		// 		return x.String(), y.String()
		// 	})
		// }
	})

	t.Run("Decimal.Exponent", func(t *testing.T) {
		require.Equal(t, int32(-12), alpacadecimal.RequireFromString("1").Exponent())
	})

	t.Run("Decimal.Float64", func(t *testing.T) {
		f, exact := alpacadecimal.RequireFromString("1.0").Float64()
		require.True(t, exact)
		require.Equal(t, float64(1), f)
	})

	t.Run("Decimal.Floor", func(t *testing.T) {
		a1 := alpacadecimal.RequireFromString("1.234")
		b1 := alpacadecimal.RequireFromString("1")
		shouldEqual(t, a1.Floor(), b1)

		a2 := alpacadecimal.RequireFromString("-1.234")
		b2 := alpacadecimal.RequireFromString("-2")
		shouldEqual(t, a2.Floor(), b2)

		a3 := alpacadecimal.RequireFromString("0")
		b3 := alpacadecimal.RequireFromString("0")
		shouldEqual(t, a3.Floor(), b3)

		a4 := alpacadecimal.RequireFromString("1")
		b4 := alpacadecimal.RequireFromString("1.0")
		shouldEqual(t, a4.Floor(), b4)

		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).Floor().String()
			y := decimal.RequireFromString(input).Floor().String()
			return x, y
		})
	})

	t.Run("Decimal.GobDecode & Decimal.GobEncode", func(t *testing.T) {
		x := alpacadecimal.NewFromInt(123456)
		data, err := x.GobEncode()
		require.NoError(t, err)

		var y alpacadecimal.Decimal
		err = y.GobDecode(data)
		require.NoError(t, err)

		shouldEqual(t, x, y)
	})

	t.Run("Decimal.GreaterThan", func(t *testing.T) {
		require.True(t, two.GreaterThan(one))
		require.False(t, two.GreaterThan(three))

		requireCompatible2(t, func(input1, input2 string) (bool, bool) {
			x := alpacadecimal.RequireFromString(input1).GreaterThan(alpacadecimal.RequireFromString(input2))
			y := decimal.RequireFromString(input1).GreaterThan(decimal.RequireFromString(input2))
			return x, y
		})
	})

	t.Run("Decimal.GreaterThanOrEqual", func(t *testing.T) {
		require.True(t, one.GreaterThanOrEqual(one))
		require.True(t, two.GreaterThanOrEqual(one))
		require.False(t, one.GreaterThanOrEqual(two))

		requireCompatible2(t, func(input1, input2 string) (bool, bool) {
			x := alpacadecimal.RequireFromString(input1).GreaterThanOrEqual(alpacadecimal.RequireFromString(input2))
			y := decimal.RequireFromString(input1).GreaterThanOrEqual(decimal.RequireFromString(input2))
			return x, y
		})
	})

	t.Run("Decimal.InexactFloat64", func(t *testing.T) {
		requireCompatible(t, func(input string) (float64, float64) {
			x := alpacadecimal.RequireFromString(input).InexactFloat64()
			y := decimal.RequireFromString(input).InexactFloat64()
			return x, y
		})
	})

	t.Run("Decimal.IntPart", func(t *testing.T) {
		x, err := alpacadecimal.NewFromString("1.1")
		require.NoError(t, err)
		require.Equal(t, int64(1), x.IntPart())

		y, err := alpacadecimal.NewFromString("-123.1")
		require.NoError(t, err)
		require.Equal(t, int64(-123), y.IntPart())

		requireCompatible(t, func(input string) (int64, int64) {
			x := alpacadecimal.RequireFromString(input).IntPart()
			y := decimal.RequireFromString(input).IntPart()
			return x, y
		})
	})

	t.Run("Decimal.IsInteger", func(t *testing.T) {
		x := alpacadecimal.RequireFromString("1.2")
		require.False(t, x.IsInteger())

		y := alpacadecimal.RequireFromString("123")
		require.True(t, y.IsInteger())

		requireCompatible(t, func(input string) (bool, bool) {
			x := alpacadecimal.RequireFromString(input).IsInteger()
			y := decimal.RequireFromString(input).IsInteger()
			return x, y
		})
	})

	t.Run("Decimal.IsNegative", func(t *testing.T) {
		x := alpacadecimal.RequireFromString("1.234")
		require.False(t, x.IsNegative())

		y := alpacadecimal.RequireFromString("0.0")
		require.False(t, y.IsNegative())

		z := alpacadecimal.RequireFromString("-12")
		require.True(t, z.IsNegative())

		requireCompatible(t, func(input string) (bool, bool) {
			x := alpacadecimal.RequireFromString(input).IsNegative()
			y := decimal.RequireFromString(input).IsNegative()
			return x, y
		})
	})

	t.Run("Decimal.IsPositive", func(t *testing.T) {
		x := alpacadecimal.RequireFromString("1.234")
		require.True(t, x.IsPositive())

		y := alpacadecimal.RequireFromString("0.0")
		require.False(t, y.IsPositive())

		z := alpacadecimal.RequireFromString("-12")
		require.False(t, z.IsPositive())

		requireCompatible(t, func(input string) (bool, bool) {
			x := alpacadecimal.RequireFromString(input).IsPositive()
			y := decimal.RequireFromString(input).IsPositive()
			return x, y
		})
	})

	t.Run("Decimal.IsZero", func(t *testing.T) {
		x := alpacadecimal.RequireFromString("1.234")
		require.False(t, x.IsZero())

		y := alpacadecimal.RequireFromString("0.0")
		require.True(t, y.IsZero())

		z := alpacadecimal.RequireFromString("-12")
		require.False(t, z.IsZero())

		requireCompatible(t, func(input string) (bool, bool) {
			x := alpacadecimal.RequireFromString(input).IsZero()
			y := decimal.RequireFromString(input).IsZero()
			return x, y
		})
	})

	t.Run("Decimal.LessThan", func(t *testing.T) {
		requireCompatible2(t, func(input1, input2 string) (bool, bool) {
			x := alpacadecimal.RequireFromString(input1).LessThan(alpacadecimal.RequireFromString(input2))
			y := decimal.RequireFromString(input1).LessThan(decimal.RequireFromString(input2))
			return x, y
		})
	})

	t.Run("Decimal.LessThanOrEqual", func(t *testing.T) {
		requireCompatible2(t, func(input1, input2 string) (bool, bool) {
			x := alpacadecimal.RequireFromString(input1).LessThanOrEqual(alpacadecimal.RequireFromString(input2))
			y := decimal.RequireFromString(input1).LessThanOrEqual(decimal.RequireFromString(input2))
			return x, y
		})
	})

	t.Run("Decimal.MarshalBinary", func(t *testing.T) {
		x := alpacadecimal.NewFromInt(123456)
		data, err := x.MarshalBinary()
		require.NoError(t, err)

		var y alpacadecimal.Decimal
		err = y.UnmarshalBinary(data)
		require.NoError(t, err)

		shouldEqual(t, x, y)
	})

	t.Run("Decimal.MarshalJSON", func(t *testing.T) {
		{
			var x alpacadecimal.Decimal
			err := x.UnmarshalJSON([]byte("123.456"))
			require.NoError(t, err)
			shouldEqual(t, x, alpacadecimal.New(123456, -3))
		}

		{
			var x alpacadecimal.Decimal
			err := x.UnmarshalJSON([]byte("error"))
			require.Error(t, err)
			shouldEqual(t, alpacadecimal.Zero, x)
		}
	})

	t.Run("Decimal.MarshalText", func(t *testing.T) {
		{
			var x alpacadecimal.Decimal
			err := x.UnmarshalText([]byte("123.456"))
			require.NoError(t, err)
			shouldEqual(t, x, alpacadecimal.New(123456, -3))
		}

		{
			var x alpacadecimal.Decimal
			err := x.UnmarshalText([]byte("error"))
			require.Error(t, err)
			shouldEqual(t, alpacadecimal.Zero, x)
		}
	})

	t.Run("Decimal.Mod", func(t *testing.T) {
		requireCompatible2(t, func(input1, input2 string) (string, string) {
			a := alpacadecimal.RequireFromString(input1)
			b := alpacadecimal.RequireFromString(input2).Floor()
			if b.IsZero() {
				b = alpacadecimal.RequireFromString("2")
			}
			r1 := a.Mod(b).String()

			x := decimal.RequireFromString(input1)
			y := decimal.RequireFromString(input2).Floor()
			if y.IsZero() {
				y = decimal.RequireFromString("2")
			}
			r2 := x.Mod(y).String()

			return r1, r2
		})
	})

	t.Run("Decimal.Mul", func(t *testing.T) {
		checkIntMul := func(a, b int64) {
			d1 := alpacadecimal.NewFromInt(a)
			d2 := alpacadecimal.NewFromInt(b)
			d3 := alpacadecimal.NewFromInt(a * b)

			require.True(t, d1.Mul(d2).Equal(d3))
		}

		checkIntMul(1, 2)
		checkIntMul(2, 3)

		checkFloatMul := func(a, b float64, expected string) {
			d1 := alpacadecimal.NewFromFloat(a)
			d2 := alpacadecimal.NewFromFloat(b)

			require.Equal(t, expected, d1.Mul(d2).String())
		}

		checkFloatMul(1.1, 2.2, "2.42")
		checkFloatMul(2.3, 0.3, "0.69")

		requireCompatible2(t, func(input1, input2 string) (string, string) {
			a := alpacadecimal.RequireFromString(input1)
			b := alpacadecimal.RequireFromString(input2).Floor()
			r1 := a.Mul(b).String()

			x := decimal.RequireFromString(input1)
			y := decimal.RequireFromString(input2).Floor()
			r2 := x.Mul(y).String()

			return r1, r2
		})
	})

	t.Run("Decimal.Neg", func(t *testing.T) {
		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).Neg().String()
			y := decimal.RequireFromString(input).Neg().String()
			return x, y
		})
	})

	t.Run("Decimal.NumDigits", func(t *testing.T) {
		// not fully compatible
		//
		// requireCompatible(t, func(input string) (int, int) {
		// 	x := alpacadecimal.RequireFromString(input).NumDigits()
		// 	y := decimal.RequireFromString(input).NumDigits()
		// 	return x, y
		// })
	})

	t.Run("Decimal.Pow", func(t *testing.T) {
		for i := int64(-100); i < 100; i += 1 {
			requireCompatible(t, func(input string) (string, string) {
				if alpacadecimal.RequireFromString(input).Equals(alpacadecimal.Zero) {
					// skip zero because decimal.Decimal would panic
					return "", ""
				}

				x := alpacadecimal.RequireFromString(input).Pow(alpacadecimal.NewFromInt(i))
				y := decimal.RequireFromString(input).Pow(decimal.NewFromInt(i))
				return x.String(), y.String()
			})
		}
	})

	t.Run("Decimal.QuoRem", func(t *testing.T) {
		for i := int32(0); i < 10; i++ {
			requireCompatible2(t, func(input1, input2 string) (string, string) {
				if alpacadecimal.RequireFromString(input2).Equals(alpacadecimal.Zero) {
					// skip if div by zero
					return "", ""
				}

				x1, x2 := alpacadecimal.RequireFromString(input1).QuoRem(alpacadecimal.RequireFromString(input2), i)
				y1, y2 := decimal.RequireFromString(input1).QuoRem(decimal.RequireFromString(input2), i)
				return x1.String() + ":" + x2.String(), y1.String() + ":" + y2.String()
			})
		}
	})

	t.Run("Decimal.Rat", func(t *testing.T) {
		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).Rat().String()
			y := decimal.RequireFromString(input).Rat().String()
			return x, y
		})
	})

	t.Run("Decimal.Round", func(t *testing.T) {
		for i := int32(0); i < 10; i++ {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).Round(i).String()
				y := decimal.RequireFromString(input).Round(i).String()
				return x, y
			})
		}

		require.Equal(t, "2", alpacadecimal.RequireFromString("1.5").Round(0).String())
		require.Equal(t, "2", decimal.RequireFromString("1.5").Round(0).String())

		require.Equal(t, "1.2", alpacadecimal.RequireFromString("1.23456").Round(1).String())
		require.Equal(t, "1.2", decimal.RequireFromString("1.23456").Round(1).String())

		require.Equal(t, "-1.23", alpacadecimal.RequireFromString("-1.23456").Round(2).String())
		require.Equal(t, "-1.23", decimal.RequireFromString("-1.23456").Round(2).String())

		require.Equal(t, "-1.235", alpacadecimal.RequireFromString("-1.23456").Round(3).String())
		require.Equal(t, "-1.235", decimal.RequireFromString("-1.23456").Round(3).String())

		require.Equal(t, "-1.2346", alpacadecimal.RequireFromString("-1.23456").Round(4).String())
		require.Equal(t, "-1.2346", decimal.RequireFromString("-1.23456").Round(4).String())

		require.Equal(t, "-1.23456", alpacadecimal.RequireFromString("-1.23456").Round(5).String())
		require.Equal(t, "-1.23456", decimal.RequireFromString("-1.23456").Round(5).String())

		require.Equal(t, "-1.23456", alpacadecimal.RequireFromString("-1.23456").Round(6).String())
		require.Equal(t, "-1.23456", decimal.RequireFromString("-1.23456").Round(6).String())
	})

	t.Run("Decimal.RoundBank", func(t *testing.T) {
		for i := int32(0); i < 10; i++ {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).RoundBank(i).String()
				y := decimal.RequireFromString(input).RoundBank(i).String()
				return x, y
			})
		}
	})

	t.Run("Decimal.RoundCash", func(t *testing.T) {
		for _, i := range []uint8{5, 10, 25, 50, 100} {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).RoundCash(i).String()
				y := decimal.RequireFromString(input).RoundCash(i).String()
				return x, y
			})
		}
	})

	t.Run("Decimal.RoundCeil", func(t *testing.T) {
		for i := int32(0); i < 10; i++ {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).RoundCeil(i).String()
				y := decimal.RequireFromString(input).RoundCeil(i).String()
				return x, y
			})
		}
	})

	t.Run("Decimal.RoundDown", func(t *testing.T) {
		for i := int32(0); i < 10; i++ {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).RoundDown(i).String()
				y := decimal.RequireFromString(input).RoundDown(i).String()
				return x, y
			})
		}
	})

	t.Run("Decimal.RoundFloor", func(t *testing.T) {
		for i := int32(0); i < 10; i++ {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).RoundFloor(i).String()
				y := decimal.RequireFromString(input).RoundFloor(i).String()
				return x, y
			})
		}
	})

	t.Run("Decimal.RoundUp", func(t *testing.T) {
		for i := int32(0); i < 10; i++ {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).RoundUp(i).String()
				y := decimal.RequireFromString(input).RoundUp(i).String()
				return x, y
			})
		}
	})

	t.Run("Decimal.Scan", func(t *testing.T) {
		check := func(source string) {
			var d alpacadecimal.Decimal
			err := d.Scan(source)
			require.NoError(t, err)
			require.Equal(t, source, d.String())
		}

		check("0")
		check("1")
		check("10")
		check("12")
		check("-1234")
		check("0.123")
		check("1.234")
	})

	t.Run("Decimal.Shift", func(t *testing.T) {
		for _, i := range []int32{1, 2, 3, 4, 5, 6} {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).Shift(i).String()
				y := decimal.RequireFromString(input).Shift(i).String()
				return x, y
			})
		}
	})

	t.Run("Decimal.Sign", func(t *testing.T) {
		requireCompatible(t, func(input string) (int, int) {
			x := alpacadecimal.RequireFromString(input).Sign()
			y := decimal.RequireFromString(input).Sign()
			return x, y
		})
	})

	t.Run("Decimal.Sin", func(t *testing.T) {
		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).Sin().String()
			y := decimal.RequireFromString(input).Sin().String()
			return x, y
		})
	})

	t.Run("Decimal.String", func(t *testing.T) {
		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).String()
			y := decimal.RequireFromString(input).String()
			return x, y
		})
	})

	t.Run("Decimal.StringFixed", func(t *testing.T) {
		for i := int32(0); i < 10; i++ {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).StringFixed(i)
				y := decimal.RequireFromString(input).StringFixed(i)
				return x, y
			})
		}
	})

	t.Run("Decimal.StringFixedBank", func(t *testing.T) {
		for i := int32(0); i < 10; i++ {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).StringFixedBank(i)
				y := decimal.RequireFromString(input).StringFixedBank(i)
				return x, y
			})
		}
	})

	t.Run("Decimal.StringFixedCash", func(t *testing.T) {
		for _, i := range []uint8{5, 10, 25, 50, 100} {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).StringFixedCash(i)
				y := decimal.RequireFromString(input).StringFixedCash(i)
				return x, y
			})
		}
	})

	t.Run("Decimal.StringScaled", func(t *testing.T) {
		for i := int32(0); i < 10; i++ {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).StringScaled(i)
				y := decimal.RequireFromString(input).StringScaled(i)
				return x, y
			})
		}
	})

	t.Run("Decimal.Sub", func(t *testing.T) {
		requireCompatible2(t, func(input1, input2 string) (string, string) {
			x := alpacadecimal.RequireFromString(input1).Sub(alpacadecimal.RequireFromString(input2)).String()
			y := decimal.RequireFromString(input1).Sub(decimal.RequireFromString(input2)).String()
			return x, y
		})
	})

	t.Run("Decimal.Tan", func(t *testing.T) {
		requireCompatible(t, func(input string) (string, string) {
			x := alpacadecimal.RequireFromString(input).Tan().String()
			y := decimal.RequireFromString(input).Tan().String()
			return x, y
		})
	})

	t.Run("Decimal.Truncate", func(t *testing.T) {
		x := alpacadecimal.NewFromFloat(1.234)
		require.Equal(t, "1", x.Truncate(0).String())
		require.Equal(t, "1.2", x.Truncate(1).String())
		require.Equal(t, "1.23", x.Truncate(2).String())
		require.Equal(t, "1.234", x.Truncate(3).String())
		require.Equal(t, "1.234", x.Truncate(4).String())

		y := alpacadecimal.NewFromFloat(-1.234)
		require.Equal(t, "-1", y.Truncate(0).String())
		require.Equal(t, "-1.2", y.Truncate(1).String())
		require.Equal(t, "-1.23", y.Truncate(2).String())
		require.Equal(t, "-1.234", y.Truncate(3).String())
		require.Equal(t, "-1.234", y.Truncate(4).String())

		for i := int32(0); i < 10; i++ {
			requireCompatible(t, func(input string) (string, string) {
				x := alpacadecimal.RequireFromString(input).Truncate(i).String()
				y := decimal.RequireFromString(input).Truncate(i).String()
				return x, y
			})
		}
	})

	t.Run("Decimal.UnmarshalBinary", func(t *testing.T) {
		x := alpacadecimal.NewFromInt(123)
		data, err := x.MarshalBinary()
		require.NoError(t, err)

		var y alpacadecimal.Decimal
		err = y.UnmarshalBinary(data)
		require.NoError(t, err)

		shouldEqual(t, x, y)
	})

	t.Run("Decimal.UnmarshalJSON", func(t *testing.T) {
		{
			x := alpacadecimal.NewFromInt(123)
			json, err := x.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, "\"123\"", string(json))
		}

		{
			x := alpacadecimal.NewFromInt(123456789)
			json, err := x.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, "\"123456789\"", string(json))
		}
	})

	t.Run("Decimal.UnmarshalText", func(t *testing.T) {
		{
			x := alpacadecimal.NewFromInt(123)
			text, err := x.MarshalText()
			require.NoError(t, err)
			require.Equal(t, "123", string(text))
		}

		{
			x := alpacadecimal.NewFromInt(123456789)
			text, err := x.MarshalText()
			require.NoError(t, err)
			require.Equal(t, "123456789", string(text))
		}
	})

	t.Run("Decimal.Value", func(t *testing.T) {
		checkInt := func(source int64, expected string) {
			d := alpacadecimal.NewFromInt(source)
			v, err := d.Value()
			require.NoError(t, err)
			require.Equal(t, expected, v.(string))
		}

		checkInt(0, "0")
		checkInt(123, "123")
		checkInt(-123, "-123")
		checkInt(12345, "12345")
		checkInt(-12345, "-12345")

		checkFloat := func(source float64, expected string) {
			d := alpacadecimal.NewFromFloat(source)
			v, err := d.Value()
			require.NoError(t, err)
			require.Equal(t, expected, v.(string))
		}

		checkFloat(0.0, "0")
		checkFloat(0.1, "0.1")
		checkFloat(-0.1, "-0.1")
		checkFloat(1.1, "1.1")
		checkFloat(-1.1, "-1.1")
		checkFloat(1.12, "1.12")
		checkFloat(-1.12, "-1.12")
		checkFloat(1000.12, "1000.12")
		checkFloat(-1000.12, "-1000.12")
		checkFloat(12345.123456789, "12345.123456789")
		checkFloat(-12345.123456789, "-12345.123456789")
	})

	t.Run("Decimal.GetFixed", func(t *testing.T) {
		x := alpacadecimal.NewFromInt(123)
		require.Equal(t, int64(123_000_000_000_000), x.GetFixed())

		y := alpacadecimal.NewFromInt(1234567890)
		require.Equal(t, int64(0), y.GetFixed())
	})

	t.Run("Decimal.GetFallback", func(t *testing.T) {
		x := alpacadecimal.NewFromInt(123)
		require.Nil(t, x.GetFallback())

		y := alpacadecimal.NewFromInt(1234567890)
		require.NotNil(t, y.GetFallback())
		require.Equal(t, "1234567890", y.GetFallback().String())
	})

	t.Run("Decimal.IsOptimized", func(t *testing.T) {
		x := alpacadecimal.NewFromInt(123)
		require.True(t, x.IsOptimized())

		y := alpacadecimal.NewFromInt(1234567890)
		require.False(t, y.IsOptimized())
	})

	t.Run("NullDecimal", func(t *testing.T) {
		var _ alpacadecimal.NullDecimal = alpacadecimal.NullDecimal{Decimal: alpacadecimal.NewFromInt(1), Valid: true}
		var _ alpacadecimal.NullDecimal = alpacadecimal.NullDecimal{Valid: false}
	})

	t.Run("NewNullDecimal", func(t *testing.T) {
		var _ alpacadecimal.NullDecimal = alpacadecimal.NewNullDecimal(alpacadecimal.NewFromInt(123))
	})

	t.Run("NullDecimal.MarshalJSON", func(t *testing.T) {
		{
			var x alpacadecimal.NullDecimal
			err := x.UnmarshalJSON([]byte("null"))
			require.NoError(t, err)
			require.False(t, x.Valid)
			shouldEqual(t, alpacadecimal.Zero, x.Decimal)
		}

		{
			var y alpacadecimal.NullDecimal
			err := y.UnmarshalJSON([]byte("123.456"))
			require.NoError(t, err)
			require.True(t, y.Valid)
			shouldEqual(t, y.Decimal, alpacadecimal.New(123456, -3))
		}

		{
			var z alpacadecimal.NullDecimal
			err := z.UnmarshalJSON([]byte("error"))
			require.Error(t, err)
			require.True(t, z.Valid) // this is to be consistent with original decimal.NullDecimal behaviour
			shouldEqual(t, alpacadecimal.Zero, z.Decimal)
		}
	})

	t.Run("NullDecimal.MarshalText", func(t *testing.T) {
		{
			var x alpacadecimal.NullDecimal
			err := x.UnmarshalText([]byte(""))
			require.NoError(t, err)
			require.False(t, x.Valid)
			shouldEqual(t, alpacadecimal.Zero, x.Decimal)
		}

		{
			var y alpacadecimal.NullDecimal
			err := y.UnmarshalText([]byte("123.456"))
			require.NoError(t, err)
			require.True(t, y.Valid)
			shouldEqual(t, y.Decimal, alpacadecimal.New(123456, -3))
		}

		{
			var z alpacadecimal.NullDecimal
			err := z.UnmarshalText([]byte("error"))
			require.Error(t, err)
			require.False(t, z.Valid)
			shouldEqual(t, alpacadecimal.Zero, z.Decimal)
		}
	})

	t.Run("NullDecimal.Scan", func(t *testing.T) {
		{
			var x alpacadecimal.NullDecimal
			err := x.Scan(nil)
			require.NoError(t, err)
			require.False(t, x.Valid)
			shouldEqual(t, alpacadecimal.Zero, x.Decimal)
		}

		{
			var x alpacadecimal.NullDecimal
			err := x.Scan("123")
			require.NoError(t, err)
			require.True(t, x.Valid)
			shouldEqual(t, alpacadecimal.NewFromInt(123), x.Decimal)
		}

		{
			var x alpacadecimal.NullDecimal
			err := x.Scan(int64(123))
			require.NoError(t, err)
			require.True(t, x.Valid)
			shouldEqual(t, alpacadecimal.NewFromInt(123), x.Decimal)
		}

		{
			var x alpacadecimal.NullDecimal
			err := x.Scan("error")
			require.Error(t, err)
			require.True(t, x.Valid) // this is to be consistent with decimal.NullDecimal
			shouldEqual(t, alpacadecimal.Zero, x.Decimal)
		}
	})

	t.Run("NullDecimal.UnmarshalJSON", func(t *testing.T) {
		{
			x := alpacadecimal.NullDecimal{Valid: false}
			json, err := x.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, "null", string(json))
		}

		{
			x := alpacadecimal.NullDecimal{Valid: true, Decimal: alpacadecimal.NewFromInt(123)}
			json, err := x.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, "\"123\"", string(json))
		}

		{
			x := alpacadecimal.NullDecimal{Valid: true, Decimal: alpacadecimal.NewFromInt(123456789)}
			json, err := x.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, "\"123456789\"", string(json))
		}
	})

	t.Run("NullDecimal.UnmarshalText", func(t *testing.T) {
		{
			x := alpacadecimal.NullDecimal{Valid: false}
			text, err := x.MarshalText()
			require.NoError(t, err)
			require.Equal(t, "", string(text))
		}

		{
			x := alpacadecimal.NullDecimal{Valid: true, Decimal: alpacadecimal.NewFromInt(123)}
			text, err := x.MarshalText()
			require.NoError(t, err)
			require.Equal(t, "123", string(text))
		}

		{
			x := alpacadecimal.NullDecimal{Valid: true, Decimal: alpacadecimal.NewFromInt(123456789)}
			text, err := x.MarshalText()
			require.NoError(t, err)
			require.Equal(t, "123456789", string(text))
		}
	})

	t.Run("NullDecimal.Value", func(t *testing.T) {
		{
			x := alpacadecimal.NullDecimal{Valid: false}
			v, err := x.Value()
			require.NoError(t, err)
			require.Nil(t, v)
		}

		{
			x := alpacadecimal.NullDecimal{Valid: true, Decimal: alpacadecimal.NewFromInt(123)}
			v, err := x.Value()
			require.NoError(t, err)
			require.Equal(t, "123", v.(string))
		}

		{
			x := alpacadecimal.NullDecimal{Valid: true, Decimal: alpacadecimal.NewFromInt(123456789)}
			v, err := x.Value()
			require.NoError(t, err)
			require.Equal(t, "123456789", v.(string))
		}
	})
}

func TestSpecialAPIs(t *testing.T) {
	x := alpacadecimal.NewFromInt(123)
	require.Equal(t, int32(-12), x.Exponent())
	require.Equal(t, "123000000000000", x.Coefficient().String())
	require.Equal(t, int64(123000000000000), x.CoefficientInt64())
	require.Equal(t, 15, x.NumDigits())

	y := decimal.NewFromInt(123)
	require.Equal(t, int32(0), y.Exponent())
	require.Equal(t, "123", y.Coefficient().String())
	require.Equal(t, int64(123), y.CoefficientInt64())
	require.Equal(t, 3, y.NumDigits())
}
