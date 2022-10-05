package alpacadecimal_test

import (
	"testing"

	"github.com/alpacahq/alpacadecimal"
	"github.com/stretchr/testify/require"
)

func TestDecimalValue(t *testing.T) {

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
}

func TestDecimalScan(t *testing.T) {
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
}

func TestDecimalMul(t *testing.T) {
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
}
