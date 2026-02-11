package alpacadecimal_test

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"testing"

	"github.com/quagmt/udecimal"
	"github.com/stretchr/testify/require"

	"github.com/alpacahq/alpacadecimal"
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
	"0.1", "1.12", "0.334", "12.33345", "334.9437853945893458", "20.0999009", "1000000000.123456", "100000000000000.01",
	"123456.123456789", "123456.1234567890", "123456.12345678901", "123456.123456789012", "123456.1234567890123",
	"1234567.123456789", "1234567.1234567890", "1234567.12345678901", "1234567.123456789012", "1234567.1234567890123",
	"12345678.123456789", "12345678.1234567890", "12345678.12345678901", "12345678.123456789012", "12345678.1234567890123",
	"9000000", "9000000.1",
	"9223371", "9223371.4", "9223371.5",
	"9223372.4", "9223372.5",
	"9223373",
	"9999999", "9999999.1",
	"10000000", "10000000.1",

	// neg decimal
	"-0.1", "-1.12", "-0.334", "-12.33345", "-34.2349389945093485", "-20.0999009", "-1000000000.123456", "-100000000000000.01",
	"-123456.123456789", "-123456.1234567890", "-123456.12345678901", "-123456.123456789012", "-123456.1234567890123",
	"-1234567.123456789", "-1234567.1234567890", "-1234567.12345678901", "-1234567.123456789012", "-1234567.1234567890123",
	"-12345678.123456789", "-12345678.1234567890", "-12345678.12345678901", "-12345678.123456789012", "-12345678.1234567890123",
	"-9000000", "-9000000.1",
	"-9223371", "-9223371.4", "-9223371.5",
	"-9223372.4", "-9223372.5",
	"-9223373",
	"-9999999", "-9999999.1",
	"-10000000", "-10000000.1",
}

// helper func to check compatibility of alpacadecimal.Decimal
func requireCompatible[T any](t *testing.T, f func(input string) (x, y T), msgAndArgs ...interface{}) {
	for _, c := range cases {
		x, y := f(c)

		require.Equal(t, x, y, fmt.Sprintf("not compatible for test %s with input %s", t.Name(), c), msgAndArgs)
	}
}

// helper func to check compatibility of alpacadecimal.Decimal with 2 inputs
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
		require.Equal(t, "12300", x.String())
	})

	t.Run("NewFromFloat", func(t *testing.T) {
		{
			x := alpacadecimal.NewFromFloat(1.234567)
			y, err := alpacadecimal.NewFromString("1.234567")
			require.NoError(t, err)
			shouldEqual(t, x, y)
		}
		{
			// This input caused optimized NewFromFloat to return an incorrect
			// value.
			x := alpacadecimal.NewFromFloat(17600.095)
			y, err := alpacadecimal.NewFromString("17600.095")
			require.NoError(t, err)
			shouldEqual(t, x, y)
		}
	})

	t.Run("NewFromFloat32", func(t *testing.T) {
		x := alpacadecimal.NewFromFloat32(-1.23)
		y, err := alpacadecimal.NewFromString("-1.23")
		require.NoError(t, err)
		shouldEqual(t, x, y)
	})

	t.Run("NewFromFloatWithExponent", func(t *testing.T) {
		x := alpacadecimal.NewFromFloatWithExponent(123.456, -2)
		require.Equal(t, "123.45", x.String())
	})

	t.Run("NewFromFormattedString", func(t *testing.T) {
		r := regexp.MustCompile("[$,]")

		input := "$5,125.99"

		x, err := alpacadecimal.NewFromFormattedString(input, r)
		require.NoError(t, err)

		require.Equal(t, "5125.99", x.String())
	})

	t.Run("NewFromDecimal", func(t *testing.T) {
		// first, with optimized decimal
		x := alpacadecimal.NewFromDecimal(udecimal.MustParse("1.23"))
		y := alpacadecimal.New(123, -2)
		shouldEqual(t, x, y)

		// the prior means of conversion from decimal commonly used
		y = alpacadecimal.RequireFromString(udecimal.MustParse("1.23").String())
		shouldEqual(t, x, y)

		// now, with out of optimization range decimal
		x = alpacadecimal.NewFromDecimal(udecimal.MustParse("0.0000000000001"))
		y = alpacadecimal.New(1, -13)
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
		}

		{
			d, err := alpacadecimal.NewFromString("+2")
			require.NoError(t, err)
			require.Equal(t, "2", d.String())
			require.True(t, d.IsOptimized())
		}

		{
			d, err := alpacadecimal.NewFromString("-22")
			require.NoError(t, err)
			require.Equal(t, "-22", d.String())
			require.True(t, d.IsOptimized())
		}

		{
			d, err := alpacadecimal.NewFromString(".123")
			require.NoError(t, err)
			require.Equal(t, "0.123", d.String())
			require.True(t, d.IsOptimized())
		}

		{
			d, err := alpacadecimal.NewFromString("-.123")
			require.NoError(t, err)
			require.Equal(t, "-0.123", d.String())
			require.True(t, d.IsOptimized())
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

	t.Run("Decimal.BigFloat", func(t *testing.T) {
		// Verify BigFloat returns correct values
		bf := alpacadecimal.RequireFromString("123.456").BigFloat()
		require.NotNil(t, bf)
		f, _ := bf.Float64()
		require.InDelta(t, 123.456, f, 0.0001)
	})

	t.Run("Decimal.BigInt", func(t *testing.T) {
		// Verify BigInt returns correct integer parts
		require.Equal(t, "123", alpacadecimal.RequireFromString("123.456").BigInt().String())
		require.Equal(t, "-123", alpacadecimal.RequireFromString("-123.456").BigInt().String())
		require.Equal(t, "0", alpacadecimal.RequireFromString("0.5").BigInt().String())
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
	})

	t.Run("Decimal.Cmp", func(t *testing.T) {
		require.Equal(t, -1, one.Cmp(two))
		require.Equal(t, 0, one.Cmp(one))
		require.Equal(t, 1, three.Cmp(one))
	})

	t.Run("Decimal.Coefficient", func(t *testing.T) {
		// this is not fully compatible
	})

	t.Run("Decimal.CoefficientInt64", func(t *testing.T) {
		// this is not fully compatible
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
			y := alpacadecimal.RequireFromString(input).String() // just compare with itself
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
	})

	t.Run("Decimal.GreaterThanOrEqual", func(t *testing.T) {
		require.True(t, one.GreaterThanOrEqual(one))
		require.True(t, two.GreaterThanOrEqual(one))
		require.False(t, one.GreaterThanOrEqual(two))
	})

	t.Run("Decimal.InexactFloat64", func(t *testing.T) {
		f := alpacadecimal.RequireFromString("1.5").InexactFloat64()
		require.Equal(t, 1.5, f)
	})

	t.Run("Decimal.IntPart", func(t *testing.T) {
		x, err := alpacadecimal.NewFromString("1.1")
		require.NoError(t, err)
		require.Equal(t, int64(1), x.IntPart())

		y, err := alpacadecimal.NewFromString("-123.1")
		require.NoError(t, err)
		require.Equal(t, int64(-123), y.IntPart())
	})

	t.Run("Decimal.IsInteger", func(t *testing.T) {
		x := alpacadecimal.RequireFromString("1.2")
		require.False(t, x.IsInteger())

		y := alpacadecimal.RequireFromString("123")
		require.True(t, y.IsInteger())
	})

	t.Run("Decimal.IsNegative", func(t *testing.T) {
		x := alpacadecimal.RequireFromString("1.234")
		require.False(t, x.IsNegative())

		y := alpacadecimal.RequireFromString("0.0")
		require.False(t, y.IsNegative())

		z := alpacadecimal.RequireFromString("-12")
		require.True(t, z.IsNegative())
	})

	t.Run("Decimal.IsPositive", func(t *testing.T) {
		x := alpacadecimal.RequireFromString("1.234")
		require.True(t, x.IsPositive())

		y := alpacadecimal.RequireFromString("0.0")
		require.False(t, y.IsPositive())

		z := alpacadecimal.RequireFromString("-12")
		require.False(t, z.IsPositive())
	})

	t.Run("Decimal.IsZero", func(t *testing.T) {
		x := alpacadecimal.RequireFromString("1.234")
		require.False(t, x.IsZero())

		y := alpacadecimal.RequireFromString("0.0")
		require.True(t, y.IsZero())

		z := alpacadecimal.RequireFromString("-12")
		require.False(t, z.IsZero())
	})

	t.Run("Decimal.LessThan", func(t *testing.T) {
		require.True(t, one.LessThan(two))
		require.False(t, two.LessThan(one))
	})

	t.Run("Decimal.LessThanOrEqual", func(t *testing.T) {
		require.True(t, one.LessThanOrEqual(one))
		require.True(t, one.LessThanOrEqual(two))
		require.False(t, two.LessThanOrEqual(one))
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
		// Basic mod tests
		a := alpacadecimal.RequireFromString("10")
		b := alpacadecimal.RequireFromString("3")
		require.Equal(t, "1", a.Mod(b).String())

		c := alpacadecimal.RequireFromString("-10")
		d := alpacadecimal.RequireFromString("3")
		require.Equal(t, "-1", c.Mod(d).String())
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
	})

	t.Run("Decimal.Neg", func(t *testing.T) {
		x := alpacadecimal.RequireFromString("1.23")
		require.Equal(t, "-1.23", x.Neg().String())

		y := alpacadecimal.RequireFromString("-4.56")
		require.Equal(t, "4.56", y.Neg().String())
	})

	t.Run("Decimal.NumDigits", func(t *testing.T) {
		// not fully compatible
	})

	t.Run("Decimal.Pow", func(t *testing.T) {
		// Basic pow tests
		x := alpacadecimal.RequireFromString("2")
		require.Equal(t, "8", x.Pow(alpacadecimal.NewFromInt(3)).String())

		y := alpacadecimal.RequireFromString("10")
		require.Equal(t, "1", y.Pow(alpacadecimal.NewFromInt(0)).String())
	})

	t.Run("Decimal.QuoRem", func(t *testing.T) {
		// Basic quorem test
		a := alpacadecimal.RequireFromString("10")
		b := alpacadecimal.RequireFromString("3")
		q, r := a.QuoRem(b, 0)
		require.Equal(t, "3", q.String())
		require.Equal(t, "1", r.String())
	})

	t.Run("Decimal.Rat", func(t *testing.T) {
		r := alpacadecimal.RequireFromString("1.5").Rat()
		require.Equal(t, "3/2", r.RatString())
	})

	t.Run("Decimal.Round", func(t *testing.T) {
		require.Equal(t, "2", alpacadecimal.RequireFromString("1.5").Round(0).String())
		require.Equal(t, "1.2", alpacadecimal.RequireFromString("1.23456").Round(1).String())
		require.Equal(t, "-1.23", alpacadecimal.RequireFromString("-1.23456").Round(2).String())
		require.Equal(t, "-1.235", alpacadecimal.RequireFromString("-1.23456").Round(3).String())
		require.Equal(t, "-1.2346", alpacadecimal.RequireFromString("-1.23456").Round(4).String())
		require.Equal(t, "-1.23456", alpacadecimal.RequireFromString("-1.23456").Round(5).String())
		require.Equal(t, "-1.23456", alpacadecimal.RequireFromString("-1.23456").Round(6).String())
	})

	t.Run("Decimal.RoundBank", func(t *testing.T) {
		// Bank rounding: 1.5 -> 2, 2.5 -> 2
		require.Equal(t, "2", alpacadecimal.RequireFromString("1.5").RoundBank(0).String())
		require.Equal(t, "2", alpacadecimal.RequireFromString("2.5").RoundBank(0).String())
		require.Equal(t, "4", alpacadecimal.RequireFromString("3.5").RoundBank(0).String())
	})

	t.Run("Decimal.RoundCash", func(t *testing.T) {
		require.Equal(t, "3.45", alpacadecimal.RequireFromString("3.43").RoundCash(5).String())
		require.Equal(t, "3.5", alpacadecimal.RequireFromString("3.45").RoundCash(10).String())
	})

	t.Run("Decimal.RoundCeil", func(t *testing.T) {
		require.Equal(t, "1.11", alpacadecimal.RequireFromString("1.1001").RoundCeil(2).String())
		require.Equal(t, "-1.4", alpacadecimal.RequireFromString("-1.454").RoundCeil(1).String())
	})

	t.Run("Decimal.RoundDown", func(t *testing.T) {
		require.Equal(t, "500", alpacadecimal.RequireFromString("545").RoundDown(-2).String())
		require.Equal(t, "1.1", alpacadecimal.RequireFromString("1.1001").RoundDown(2).String())
	})

	t.Run("Decimal.RoundFloor", func(t *testing.T) {
		require.Equal(t, "1.1", alpacadecimal.RequireFromString("1.1001").RoundFloor(2).String())
		require.Equal(t, "-1.5", alpacadecimal.RequireFromString("-1.454").RoundFloor(1).String())
	})

	t.Run("Decimal.RoundUp", func(t *testing.T) {
		require.Equal(t, "600", alpacadecimal.RequireFromString("545").RoundUp(-2).String())
		require.Equal(t, "500", alpacadecimal.RequireFromString("500").RoundUp(-2).String())
		require.Equal(t, "1.11", alpacadecimal.RequireFromString("1.1001").RoundUp(2).String())
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
		x := alpacadecimal.RequireFromString("1.23")
		require.Equal(t, "12.3", x.Shift(1).String())
		require.Equal(t, "123", x.Shift(2).String())
	})

	t.Run("Decimal.Sign", func(t *testing.T) {
		require.Equal(t, 1, alpacadecimal.RequireFromString("1.23").Sign())
		require.Equal(t, 0, alpacadecimal.RequireFromString("0").Sign())
		require.Equal(t, -1, alpacadecimal.RequireFromString("-1.23").Sign())
	})

	t.Run("Decimal.String", func(t *testing.T) {
		requireCompatible(t, func(input string) (string, string) {
			d := alpacadecimal.RequireFromString(input)
			x := d.String()
			// Round-trip: parse our own output to verify consistency
			y := alpacadecimal.RequireFromString(x).String()
			return x, y
		})
	})

	t.Run("Decimal.StringFixed", func(t *testing.T) {
		require.Equal(t, "1.23", alpacadecimal.RequireFromString("1.234").StringFixed(2))
		require.Equal(t, "1.2340", alpacadecimal.RequireFromString("1.234").StringFixed(4))
		require.Equal(t, "1", alpacadecimal.RequireFromString("1.234").StringFixed(0))
	})

	t.Run("Decimal.StringFixedBank", func(t *testing.T) {
		require.Equal(t, "1.24", alpacadecimal.RequireFromString("1.235").StringFixedBank(2))
		require.Equal(t, "1.22", alpacadecimal.RequireFromString("1.225").StringFixedBank(2))
	})

	t.Run("Decimal.StringFixedCash", func(t *testing.T) {
		require.Equal(t, "3.45", alpacadecimal.RequireFromString("3.43").StringFixedCash(5))
	})

	t.Run("Decimal.StringScaled", func(t *testing.T) {
		require.Equal(t, "1.23", alpacadecimal.RequireFromString("1.234").StringScaled(-2))
	})

	t.Run("Decimal.Sub", func(t *testing.T) {
		require.Equal(t, "-1", one.Sub(two).String())
		require.Equal(t, "1", two.Sub(one).String())
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
}

func TestNewFromStringHighPrecision(t *testing.T) {
	// Strings with >19 fractional digits should be truncated, not rejected.
	tests := []struct {
		input    string
		expected string
	}{
		// 26 fractional digits (margin interest from ledger)
		{"23.86564595277777777790548692", "23.8656459527777777779"},
		// 20 fractional digits
		{"-0.00047067901234567857", "-0.0004706790123456785"},
		// exactly 19 fractional digits (no truncation)
		{"1.1234567890123456789", "1.1234567890123456789"},
		// 25 fractional digits, negative
		{"-99.1234567890123456789012345", "-99.1234567890123456789"},
		// many trailing digits after 19 (trailing zeros get trimmed by udecimal)
		{"0.1000000000000000000999", "0.1"},
		// integer with no fractional part (unaffected)
		{"12345", "12345"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := alpacadecimal.NewFromString(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.expected, d.String())
		})
	}

	// RequireFromString should not panic on >19 fractional digits
	require.NotPanics(t, func() {
		alpacadecimal.RequireFromString("23.86564595277777777790548692")
	})
}

func TestNewFromFloatHighPrecision(t *testing.T) {
	// Floats whose minimal string representation exceeds 19 fractional digits
	// should be rounded to 19 fractional digits, not panic.
	tests := []struct {
		name  string
		bits  uint64
		check func(t *testing.T, d alpacadecimal.Decimal)
	}{
		{
			name: "very small float from oms2",
			bits: 0x3f269704679aa53d, // ≈0.000172347...
			check: func(t *testing.T, d alpacadecimal.Decimal) {
				require.True(t, d.IsPositive())
				require.True(t, d.LessThan(alpacadecimal.RequireFromString("0.001")))
			},
		},
		{
			name: "negative small float",
			bits: func() uint64 {
				return math.Float64bits(-0.00047067901234567857)
			}(),
			check: func(t *testing.T, d alpacadecimal.Decimal) {
				require.True(t, d.IsNegative())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := math.Float64frombits(tt.bits)
			require.NotPanics(t, func() {
				d := alpacadecimal.NewFromFloat(f)
				tt.check(t, d)
			})
		})
	}

	// NaN and Inf should still panic
	require.Panics(t, func() { alpacadecimal.NewFromFloat(math.NaN()) })
	require.Panics(t, func() { alpacadecimal.NewFromFloat(math.Inf(1)) })
	require.Panics(t, func() { alpacadecimal.NewFromFloat(math.Inf(-1)) })
}

func TestCoefficientExponentRoundtrip(t *testing.T) {
	// Coefficient and Exponent must satisfy: value = Coefficient * 10^Exponent
	tests := []struct {
		input string
	}{
		// optimized (fixed-point) values
		{"0"},
		{"1"},
		{"-1"},
		{"123.456"},
		{"9223372"},
		// fallback values
		{"10000000"},
		{"10000000.0"},
		{"0.0000000000001"},
		{"99999999999.123456789"},
		{"-123456789.987654321"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d := alpacadecimal.RequireFromString(tt.input)
			coeff := d.Coefficient()
			exp := d.Exponent()

			// Reconstruct: coeff * 10^exp
			var reconstructed *big.Rat
			if exp >= 0 {
				scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)
				reconstructed = new(big.Rat).SetInt(new(big.Int).Mul(coeff, scale))
			} else {
				scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exp)), nil)
				reconstructed = new(big.Rat).SetFrac(coeff, scale)
			}

			original := new(big.Rat)
			original.SetString(tt.input)

			require.True(t, original.Cmp(reconstructed) == 0,
				"input=%s coeff=%s exp=%d reconstructed=%s",
				tt.input, coeff.String(), exp, reconstructed.FloatString(20))
		})
	}
}
