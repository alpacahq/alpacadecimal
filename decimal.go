package alpacadecimal

import (
	"database/sql/driver"
	"math"
	"math/big"
	"strconv"

	"github.com/shopspring/decimal"
)

// currently support 12 precision, this is tunnable,
// more precision => smaller maxInt
// less precision => bigger maxInt
const scale = 1e12
const maxInt int64 = int64(math.MaxInt64) / scale
const minInt int64 = int64(math.MinInt64) / scale
const maxIntInFixed int64 = maxInt * scale
const minIntInFixed int64 = minInt * scale
const a1000InFixed int64 = 1000 * scale
const aNeg1000InFixed int64 = -1000 * scale
const aCentInFixed int64 = scale / 100

var pow10Table []int64 = []int64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5,
	1e6, 1e7, 1e8, 1e9, 1e10, 1e11,
	1e11, 1e12, 1e13, 1e14, 1e15, 1e16,
}

type Decimal struct {
	// represent decimal with 12 precision, 1.23 will have `i = 1_230_000_000_000`
	// max support decimal is 9_223_372.000_000_000_000
	// min support decimal is -9_223_372.000_000_000_000
	fixed int64

	// fallback to original decimal.Decimal if necessary
	fallback *decimal.Decimal
}

// cache value from -1000.00 to 1000.00
// with `valueCache[0] = "-1000"`
//      `valueCache[100000] = "0"`
//      `valueCache[200000] = "1000"`
// this consumes about 9 MB in memory with pprof check.
var valueCache [200001]driver.Value

// var scanCache map[string]int64 // not sure if it worths it ?

func init() {
	// init cache
	for i := 0; i < 200001; i++ {
		str := strconv.FormatFloat(float64(i-100000)/100, 'f', -1, 64)

		valueCache[i] = str
		// scanCache[str] = int64(i-100000) * scale / 100
	}
}

func NewFromInt(x int64) Decimal {
	if x >= minInt && x <= maxInt {
		return Decimal{fixed: x * scale}
	}
	fallback := decimal.NewFromInt(x)
	return Decimal{fallback: &fallback}
}

func NewFromFloat(f float64) Decimal {
	picoFloat := f * float64(scale)
	picoInt64 := int64(picoFloat)

	// check if it's within range and is whole number
	if picoInt64 >= minIntInFixed && picoInt64 <= maxIntInFixed && picoFloat == float64(picoInt64) {
		return Decimal{fixed: picoInt64}
	}

	fallback := decimal.NewFromFloat(f)
	return Decimal{fallback: &fallback}
}

func NewFromFloat32(f float32) Decimal {
	return NewFromFloat(float64(f))
}

func NewFromDecimal(d decimal.Decimal) Decimal {
	// TODO: if it's within optimization range
	// might cast to optimized version instead of fallback
	return Decimal{fallback: &d}
}

func (d Decimal) String() string {
	if d.fallback == nil {
		// "-9223372.000000000000" => max length = 21 bytes
		var s [21]byte
		start := 7
		end := 8

		var ufixed uint64
		if d.fixed >= 0 {
			ufixed = uint64(d.fixed)
		} else {
			ufixed = uint64(d.fixed * -1)
		}

		integerPart := ufixed / scale
		fractionalPart := ufixed % scale

		// integer part
		if integerPart == 0 {
			s[start] = '0'
		} else {
			for integerPart >= 10 {
				s[start] = byte(integerPart%10 + '0')
				start--
				integerPart /= 10
			}
			s[start] = byte(integerPart + '0')
		}

		// fractional part
		if fractionalPart > 0 {
			s[8] = '.'
			for i := 20; i > 8; i-- {
				is := fractionalPart % 10
				fractionalPart /= 10
				if is != 0 {
					s[i] = byte(is + '0')
					end = i + 1
					for j := i - 1; j > 8; j-- {
						s[j] = byte(fractionalPart%10 + '0')
						fractionalPart /= 10
					}
					break
				}
			}
		}

		// sign part
		if d.fixed < 0 {
			start -= 1
			s[start] = '-'
		}

		return string(s[start:end])
	}

	return d.fallback.String()
}

// sql support

// sql.Valuer interface
func (d Decimal) Value() (driver.Value, error) {
	if d.fallback == nil {
		// cache hit
		if d.fixed <= a1000InFixed && d.fixed >= aNeg1000InFixed && d.fixed%aCentInFixed == 0 {
			return valueCache[d.fixed/aCentInFixed+100000], nil
		}

		return d.String(), nil
	}

	return d.fallback.Value()
}

// sql.Scanner interface
func (d *Decimal) Scan(value interface{}) error {
	switch v := value.(type) {
	case float32:
		*d = NewFromFloat32(v)
		return nil

	case float64:
		*d = NewFromFloat(v)
		return nil

	case int64:
		*d = NewFromInt(v)
		return nil

	case []byte:
		fixed, ok := parseFixed(v)
		if ok {
			d.fixed = fixed
			return nil
		}

	case string:
		fixed, ok := parseFixed(v)
		if ok {
			d.fixed = fixed
			return nil
		}
	}

	var fallback decimal.Decimal
	if err := fallback.Scan(value); err != nil {
		return err
	}
	d.fallback = &fallback
	return nil
}

// common example: "0", "0.00", "0.001"
func parseFixed[T string | []byte](v T) (int64, bool) {
	// remove quotes if any
	if len(v) > 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}

	// max len of fixed is 21, e.g. -9_223_372.000_000_000_000
	if len(v) > 21 {
		return 0, false
	}

	// remove trailing '0' if any (e.g. "0.000")
	if len(v) > 1 && v[len(v)-1] == '0' {
		for _, c := range []byte(v) {
			if c == '.' {
				for len(v) > 0 && v[len(v)-1] == '0' {
					v = v[:len(v)-1]
				}
				break
			}
		}
	}

	// remove trailing '.' if any
	if len(v) > 1 && v[len(v)-1] == '.' {
		v = v[:len(v)-1]
	}

	negative := false
	if len(v) > 1 {
		switch v[0] {
		case '+':
			v = v[:len(v)-1]
		case '-':
			v = v[:len(v)-1]
			negative = true
		}

	}

	if len(v) == 0 {
		return 0, false
	}

	var fixed int64 = 0

	for i, c := range []byte(v) {
		if '0' <= c && c <= '9' {
			fixed *= 10
			fixed += int64(c - '0')
			if fixed >= maxInt {
				// out of range
				return 0, false
			}
		} else if c == '.' {
			// handle fractional part
			s := v[i+1:]
			if len(s) > 12 {
				// out of range
				return 0, false
			}
			for _, c := range []byte(s) {
				if '0' <= c && c <= '9' {
					fixed *= 10
					fixed += int64(c - '0')
				} else {
					// invalid case
					return 0, false
				}
			}
			fixed *= pow10Table[12-len(s)]
			if negative {
				return -fixed, true
			} else {
				return fixed, true
			}
		} else {
			// invalid case
			return 0, false
		}
	}
	// no fractional part
	if negative {
		return -fixed * scale, true
	} else {
		return fixed * scale, true
	}
}

// arithmatic operations

func (d Decimal) Add(d2 Decimal) Decimal {
	// if result of add is not overflow,
	// we can keep result as optimized format as well.
	// otherwise, we would need to fallback to decimal.Decimal
	if d.fallback == nil && d2.fallback == nil {
		// check overflow
		// based on https://stackoverflow.com/a/33643773
		if d2.fixed > 0 {
			if d.fixed <= maxIntInFixed-d2.fixed {
				return Decimal{fixed: d.fixed + d2.fixed}
			}
		} else {
			if d.fixed >= minIntInFixed-d2.fixed {
				return Decimal{fixed: d.fixed + d2.fixed}
			}
		}
	}

	result := d.asFallback().Add(d2.asFallback())

	return Decimal{fallback: &result}
}

func (d Decimal) asFallback() decimal.Decimal {
	if d.fallback == nil {
		x := big.NewInt(d.fixed)
		return decimal.NewFromBigInt(x, -12)
	}
	return *d.fallback
}

func (d Decimal) Mul(d2 Decimal) Decimal {

	if d.fallback == nil && d2.fallback == nil {
		fixed, ok := mul(d.fixed, d2.fixed)
		if ok {
			return Decimal{fixed: fixed}
		}
	}

	result := d.asFallback().Mul(d2.asFallback())

	return Decimal{fallback: &result}
}

func mul(x, y int64) (int64, bool) {
	if x == 0 || y == 0 {
		return 0, true
	}

	negative := false

	if x < 0 {
		x = -x
		negative = !negative
	}

	if y < 0 {
		y = -y
		negative = !negative
	}

	// x * y = (x_int + x_fractional) * (y_int + y_fractional)
	//       = x_int * y_int + x_int * y_fractional
	//       + x_fractional * y_fractional + x_fractional * y_fractional

	x_int := x / scale
	x_fractional := x % scale

	y_int := y / scale
	y_fractional := y % scale

	var result int64

	if x_int != 0 && y_int != 0 {
		z := x_int * y_int
		if z > maxInt {
			// out of range
			return 0, false
		}
		result = z * scale
	}

	if x_fractional != 0 && y_fractional != 0 {
		// x_fractional * y_fractional = x_fractional_a * y_fractional_a
		//                             + x_fractional_a * y_fractional_b
		//                             + x_fractional_b * y_fractional_a
		//                             + x_fractional_b * y_fractional_b
		x_fractional_a := x_fractional / 1000_000
		x_fractional_b := x_fractional % 1000_000
		y_fractional_a := y_fractional / 1000_000
		y_fractional_b := y_fractional % 1000_000

		s := x_fractional_a * y_fractional_a

		if x_fractional_b != 0 || y_fractional_b != 0 {
			p1 := x_fractional_a*y_fractional_b + x_fractional_b*y_fractional_a
			p2 := x_fractional_b * y_fractional_b

			if p1%1000_000 != 0 || p2%scale != 0 {
				// out of range
				return 0, false
			}

			s += p1/1000_000 + p2/scale
		}

		if result <= maxIntInFixed-s {
			result += s
		} else {
			// out of range
			return 0, false
		}
	}

	if x_int != 0 && y_fractional != 0 {
		p := x_int * y_fractional
		if result <= maxIntInFixed-p {
			result += p
		} else {
			// out of range
			return 0, false
		}
	}

	if x_fractional != 0 && y_int != 0 {
		p := x_fractional * y_int
		if result <= maxIntInFixed-p {
			result += p
		} else {
			// out of range
			return 0, false
		}
	}

	if negative {
		result *= -1
	}

	return result, true
}

func (d Decimal) Equal(d2 Decimal) bool {
	if d.fallback == nil && d2.fallback == nil {
		return d.fixed == d2.fixed
	}
	return d.asFallback().Equal(d2.asFallback())
}
