package alpacadecimal

import (
	"database/sql/driver"
	"math"
	"math/big"
	"regexp"
	"strconv"

	"github.com/shopspring/decimal"
)

// currently support 12 precision, this is tunnable,
// more precision => smaller maxInt
// less precision => bigger maxInt
const precision = 12
const scale = 1e12
const maxInt int64 = int64(math.MaxInt64) / scale
const minInt int64 = int64(math.MinInt64) / scale
const maxIntInFixed int64 = maxInt * scale
const minIntInFixed int64 = minInt * scale
const a1000InFixed int64 = 1000 * scale
const aNeg1000InFixed int64 = -1000 * scale
const aCentInFixed int64 = scale / 100

var pow10Table []int64 = []int64{
	1e0, 1e1, 1e2, 1e3, 1e4,
	1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14,
	1e15, 1e16, 1e17, 1e18,
}

// cache value from -1000.00 to 1000.00
// with
//
//	`valueCache[0] = "-1000"`
//	`valueCache[100000] = "0"`
//	`valueCache[200000] = "1000"`
//
// this consumes about 9 MB in memory with pprof check.
const cacheSize = 200001
const cacheOffset = 100000

var valueCache [cacheSize]driver.Value
var stringCache [cacheSize]string

func init() {
	// init cache
	for i := 0; i < cacheSize; i++ {
		str := strconv.FormatFloat(float64(i-cacheOffset)/100, 'f', -1, 64)

		valueCache[i] = str
		stringCache[i] = str
	}
}

// API

// APIs are marked as either "optimized" or "fallbacked"
// where "optimized" means that it's specially optimized
// where "fallback" means that it's not optimized and fallback from decimal.Decimal
// mostly due to lack of usage in Alpaca. we should be able to move "fallback" to "optimized" as needed.

// Variables
var DivisionPrecision = decimal.DivisionPrecision
var ExpMaxIterations = decimal.ExpMaxIterations
var MarshalJSONWithoutQuotes = decimal.MarshalJSONWithoutQuotes
var Zero = Decimal{fixed: 0}

func RescalePair(d1 Decimal, d2 Decimal) (Decimal, Decimal) {
	if d1.fallback == nil && d2.fallback == nil {
		return d1, d2
	}
	dd1, dd2 := decimal.RescalePair(d1.asFallback(), d2.asFallback())
	return newFromDecimal(dd1), newFromDecimal(dd2)
}

type Decimal struct {
	// represent decimal with 12 precision, 1.23 will have `fixed = 1_230_000_000_000`
	// max support decimal is 9_223_372.000_000_000_000
	// min support decimal is -9_223_372.000_000_000_000
	fixed int64

	// fallback to original decimal.Decimal if necessary
	fallback *decimal.Decimal
}

// optimized:
// Avg returns the average value of the provided first and rest Decimals
func Avg(first Decimal, rest ...Decimal) Decimal {
	return Sum(first, rest...).Div(NewFromInt(int64(1 + len(rest))))
}

// optimized:
// Max returns the largest Decimal that was passed in the arguments.
func Max(first Decimal, rest ...Decimal) Decimal {
	result := first
	for _, item := range rest {
		if item.GreaterThan(result) {
			result = item
		}
	}
	return result
}

// optimized:
// Min returns the smallest Decimal that was passed in the arguments.
func Min(first Decimal, rest ...Decimal) Decimal {
	result := first
	for _, item := range rest {
		if item.LessThan(result) {
			result = item
		}
	}
	return result
}

// optimized:
// New returns a new fixed-point decimal, value * 10 ^ exp.
func New(value int64, exp int32) Decimal {
	d, done := tryOptNew(value, exp)
	if done {
		return d
	}
	return newFromDecimal(decimal.New(value, exp))
}

func tryOptNew(value int64, exp int32) (Decimal, bool) {
	if exp >= -12 {
		if exp <= 0 {
			s := pow10Table[-exp]
			if value >= minInt*s && value <= maxInt*s {
				return Decimal{fixed: value * pow10Table[precision+exp]}, true
			}
		} else if exp <= 6 { // when exp > 6, it would be greater than maxInt
			s := pow10Table[exp]
			if value >= minInt/s && value <= maxInt/s {
				return Decimal{fixed: value * pow10Table[precision+exp]}, true
			}
		}
	}
	return Decimal{}, false
}

// fallback:
// NewFromBigInt returns a new Decimal from a big.Int, value * 10 ^ exp
func NewFromBigInt(value *big.Int, exp int32) Decimal {
	return newFromDecimal(decimal.NewFromBigInt(value, exp))
}

// optimized:
// NewFromFloat converts a float64 to Decimal.
//
// NOTE: this will panic on NaN, +/-inf
func NewFromFloat(f float64) Decimal {
	picoFloat := f * float64(scale)
	picoInt64 := int64(picoFloat)

	// check if it's within range and is whole number
	// integer overflow is accounted for via the `picoFloat == float64(picoInt64)` check
	if picoInt64 >= minIntInFixed && picoInt64 <= maxIntInFixed && picoFloat == float64(picoInt64) {
		return Decimal{fixed: picoInt64}
	}

	return newFromDecimal(decimal.NewFromFloat(f))
}

// fallback:
// NewFromFloat32 converts a float32 to Decimal.
//
// The converted number will contain the number of significant digits that can be
// represented in a float with reliable roundtrip.
// This is typically 6-8 digits depending on the input.
// See https://www.exploringbinary.com/decimal-precision-of-binary-floating-point-numbers/ for more information.
//
// For slightly faster conversion, use NewFromFloatWithExponent where you can specify the precision in absolute terms.
//
// NOTE: this will panic on NaN, +/-inf
func NewFromFloat32(f float32) Decimal {
	return newFromDecimal(decimal.NewFromFloat32(f))
}

// fallback:
// NewFromFloatWithExponent converts a float64 to Decimal, with an arbitrary
// number of fractional digits.
//
// Example:
//
//	NewFromFloatWithExponent(123.456, -2).String() // output: "123.46"
func NewFromFloatWithExponent(value float64, exp int32) Decimal {
	return newFromDecimal(decimal.NewFromFloatWithExponent(value, exp))
}

// fallback:
// NewFromFormattedString returns a new Decimal from a formatted string representation.
// The second argument - replRegexp, is a regular expression that is used to find characters that should be
// removed from given decimal string representation. All matched characters will be replaced with an empty string.
func NewFromFormattedString(value string, replRegexp *regexp.Regexp) (Decimal, error) {
	d, err := decimal.NewFromFormattedString(value, replRegexp)
	if err != nil {
		return Zero, err
	}
	return newFromDecimal(d), nil
}

// optimized:
// NewFromInt converts a int64 to Decimal.
func NewFromInt(x int64) Decimal {
	if x >= minInt && x <= maxInt {
		return Decimal{fixed: x * scale}
	}
	return newFromDecimal(decimal.NewFromInt(x))
}

// optimized:
// NewFromInt32 converts a int32 to Decimal.
func NewFromInt32(value int32) Decimal {
	return NewFromInt(int64(value))
}

// optimized:
// NewFromString returns a new Decimal from a string representation.
func NewFromString(value string) (Decimal, error) {
	if fixed, ok := parseFixed(value); ok {
		return Decimal{fixed: fixed}, nil
	}

	// fallback
	d, err := decimal.NewFromString(value)
	if err != nil {
		return Zero, err
	}
	return newFromDecimal(d), nil
}

// optimized:
// RequireFromString returns a new Decimal from a string representation
// or panics if NewFromString would have returned an error.
func RequireFromString(value string) Decimal {
	d, err := NewFromString(value)
	if err != nil {
		panic(err)
	}
	return d
}

// optimized:
// Sum returns the combined total of the provided first and rest Decimals
func Sum(first Decimal, rest ...Decimal) Decimal {
	result := first
	for _, item := range rest {
		result = result.Add(item)
	}
	return result
}

// optimized:
// Abs returns the absolute value of the decimal.
func (d Decimal) Abs() Decimal {
	if d.fallback == nil {
		if d.fixed >= 0 {
			return d
		} else {
			return Decimal{fixed: -d.fixed}
		}
	}
	return newFromDecimal(d.fallback.Abs())
}

// optimized:
// Add returns d + d2.
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

	return newFromDecimal(d.asFallback().Add(d2.asFallback()))
}

// fallback:
// Atan returns the arctangent, in radians, of x.
func (d Decimal) Atan() Decimal {
	return newFromDecimal(d.asFallback().Atan())
}

// fallback:
// BigFloat returns decimal as BigFloat.
func (d Decimal) BigFloat() *big.Float {
	return d.asFallback().BigFloat()
}

// fallback:
// BigInt returns integer component of the decimal as a BigInt.
func (d Decimal) BigInt() *big.Int {
	return d.asFallback().BigInt()
}

// optimized:
// Ceil returns the nearest integer value greater than or equal to d.
func (d Decimal) Ceil() Decimal {
	if d.fallback == nil {
		m := d.fixed % scale
		if m == 0 {
			return Decimal{fixed: d.fixed}
		}
		if m > 0 {
			return Decimal{fixed: d.fixed - m + scale}
		}
		return Decimal{fixed: d.fixed - m}
	}
	return newFromDecimal(d.asFallback().Ceil())
}

// optimized:
// Cmp compares the numbers represented by d and d2 and returns:
//
//	-1 if d <  d2
//	 0 if d == d2
//	+1 if d >  d2
func (d Decimal) Cmp(d2 Decimal) int {
	if d.fallback == nil && d2.fallback == nil {
		switch {
		case d.fixed < d2.fixed:
			return -1
		case d.fixed == d2.fixed:
			return 0
		default:
			return 1
		}
	}
	return d.asFallback().Cmp(d2.asFallback())
}

// optimized:
// Coefficient returns the coefficient of the decimal. It is scaled by 10^Exponent()
func (d Decimal) Coefficient() *big.Int {
	if d.fallback == nil {
		return big.NewInt(d.fixed)
	}
	return d.asFallback().Coefficient()
}

// optimized:
// CoefficientInt64 returns the coefficient of the decimal as int64. It is scaled by 10^Exponent()
func (d Decimal) CoefficientInt64() int64 {
	if d.fallback == nil {
		return d.fixed
	}
	return d.asFallback().CoefficientInt64()
}

// optimized:
// Copy returns a copy of decimal with the same value and exponent, but a different pointer to value.
func (d Decimal) Copy() Decimal {
	if d.fallback == nil {
		return Decimal{fixed: d.fixed}
	}
	return newFromDecimal(d.fallback.Copy())
}

// fallback:
// Cos returns the cosine of the radian argument x.
func (d Decimal) Cos() Decimal {
	return newFromDecimal(d.asFallback().Cos())
}

// optimized:
// Div returns d / d2. If it doesn't divide exactly, the result will have
// DivisionPrecision digits after the decimal point.
func (d Decimal) Div(d2 Decimal) Decimal {
	if d.fallback == nil && d2.fallback == nil {
		fixed, ok := div(d.fixed, d2.fixed)
		if ok {
			return Decimal{fixed: fixed}
		}
	}
	return d.DivRound(d2, int32(DivisionPrecision))
}

// fallback:
// DivRound divides and rounds to a given precision
func (d Decimal) DivRound(d2 Decimal, precision int32) Decimal {
	return newFromDecimal(d.asFallback().DivRound(d2.asFallback(), precision))
}

// optimized:
// Equal returns whether the numbers represented by d and d2 are equal.
func (d Decimal) Equal(d2 Decimal) bool {
	if d.fallback == nil && d2.fallback == nil {
		return d.fixed == d2.fixed
	}
	return d.asFallback().Equal(d2.asFallback())
}

// fallback:
// Equals is deprecated, please use Equal method instead
func (d Decimal) Equals(d2 Decimal) bool {
	return d.Equal(d2)
}

// fallback:
// ExpHullAbrham calculates the natural exponent of decimal (e to the power of d) using Hull-Abraham algorithm.
// OverallPrecision argument specifies the overall precision of the result (integer part + decimal part).
func (d Decimal) ExpHullAbrham(overallPrecision uint32) (Decimal, error) {
	dec, err := d.asFallback().ExpHullAbrham(overallPrecision)
	if err != nil {
		return Zero, err
	}
	return newFromDecimal(dec), nil
}

// fallback:
// ExpTaylor calculates the natural exponent of decimal (e to the power of d) using Taylor series expansion.
// Precision argument specifies how precise the result must be (number of digits after decimal point).
// Negative precision is allowed.
func (d Decimal) ExpTaylor(precision int32) (Decimal, error) {
	dec, err := d.asFallback().ExpTaylor(precision)
	if err != nil {
		return Zero, err
	}
	return newFromDecimal(dec), nil
}

// optimized:
// Exponent returns the exponent, or scale component of the decimal.
func (d Decimal) Exponent() int32 {
	if d.fallback == nil {
		return -precision
	}
	return d.fallback.Exponent()
}

// fallback:
// Float64 returns the nearest float64 value for d and a bool indicating
// whether f represents d exactly.
func (d Decimal) Float64() (f float64, exact bool) {
	return d.asFallback().Float64()
}

// optimized:
// Floor returns the nearest integer value less than or equal to d.
func (d Decimal) Floor() Decimal {
	if d.fallback == nil {
		m := d.fixed % scale
		if m == 0 {
			return Decimal{fixed: d.fixed}
		}
		if m > 0 {
			return Decimal{fixed: d.fixed - m}
		}
		return Decimal{fixed: d.fixed - m - scale}
	}
	return newFromDecimal(d.asFallback().Floor())
}

// fallback: (can be optimized if needed)
func (d *Decimal) GobDecode(data []byte) error {
	return d.UnmarshalBinary(data)
}

// fallback: (can be optimized if needed)
func (d Decimal) GobEncode() ([]byte, error) {
	return d.MarshalBinary()
}

// optimized:
// GreaterThan (GT) returns true when d is greater than d2.
func (d Decimal) GreaterThan(d2 Decimal) bool {
	if d.fallback == nil && d2.fallback == nil {
		return d.fixed > d2.fixed
	}
	return d.asFallback().GreaterThan(d2.asFallback())
}

// optimized:
// GreaterThanOrEqual (GTE) returns true when d is greater than or equal to d2.
func (d Decimal) GreaterThanOrEqual(d2 Decimal) bool {
	if d.fallback == nil && d2.fallback == nil {
		return d.fixed >= d2.fixed
	}
	return d.asFallback().GreaterThanOrEqual(d2.asFallback())
}

// fallback:
// InexactFloat64 returns the nearest float64 value for d.
// It doesn't indicate if the returned value represents d exactly.
func (d Decimal) InexactFloat64() float64 {
	return d.asFallback().InexactFloat64()
}

// optimized:
// IntPart returns the integer component of the decimal.
func (d Decimal) IntPart() int64 {
	if d.fallback == nil {
		return d.fixed / scale
	}
	return d.fallback.IntPart()
}

// optimized:
// IsInteger returns true when decimal can be represented as an integer value, otherwise, it returns false.
func (d Decimal) IsInteger() bool {
	if d.fallback == nil {
		return d.fixed%scale == 0
	}
	return d.fallback.IsInteger()
}

// optimized:
// IsNegative return
//
//	true if d < 0
//	false if d == 0
//	false if d > 0
func (d Decimal) IsNegative() bool {
	if d.fallback == nil {
		return d.fixed < 0
	}
	return d.fallback.IsNegative()
}

// optimized:
// IsPositive return
//
//	true if d > 0
//	false if d == 0
//	false if d < 0
func (d Decimal) IsPositive() bool {
	if d.fallback == nil {
		return d.fixed > 0
	}
	return d.fallback.IsPositive()
}

// optimized:
// IsZero return
//
//	true if d == 0
//	false if d > 0
//	false if d < 0
func (d Decimal) IsZero() bool {
	if d.fallback == nil {
		return d.fixed == 0
	}
	return d.fallback.IsZero()
}

// optimized:
// LessThan (LT) returns true when d is less than d2.
func (d Decimal) LessThan(d2 Decimal) bool {
	if d.fallback == nil && d2.fallback == nil {
		return d.fixed < d2.fixed
	}
	return d.asFallback().LessThan(d2.asFallback())
}

// optimized:
// LessThanOrEqual (LTE) returns true when d is less than or equal to d2.
func (d Decimal) LessThanOrEqual(d2 Decimal) bool {
	if d.fallback == nil && d2.fallback == nil {
		return d.fixed <= d2.fixed
	}
	return d.asFallback().LessThanOrEqual(d2.asFallback())
}

// fallback:
// MarshalBinary implements the encoding.BinaryMarshaler interface.
func (d Decimal) MarshalBinary() (data []byte, err error) {
	return d.asFallback().MarshalBinary()
}

// optimized:
func (d Decimal) MarshalJSON() ([]byte, error) {
	var str string
	if MarshalJSONWithoutQuotes {
		str = d.String()
	} else {
		str = "\"" + d.String() + "\""
	}
	return []byte(str), nil
}

// optimized:
func (d Decimal) MarshalText() (text []byte, err error) {
	return []byte(d.String()), nil
}

func (d Decimal) Mod(d2 Decimal) Decimal {
	return newFromDecimal(d.asFallback().Mod(d2.asFallback()))
}

// optimized:
// Mul returns d * d2
func (d Decimal) Mul(d2 Decimal) Decimal {
	if d.fallback == nil && d2.fallback == nil {
		fixed, ok := mul(d.fixed, d2.fixed)
		if ok {
			return Decimal{fixed: fixed}
		}
	}
	return newFromDecimal(d.asFallback().Mul(d2.asFallback()))
}

// optimized:
// Neg returns -d
func (d Decimal) Neg() Decimal {
	if d.fallback == nil {
		return Decimal{fixed: -d.fixed}
	}
	return newFromDecimal(d.fallback.Neg())
}

// fallback:
// NumDigits returns the number of digits of the decimal coefficient (d.Value)
func (d Decimal) NumDigits() int {
	return d.asFallback().NumDigits()
}

// fallback:
// Pow returns d to the power d2
func (d Decimal) Pow(d2 Decimal) Decimal {
	return newFromDecimal(d.asFallback().Pow(d2.asFallback()))
}

// fallback:
// QuoRem does divsion with remainder
func (d Decimal) QuoRem(d2 Decimal, precision int32) (Decimal, Decimal) {
	x, y := d.asFallback().QuoRem(d2.asFallback(), precision)
	return newFromDecimal(x), newFromDecimal(y)
}

// fallback:
// Rat returns a rational number representation of the decimal.
func (d Decimal) Rat() *big.Rat {
	return d.asFallback().Rat()
}

// optimized:
// Round rounds the decimal to places decimal places.
// If places < 0, it will round the integer part to the nearest 10^(-places).
func (d Decimal) Round(places int32) Decimal {
	if d.fallback == nil {
		if places >= precision {
			// no need to round
			return d
		}
		if places >= 0 {
			s := pow10Table[precision-places]
			m := d.fixed % s
			if m == 0 {
				// no need to round
				return d
			}

			if m > 0 {
				if m*2 >= s {
					return Decimal{fixed: d.fixed - m + s}
				} else {
					return Decimal{fixed: d.fixed - m}
				}
			} else {
				if -m*2 >= s {
					return Decimal{fixed: d.fixed - m - s}
				} else {
					return Decimal{fixed: d.fixed - m}
				}
			}
		}
	}
	return newFromDecimal(d.asFallback().Round(places))
}

// fallback:
// RoundBank rounds the decimal to places decimal places.
// If the final digit to round is equidistant from the nearest two integers the
// rounded value is taken as the even number
//
// If places < 0, it will round the integer part to the nearest 10^(-places).
func (d Decimal) RoundBank(places int32) Decimal {
	return newFromDecimal(d.asFallback().RoundBank(places))
}

// fallback:
// RoundCash aka Cash/Penny/öre rounding rounds decimal to a specific
// interval. The amount payable for a cash transaction is rounded to the nearest
// multiple of the minimum currency unit available. The following intervals are
// available: 5, 10, 25, 50 and 100; any other number throws a panic.
//
//	  5:   5 cent rounding 3.43 => 3.45
//	 10:  10 cent rounding 3.45 => 3.50 (5 gets rounded up)
//	 25:  25 cent rounding 3.41 => 3.50
//	 50:  50 cent rounding 3.75 => 4.00
//	100: 100 cent rounding 3.50 => 4.00
//
// For more details: https://en.wikipedia.org/wiki/Cash_rounding
func (d Decimal) RoundCash(interval uint8) Decimal {
	return newFromDecimal(d.asFallback().RoundCash(interval))
}

// fallback:
// RoundCeil rounds the decimal towards +infinity.
//
// Example:
//
//	NewFromFloat(545).RoundCeil(-2).String()   // output: "600"
//	NewFromFloat(500).RoundCeil(-2).String()   // output: "500"
//	NewFromFloat(1.1001).RoundCeil(2).String() // output: "1.11"
//	NewFromFloat(-1.454).RoundCeil(1).String() // output: "-1.5"
func (d Decimal) RoundCeil(places int32) Decimal {
	return newFromDecimal(d.asFallback().RoundCeil(places))
}

// fallback:
// RoundDown rounds the decimal towards zero.
//
// Example:
//
//	NewFromFloat(545).RoundDown(-2).String()   // output: "500"
//	NewFromFloat(-500).RoundDown(-2).String()   // output: "-500"
//	NewFromFloat(1.1001).RoundDown(2).String() // output: "1.1"
//	NewFromFloat(-1.454).RoundDown(1).String() // output: "-1.5"
func (d Decimal) RoundDown(places int32) Decimal {
	return newFromDecimal(d.asFallback().RoundDown(places))
}

// fallback:
// RoundFloor rounds the decimal towards -infinity.
//
// Example:
//
//	NewFromFloat(545).RoundFloor(-2).String()   // output: "500"
//	NewFromFloat(-500).RoundFloor(-2).String()   // output: "-500"
//	NewFromFloat(1.1001).RoundFloor(2).String() // output: "1.1"
//	NewFromFloat(-1.454).RoundFloor(1).String() // output: "-1.4"
func (d Decimal) RoundFloor(places int32) Decimal {
	return newFromDecimal(d.asFallback().RoundFloor(places))
}

// fallback:
// RoundUp rounds the decimal away from zero.
//
// Example:
//
//	NewFromFloat(545).RoundUp(-2).String()   // output: "600"
//	NewFromFloat(500).RoundUp(-2).String()   // output: "500"
//	NewFromFloat(1.1001).RoundUp(2).String() // output: "1.11"
//	NewFromFloat(-1.454).RoundUp(1).String() // output: "-1.4"
func (d Decimal) RoundUp(places int32) Decimal {
	return newFromDecimal(d.asFallback().RoundUp(places))
}

// optimized:
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
			d.fallback = nil
			return nil
		}

	case string:
		fixed, ok := parseFixed(v)
		if ok {
			d.fixed = fixed
			d.fallback = nil
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

// fallback:
// Binary shift left (k > 0) or right (k < 0).
func (d Decimal) Shift(shift int32) Decimal {
	return newFromDecimal(d.asFallback().Shift(shift))
}

// optimized:
// Sign returns:
//
//	-1 if d <  0
//	 0 if d == 0
//	+1 if d >  0
func (d Decimal) Sign() int {
	if d.fallback == nil {
		if d.fixed > 0 {
			return 1
		}
		if d.fixed < 0 {
			return -1
		}
		return 0
	}
	return d.asFallback().Sign()
}

// fallback:
// Sin returns the sine of the radian argument x.
func (d Decimal) Sin() Decimal {
	return newFromDecimal(d.asFallback().Sin())
}

// optimized:
// String returns the string representation of the decimal
// with the fixed point.
func (d Decimal) String() string {
	if d.fallback == nil {
		// cache hit
		if d.fixed <= a1000InFixed && d.fixed >= aNeg1000InFixed && d.fixed%aCentInFixed == 0 {
			return stringCache[d.fixed/aCentInFixed+cacheOffset]
		}

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

// fallback:
// StringFixed returns a rounded fixed-point string with places digits after
// the decimal point.
func (d Decimal) StringFixed(places int32) string {
	return d.asFallback().StringFixed(places)
}

// fallback:
// StringFixedBank returns a banker rounded fixed-point string with places digits
// after the decimal point.
func (d Decimal) StringFixedBank(places int32) string {
	return d.asFallback().StringFixedBank(places)
}

// fallback:
// StringFixedCash returns a Swedish/Cash rounded fixed-point string. For
// more details see the documentation at function RoundCash.
func (d Decimal) StringFixedCash(interval uint8) string {
	return d.asFallback().StringFixedCash(interval)
}

// fallback:
// DEPRECATED! Use StringFixed instead.
func (d Decimal) StringScaled(exp int32) string {
	return d.asFallback().StringScaled(exp)
}

// optimized:
// Sub returns d - d2.
func (d Decimal) Sub(d2 Decimal) Decimal {
	return d.Add(d2.Neg())
}

// fallback:
// Tan returns the tangent of the radian argument x.
func (d Decimal) Tan() Decimal {
	return newFromDecimal(d.asFallback().Tan())
}

// optimized:
// Truncate truncates off digits from the number, without rounding.
func (d Decimal) Truncate(precision int32) Decimal {
	if d.fallback == nil {
		s := pow10Table[12-precision]
		return Decimal{fixed: d.fixed / s * s}
	}
	return newFromDecimal(d.asFallback().Truncate(precision))
}

// fallback:
// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface. As a string representation
// is already used when encoding to text, this method stores that string as []byte
func (d *Decimal) UnmarshalBinary(data []byte) error {
	var dd decimal.Decimal
	if err := dd.UnmarshalBinary(data); err != nil {
		return err
	}
	ddd := newFromDecimal(dd)
	d.fixed = ddd.fixed
	d.fallback = ddd.fallback
	return nil
}

// optimized:
// UnmarshalJSON implements the json.Unmarshaler interface.
func (d *Decimal) UnmarshalJSON(decimalBytes []byte) error {
	if fixed, ok := parseFixed(decimalBytes); ok {
		d.fixed = fixed
		d.fallback = nil
		return nil
	}

	var fallback decimal.Decimal
	if err := fallback.UnmarshalJSON(decimalBytes); err != nil {
		return err
	}
	d.fallback = &fallback
	return nil
}

// optimized:
// UnmarshalText implements the encoding.TextUnmarshaler interface for XML
// deserialization.
func (d *Decimal) UnmarshalText(text []byte) error {
	if fixed, ok := parseFixed(text); ok {
		d.fixed = fixed
		d.fallback = nil
		return nil
	}

	var dd decimal.Decimal
	if err := dd.UnmarshalText(text); err != nil {
		return err
	}
	ddd := newFromDecimal(dd)
	d.fixed = ddd.fixed
	d.fallback = ddd.fallback
	return nil
}

// optimized:
// sql.Valuer interface
func (d Decimal) Value() (driver.Value, error) {
	if d.fallback == nil {
		// cache hit
		if d.fixed <= a1000InFixed && d.fixed >= aNeg1000InFixed && d.fixed%aCentInFixed == 0 {
			return valueCache[d.fixed/aCentInFixed+cacheOffset], nil
		}

		return d.String(), nil
	}

	return d.fallback.Value()
}

// Extra API to support get internal state.
// e.g. might be useful for flatbuffers encode / decode.
func (d Decimal) GetFixed() int64 {
	return d.fixed
}

func (d Decimal) GetFallback() *decimal.Decimal {
	return d.fallback
}

func (d Decimal) IsOptimized() bool {
	return d.fallback == nil
}

// NullDecimal support
type NullDecimal struct {
	Decimal Decimal
	Valid   bool
}

func NewNullDecimal(d Decimal) NullDecimal {
	return NullDecimal{
		Decimal: d,
		Valid:   true,
	}
}

func (d NullDecimal) MarshalJSON() ([]byte, error) {
	if !d.Valid {
		return []byte("null"), nil
	}
	return d.Decimal.MarshalJSON()
}

func (d NullDecimal) MarshalText() (text []byte, err error) {
	if !d.Valid {
		return []byte{}, nil
	}
	return d.Decimal.MarshalText()
}

func (d *NullDecimal) Scan(value interface{}) error {
	if value == nil {
		d.Valid = false
		return nil
	}
	d.Valid = true
	return d.Decimal.Scan(value)
}

func (d *NullDecimal) UnmarshalJSON(decimalBytes []byte) error {
	if string(decimalBytes) == "null" {
		d.Valid = false
		return nil
	}
	d.Valid = true
	return d.Decimal.UnmarshalJSON(decimalBytes)
}

func (d *NullDecimal) UnmarshalText(text []byte) error {
	str := string(text)

	// check for empty XML or XML without body e.g., <tag></tag>
	if str == "" {
		d.Valid = false
		return nil
	}

	if err := d.Decimal.UnmarshalText(text); err != nil {
		d.Valid = false
		return err
	}

	d.Valid = true
	return nil
}

func (d NullDecimal) Value() (driver.Value, error) {
	if !d.Valid {
		return nil, nil
	}
	return d.Decimal.Value()
}

// optimized:
// Create a new alpacadecimal.Decimal from a decimal.Decimal.
// Attempts to set the fixed value if possible.
func NewFromDecimal(d decimal.Decimal) Decimal {
	co := d.Coefficient()
	if !co.IsInt64() {
		return newFromDecimal(d) // fallback
	}
	value := co.Int64()
	exp := d.Exponent()
	res, done := tryOptNew(value, exp)
	if done {
		return res
	}
	return newFromDecimal(d)
}

// internal implementation
func newFromDecimal(d decimal.Decimal) Decimal {
	return Decimal{fallback: &d}
}

// sql support

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
			v = v[1:]
		case '-':
			v = v[1:]
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

func (d Decimal) asFallback() decimal.Decimal {
	if d.fallback == nil {
		return decimal.New(d.fixed, -precision)
	}
	return *d.fallback
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

func div(x, y int64) (int64, bool) {
	if x == 0 {
		return 0, y != 0
	}

	fz := float64(x) / float64(y)
	z := int64(fz * scale)

	// this `mul` check is to ensure we do not
	// lose precision from previous float64 operations.
	if xx, ok := mul(y, z); ok && x == xx {
		return z, true
	} else {
		return 0, false
	}
}
