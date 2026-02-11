package alpacadecimal

import (
	"database/sql/driver"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/quagmt/udecimal"
)

// currently support 12 precision, this is tunnable,
// more precision => smaller maxInt
// less precision => bigger maxInt
const (
	precision                        = 12
	scale                            = 1e12
	maxInt                     int64 = int64(math.MaxInt64) / scale
	minInt                     int64 = int64(math.MinInt64) / scale
	maxIntInFixed              int64 = maxInt * scale
	minIntInFixed              int64 = minInt * scale
	a1000InFixed               int64 = 1000 * scale
	aNeg1000InFixed            int64 = -1000 * scale
	aCentInFixed               int64 = scale / 100
	maxRoundUpThresholdInFixed int64 = 9 * 1e18  // 900_000_000_000_000_000
	minRoundUpThresholdInFixed int64 = -9 * 1e18 // -900_000_000_000_000_000
)

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
const (
	cacheSize   = 200001
	cacheOffset = 100000
)

var (
	valueCache  [cacheSize]driver.Value
	stringCache [cacheSize]string
)

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
// where "fallback" means that it's not optimized and fallback from udecimal.Decimal
// mostly due to lack of usage in Alpaca. we should be able to move "fallback" to "optimized" as needed.

// Variables
var (
	DivisionPrecision        = 16
	MarshalJSONWithoutQuotes = false
	Zero                     = Decimal{fixed: 0}
)

type Decimal struct {
	// fallback to udecimal.Decimal if necessary
	fallback *udecimal.Decimal

	// represent decimal with 12 precision, 1.23 will have `fixed = 1_230_000_000_000`
	// max support decimal is 9_223_372.000_000_000_000
	// min support decimal is -9_223_372.000_000_000_000
	fixed int64
}

// optimized:
// Avg returns the average value of the provided first and rest Decimals
func Avg(first Decimal, rest ...Decimal) Decimal {
	divisor := NewFromInt(int64(1 + len(rest)))
	sum := first.Div(divisor)
	for _, item := range rest {
		sum = sum.Add(item.Div(divisor))
	}
	return sum
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
	d, ok := tryOptNew(value, exp)
	if ok {
		return d
	}

	// Common case: exp in [-19, 0] maps directly to NewFromInt64(value, -exp)
	if exp <= 0 && exp >= -19 {
		fb, err := udecimal.NewFromInt64(value, uint8(-exp))
		if err != nil {
			panic(fmt.Sprintf("alpacadecimal.New: %v", err))
		}
		return newFromFallback(fb)
	}

	// Rare: positive exp or exp < -19.
	// Build value and 10^exp as udecimals, then multiply.
	fbValue, err := udecimal.NewFromInt64(value, 0)
	if err != nil {
		panic(fmt.Sprintf("alpacadecimal.New: %v", err))
	}
	ten, _ := udecimal.NewFromInt64(10, 0)
	fbScale, err := ten.PowInt32(exp)
	if err != nil {
		panic(fmt.Sprintf("alpacadecimal.New: %v", err))
	}
	return newFromFallback(fbValue.Mul(fbScale))
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
	s := formatBigIntWithExp(value, exp)
	fb, err := udecimal.Parse(s)
	if err != nil {
		panic(fmt.Sprintf("alpacadecimal.NewFromBigInt: failed to parse %q: %v", s, err))
	}
	return newFromFallback(fb)
}

// optimized:
// NewFromFloat converts a float64 to Decimal.
//
// NOTE: this will panic on NaN, +/-inf
func NewFromFloat(f float64) Decimal {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		panic(fmt.Sprintf("alpacadecimal.NewFromFloat: cannot create Decimal from %v", f))
	}
	// Convert float to string to avoid precision issues.
	// If the minimal representation exceeds 19 fractional digits
	// (udecimal's max precision), re-format with rounding to 19 places.
	str := strconv.FormatFloat(f, 'f', -1, 64)
	if dotIdx := strings.IndexByte(str, '.'); dotIdx >= 0 && len(str)-dotIdx-1 > 19 {
		str = strconv.FormatFloat(f, 'f', 19, 64)
	}
	d, err := NewFromString(str)
	if err != nil {
		panic(fmt.Sprintf("alpacadecimal.NewFromFloat: %v", err))
	}
	return d
}

// fallback:
// NewFromFloat32 converts a float32 to Decimal.
//
// NOTE: this will panic on NaN, +/-inf
func NewFromFloat32(f float32) Decimal {
	// Use 32-bit precision to avoid float32→float64 artifacts
	s := strconv.FormatFloat(float64(f), 'f', -1, 32)
	d, err := NewFromString(s)
	if err != nil {
		panic(fmt.Sprintf("alpacadecimal.NewFromFloat32: %v", err))
	}
	return d
}

// fallback:
// NewFromFloatWithExponent converts a float64 to Decimal, with an arbitrary
// number of fractional digits.
//
// Example:
//
//	NewFromFloatWithExponent(123.456, -2).String() // output: "123.46"
func NewFromFloatWithExponent(value float64, exp int32) Decimal {
	return NewFromFloat(value).Truncate(-exp)
}

// fallback:
// NewFromFormattedString returns a new Decimal from a formatted string representation.
// The second argument - replRegexp, is a regular expression that is used to find characters that should be
// removed from given decimal string representation. All matched characters will be replaced with an empty string.
func NewFromFormattedString(value string, replRegexp *regexp.Regexp) (Decimal, error) {
	cleaned := replRegexp.ReplaceAllString(value, "")
	return NewFromString(cleaned)
}

// optimized:
// NewFromInt converts a int64 to Decimal.
func NewFromInt(x int64) Decimal {
	if x >= minInt && x <= maxInt {
		return Decimal{fixed: x * scale}
	}
	fb, err := udecimal.NewFromInt64(x, 0)
	if err != nil {
		panic(fmt.Sprintf("alpacadecimal.NewFromInt: %v", err))
	}
	return newFromFallback(fb)
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

	// udecimal supports at most 19 fractional digits.
	// Truncate excess fractional digits so we don't reject valid inputs.
	v := value
	if dotIdx := strings.IndexByte(v, '.'); dotIdx >= 0 && len(v)-dotIdx-1 > 19 {
		v = v[:dotIdx+1+19]
	}

	// fallback
	d, err := udecimal.Parse(v)
	if err != nil {
		return Zero, fmt.Errorf("can't convert %s to decimal: %w", value, err)
	}
	return newFromFallback(d), nil
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
	return newFromFallback(d.fallback.Abs())
}

// optimized:
// Add returns d + d2.
func (d Decimal) Add(d2 Decimal) Decimal {
	// if result of add is not overflow,
	// we can keep result as optimized format as well.
	// otherwise, we would need to fallback to udecimal.Decimal
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

	return newFromFallback(d.asFallback().Add(d2.asFallback()))
}

// fallback:
// BigFloat returns decimal as BigFloat.
func (d Decimal) BigFloat() *big.Float {
	bf, _, _ := new(big.Float).Parse(d.String(), 10)
	return bf
}

// fallback:
// BigInt returns integer component of the decimal as a BigInt.
func (d Decimal) BigInt() *big.Int {
	s := d.Truncate(0).String()
	// Remove any decimal point (e.g. "123." or "123.000" after truncation)
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		s = s[:idx]
	}
	bi := new(big.Int)
	bi.SetString(s, 10)
	return bi
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
	return newFromFallback(d.asFallback().Ceil())
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
	// For fallback, compute coefficient from string.
	// The coefficient must satisfy: value = coefficient * 10^Exponent().
	// Exponent() returns -PrecUint(), so we need exactly PrecUint() fractional
	// digits in the coefficient computation. String() may trim trailing zeros,
	// so we must pad to match.
	s := d.fallback.String()
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg = true
		s = s[1:]
	}
	prec := int(d.fallback.PrecUint())
	// Count actual fractional digits in the string
	actualFrac := 0
	dotIdx := strings.IndexByte(s, '.')
	if dotIdx >= 0 {
		actualFrac = len(s) - dotIdx - 1
		s = s[:dotIdx] + s[dotIdx+1:]
	}
	// Pad with trailing zeros if String() trimmed them
	if actualFrac < prec {
		s += strings.Repeat("0", prec-actualFrac)
	}
	// Remove leading zeros
	s = strings.TrimLeft(s, "0")
	if s == "" {
		s = "0"
	}
	bi := new(big.Int)
	bi.SetString(s, 10)
	if neg {
		bi.Neg(bi)
	}
	return bi
}

// optimized:
// CoefficientInt64 returns the coefficient of the decimal as int64. It is scaled by 10^Exponent()
func (d Decimal) CoefficientInt64() int64 {
	if d.fallback == nil {
		return d.fixed
	}
	return d.Coefficient().Int64()
}

// optimized:
// Copy returns a copy of decimal with the same value and exponent, but a different pointer to value.
func (d Decimal) Copy() Decimal {
	if d.fallback == nil {
		return Decimal{fixed: d.fixed}
	}
	cp := *d.fallback
	return Decimal{fallback: &cp}
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
func (d Decimal) DivRound(d2 Decimal, prec int32) Decimal {
	fb1 := d.asFallback()
	fb2 := d2.asFallback()
	if fb2.IsZero() {
		panic("decimal division by zero")
	}

	// Parse both into big.Int coefficients and their decimal precisions
	num, p1 := parseToBigIntAndPrec(fb1.String())
	den, p2 := parseToBigIntAndPrec(fb2.String())

	// We want: result = (num / 10^p1) / (den / 10^p2) rounded to `prec` decimal places
	//        = num * 10^(p2 - p1) / den, then round to `prec` places
	// To get prec+1 digits for rounding: multiply num by 10^(prec + 1 + p2 - p1)
	scaleExp := int64(prec) + 1 + int64(p2) - int64(p1)
	if scaleExp > 0 {
		scaleFactor := new(big.Int).Exp(big.NewInt(10), big.NewInt(scaleExp), nil)
		num.Mul(num, scaleFactor)
	} else if scaleExp < 0 {
		scaleFactor := new(big.Int).Exp(big.NewInt(10), big.NewInt(-scaleExp), nil)
		den.Mul(den, scaleFactor)
	}

	q, _ := new(big.Int).QuoRem(num, den, new(big.Int))

	// q now has prec+1 implicit decimal digits. Round the last digit (half away from zero).
	isNeg := q.Sign() < 0
	aq := new(big.Int).Abs(q)
	lastDigit := new(big.Int).Mod(aq, big.NewInt(10)).Int64()
	aq.Div(aq, big.NewInt(10))
	if lastDigit >= 5 {
		aq.Add(aq, big.NewInt(1))
	}
	if isNeg {
		aq.Neg(aq)
	}

	return bigIntToDecimalWithPrec(aq, prec)
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

// optimized:
// Exponent returns the exponent, or scale component of the decimal.
func (d Decimal) Exponent() int32 {
	if d.fallback == nil {
		return -precision
	}
	return -int32(d.fallback.PrecUint())
}

// fallback:
// Float64 returns the nearest float64 value for d and a bool indicating
// whether f represents d exactly.
func (d Decimal) Float64() (f float64, exact bool) {
	f = d.InexactFloat64()
	// Check round-trip
	str := strconv.FormatFloat(f, 'f', -1, 64)
	roundTrip, err := NewFromString(str)
	if err != nil {
		return f, false
	}
	return f, roundTrip.Equal(d)
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
	return newFromFallback(d.asFallback().Floor())
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
	f, _ := strconv.ParseFloat(d.String(), 64)
	return f
}

// optimized:
// IntPart returns the integer component of the decimal.
func (d Decimal) IntPart() int64 {
	if d.fallback == nil {
		return d.fixed / scale
	}
	v, err := d.fallback.Int64()
	if err != nil {
		// Overflow: truncate to 0 decimal places and parse
		s := d.fallback.Trunc(0).String()
		if idx := strings.IndexByte(s, '.'); idx >= 0 {
			s = s[:idx]
		}
		i, _ := strconv.ParseInt(s, 10, 64)
		return i
	}
	return v
}

// optimized:
// IsInteger returns true when decimal can be represented as an integer value, otherwise, it returns false.
func (d Decimal) IsInteger() bool {
	if d.fallback == nil {
		return d.fixed%scale == 0
	}
	return d.fallback.Trunc(0).Equal(*d.fallback)
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
	return d.fallback.IsNeg()
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
	return d.fallback.IsPos()
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
	fb1 := d.asFallback()
	fb2 := d2.asFallback()
	result, err := fb1.Mod(fb2)
	if err != nil {
		panic("decimal division by zero")
	}
	return newFromFallback(result)
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
	return newFromFallback(d.asFallback().Mul(d2.asFallback()))
}

// optimized:
// Neg returns -d
func (d Decimal) Neg() Decimal {
	if d.fallback == nil {
		return Decimal{fixed: -d.fixed}
	}
	return newFromFallback(d.fallback.Neg())
}

// fallback:
// NumDigits returns the number of digits of the decimal coefficient (d.Value)
func (d Decimal) NumDigits() int {
	coef := d.Coefficient()
	if coef.Sign() == 0 {
		return 1
	}
	s := new(big.Int).Abs(coef).String()
	return len(s)
}

// fallback:
// Pow returns d to the power d2
func (d Decimal) Pow(d2 Decimal) Decimal {
	fb1 := d.asFallback()
	fb2 := d2.asFallback()
	result, err := fb1.PowToIntPart(fb2)
	if err != nil {
		panic(fmt.Sprintf("decimal pow error: %v", err))
	}
	return newFromFallback(result)
}

// fallback:
// QuoRem does divsion with remainder
func (d Decimal) QuoRem(d2 Decimal, prec int32) (Decimal, Decimal) {
	// Reimplement with big.Int to support the precision parameter
	fb1 := d.asFallback()
	fb2 := d2.asFallback()
	if fb2.IsZero() {
		panic("decimal division by zero")
	}

	// Parse both to big.Int coefficients
	s1 := fb1.String()
	s2 := fb2.String()

	num, p1 := parseToBigIntAndPrec(s1)
	den, p2 := parseToBigIntAndPrec(s2)

	// Align scales: both need to be at max(p1, p2) + prec
	targetScale := int64(prec)
	if targetScale < 0 {
		targetScale = 0
	}
	// Align numerator and denominator to same scale, then add prec extra digits to numerator
	commonPrec := p1
	if p2 > commonPrec {
		commonPrec = p2
	}
	// Scale num to commonPrec + prec, den to commonPrec
	numScale := int64(commonPrec) - int64(p1) + targetScale
	denScale := int64(commonPrec) - int64(p2)

	if numScale > 0 {
		num.Mul(num, new(big.Int).Exp(big.NewInt(10), big.NewInt(numScale), nil))
	}
	if denScale > 0 {
		den.Mul(den, new(big.Int).Exp(big.NewInt(10), big.NewInt(denScale), nil))
	}

	q, r := new(big.Int).QuoRem(num, den, new(big.Int))

	// q is the quotient with `prec` decimal places
	// r is the remainder with `commonPrec + prec` decimal places
	// But remainder should satisfy: d = q * d2 + r
	// where q has `prec` decimal places

	qDec := bigIntToDecimalWithPrec(q, prec)
	// Remainder: r has (commonPrec + prec) implicit decimal places
	// but we need remainder = d - q * d2
	// Remainder scale = commonPrec + prec
	remPrec := int32(commonPrec) + prec
	rDec := bigIntToDecimalWithPrec(r, remPrec)

	return qDec, rDec
}

// fallback:
// Rat returns a rational number representation of the decimal.
func (d Decimal) Rat() *big.Rat {
	r := new(big.Rat)
	r.SetString(d.String())
	return r
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
	return roundFallbackHAZ(d.asFallback(), places)
}

// fallback:
// RoundBank rounds the decimal to places decimal places.
// If the final digit to round is equidistant from the nearest two integers the
// rounded value is taken as the even number
//
// If places < 0, it will round the integer part to the nearest 10^(-places).
func (d Decimal) RoundBank(places int32) Decimal {
	if places >= 0 && places <= 19 {
		fb := d.asFallback()
		return newFromFallback(fb.RoundBank(uint8(places)))
	}
	return roundFallbackBank(d.asFallback(), places)
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
	var multiplier Decimal
	switch interval {
	case 5:
		multiplier = NewFromInt(20)
	case 10:
		multiplier = NewFromInt(10)
	case 25:
		multiplier = NewFromInt(4)
	case 50:
		multiplier = NewFromInt(2)
	case 100:
		multiplier = NewFromInt(1)
	default:
		panic(fmt.Sprintf("unsupported cash rounding interval: %d", interval))
	}
	return d.Mul(multiplier).Round(0).Div(multiplier).Truncate(2)
}

// fallback:
// RoundCeil rounds the decimal towards +infinity.
//
// Example:
//
//	NewFromFloat(545).RoundCeil(-2).String()   // output: "600"
//	NewFromFloat(500).RoundCeil(-2).String()   // output: "500"
//	NewFromFloat(1.1001).RoundCeil(2).String() // output: "1.11"
//	NewFromFloat(-1.454).RoundCeil(1).String() // output: "-1.4"
func (d Decimal) RoundCeil(places int32) Decimal {
	truncated := d.RoundDown(places)
	if truncated.Equal(d) {
		return d
	}
	if d.IsPositive() {
		return truncated.Add(newShiftedOne(places))
	}
	return truncated
}

// optimized:
// RoundDown rounds the decimal towards zero.
//
// Example:
//
//	NewFromFloat(545).RoundDown(-2).String()   // output: "500"
//	NewFromFloat(-500).RoundDown(-2).String()   // output: "-500"
//	NewFromFloat(1.1001).RoundDown(2).String() // output: "1.1"
//	NewFromFloat(-1.454).RoundDown(1).String() // output: "-1.5"
func (d Decimal) RoundDown(places int32) Decimal {
	if d.fallback != nil || places <= -7 {
		return roundDownFallback(d, places)
	}

	if places >= precision {
		// no need to round
		return d
	}

	s := pow10Table[precision-places]
	rescaled := (d.fixed / s) * s
	if rescaled == d.fixed {
		return d
	}

	return Decimal{fixed: rescaled}
}

func roundDownFallback(d Decimal, places int32) Decimal {
	fb := d.asFallback()
	if places >= 0 && places <= 19 {
		result := fb.Trunc(uint8(places))
		if places <= -7 {
			return newFromFallback(result)
		}
		return NewFromUDecimal(result)
	}
	// Negative places: round toward zero at 10^(-places) boundary
	return roundDownBigInt(fb, places)
}

// fallback:
// RoundFloor rounds the decimal towards -infinity.
//
// Example:
//
//	NewFromFloat(545).RoundFloor(-2).String()   // output: "500"
//	NewFromFloat(-500).RoundFloor(-2).String()   // output: "-500"
//	NewFromFloat(1.1001).RoundFloor(2).String() // output: "1.1"
//	NewFromFloat(-1.454).RoundFloor(1).String() // output: "-1.5"
func (d Decimal) RoundFloor(places int32) Decimal {
	truncated := d.RoundDown(places)
	if truncated.Equal(d) {
		return d
	}
	if d.IsNegative() {
		return truncated.Sub(newShiftedOne(places))
	}
	return truncated
}

// optimized:
// RoundUp rounds the decimal away from zero.
//
// Example:
//
//	NewFromFloat(545).RoundUp(-2).String()   // output: "600"
//	NewFromFloat(500).RoundUp(-2).String()   // output: "500"
//	NewFromFloat(1.1001).RoundUp(2).String() // output: "1.11"
//	NewFromFloat(-1.454).RoundUp(1).String() // output: "-1.4"
func (d Decimal) RoundUp(places int32) Decimal {
	if d.IsZero() {
		return d
	}
	if d.fallback != nil ||
		// roundup result is always more than MaxIntInFixed
		places <= -7 ||
		// roundup could cause fallback depending on fixed value
		// i.e. 9_000_000.0 with places=-6 =>  9_000_000 (optimizable)
		// i.e. 9_000_000.1 with places=-6 => 10_000_000 (fallback)
		// i.e. 9_200_000.1 with places=-5 =>  9_300_000 (fallback)
		(places < 0 &&
			(d.fixed > maxRoundUpThresholdInFixed || d.fixed < minRoundUpThresholdInFixed)) {
		// fallback
		sd := roundUpFallback(d, places)
		if places <= -7 {
			return sd
		}
		return sd
	}

	if places >= precision {
		// no need to round
		return d
	}

	s := pow10Table[precision-places]
	rescaled := (d.fixed / s) * s
	if rescaled == d.fixed {
		return d
	}

	if d.fixed >= 0 {
		return Decimal{fixed: rescaled + (1 * s)}
	}
	return Decimal{fixed: rescaled - (1 * s)}
}

func roundUpFallback(d Decimal, places int32) Decimal {
	fb := d.asFallback()
	if places >= 0 && places <= 19 {
		result := fb.RoundAwayFromZero(uint8(places))
		return NewFromUDecimal(result)
	}
	// Negative places: use big.Int
	return roundUpBigInt(fb, places)
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

	// fallback: try parsing as string
	var str string
	switch v := value.(type) {
	case []byte:
		str = string(v)
	case string:
		str = v
	default:
		// For other types, try udecimal Scan
		var fb udecimal.Decimal
		if err := fb.Scan(value); err != nil {
			return err
		}
		d.fallback = &fb
		return nil
	}
	fb, err := udecimal.Parse(str)
	if err != nil {
		return fmt.Errorf("can't convert %s to decimal: %w", str, err)
	}
	d.fallback = &fb
	return nil
}

// fallback:
// Shift multiplies d by 10^shift.
func (d Decimal) Shift(shift int32) Decimal {
	return d.Mul(New(1, shift))
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
	rounded := d.Round(places)
	return padStringToPlaces(rounded.String(), places)
}

// fallback:
// StringFixedBank returns a banker rounded fixed-point string with places digits
// after the decimal point.
func (d Decimal) StringFixedBank(places int32) string {
	rounded := d.RoundBank(places)
	return padStringToPlaces(rounded.String(), places)
}

// fallback:
// StringFixedCash returns a Swedish/Cash rounded fixed-point string. For
// more details see the documentation at function RoundCash.
func (d Decimal) StringFixedCash(interval uint8) string {
	rounded := d.RoundCash(interval)
	return padStringToPlaces(rounded.String(), 2)
}

// fallback:
// DEPRECATED! Use StringFixed instead.
func (d Decimal) StringScaled(exp int32) string {
	return d.StringFixed(-exp)
}

// optimized:
// Sub returns d - d2.
func (d Decimal) Sub(d2 Decimal) Decimal {
	return d.Add(d2.Neg())
}

// optimized:
// Truncate truncates off digits from the number, without rounding.
func (d Decimal) Truncate(precision int32) Decimal {
	if d.fallback == nil && precision >= 0 && precision <= 12 {
		s := pow10Table[12-precision]
		return Decimal{fixed: d.fixed / s * s}
	}
	if precision >= 0 && precision <= 19 {
		fb := d.asFallback()
		return newFromFallback(fb.Trunc(uint8(precision)))
	}
	if precision < 0 {
		return truncateNegativePrecision(d, precision)
	}

	panic("alpacadecimal.Truncate: invalid precision")
}

// fallback:
// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface.
func (d *Decimal) UnmarshalBinary(data []byte) error {
	var dd udecimal.Decimal
	if err := dd.UnmarshalBinary(data); err != nil {
		return err
	}
	ddd := newFromFallback(dd)
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

	str := string(decimalBytes)
	// Remove quotes
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}

	fb, err := udecimal.Parse(str)
	if err != nil {
		return fmt.Errorf("error decoding string %q: can't convert to decimal: %w", str, err)
	}
	result := newFromFallback(fb)
	d.fixed = result.fixed
	d.fallback = result.fallback
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

	str := string(text)
	fb, err := udecimal.Parse(str)
	if err != nil {
		return fmt.Errorf("error decoding string %q: can't convert to decimal: %w", str, err)
	}
	ddd := newFromFallback(fb)
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

	return d.fallback.String(), nil
}

// Extra API to support get internal state.
// e.g. might be useful for flatbuffers encode / decode.
func (d Decimal) GetFixed() int64 {
	return d.fixed
}

func (d Decimal) GetFallback() *udecimal.Decimal {
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
// Create a new alpacadecimal.Decimal from a udecimal.Decimal.
// Attempts to set the fixed value if possible.
func NewFromUDecimal(d udecimal.Decimal) Decimal {
	// Try to optimize via string parsing
	s := d.String()
	if fixed, ok := parseFixed(s); ok {
		return Decimal{fixed: fixed}
	}
	return newFromFallback(d)
}

// NewFromDecimal is an alias for NewFromUDecimal for API compatibility naming.
func NewFromDecimal(d udecimal.Decimal) Decimal {
	return NewFromUDecimal(d)
}

// internal implementation
func newFromFallback(d udecimal.Decimal) Decimal {
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

func (d Decimal) asFallback() udecimal.Decimal {
	if d.fallback == nil {
		r, _ := udecimal.NewFromInt64(d.fixed, precision)
		return r
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

// Helper functions

// formatBigIntWithExp formats a big.Int with an exponent to produce a decimal string.
func formatBigIntWithExp(v *big.Int, exp int32) string {
	if v.Sign() == 0 {
		return "0"
	}

	neg := v.Sign() < 0
	abs := new(big.Int).Abs(v)
	s := abs.String()

	if exp >= 0 {
		s = s + strings.Repeat("0", int(exp))
		if neg {
			return "-" + s
		}
		return s
	}

	// exp < 0: insert decimal point
	decPlaces := int(-exp)
	if decPlaces >= len(s) {
		// Need leading zeros
		s = strings.Repeat("0", decPlaces-len(s)+1) + s
	}
	insertPos := len(s) - decPlaces
	result := s[:insertPos] + "." + s[insertPos:]

	// Trim trailing zeros
	result = strings.TrimRight(result, "0")
	result = strings.TrimRight(result, ".")

	if neg {
		return "-" + result
	}
	return result
}

// padStringToPlaces pads or formats a decimal string to have exactly `places` digits after the decimal point.
func padStringToPlaces(s string, places int32) string {
	if places <= 0 {
		// For zero or negative places, just return the rounded integer
		if idx := strings.IndexByte(s, '.'); idx >= 0 {
			s = s[:idx]
		}
		return s
	}

	dotIdx := strings.IndexByte(s, '.')
	if dotIdx < 0 {
		return s + "." + strings.Repeat("0", int(places))
	}

	fracLen := int32(len(s) - dotIdx - 1)
	if fracLen >= places {
		return s
	}
	return s + strings.Repeat("0", int(places-fracLen))
}

// newShiftedOne returns 10^(-places) as a Decimal
func newShiftedOne(places int32) Decimal {
	return New(1, -places)
}

// roundFallbackHAZ rounds using half-away-from-zero for negative places
func roundFallbackHAZ(fb udecimal.Decimal, places int32) Decimal {
	if places >= 0 && places <= 19 {
		return newFromFallback(fb.RoundHAZ(uint8(places)))
	}
	// Negative places: round to nearest 10^(-places)
	return roundHAZBigInt(fb, places)
}

// roundFallbackBank rounds using banker's rounding for negative places
func roundFallbackBank(fb udecimal.Decimal, places int32) Decimal {
	// Negative places: use big.Int
	return roundBankBigInt(fb, places)
}

// Helper to parse a decimal string into big.Int coefficient and precision
func parseToBigIntAndPrec(s string) (*big.Int, int32) {
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg = true
		s = s[1:]
	}
	var prec int32
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		prec = int32(len(s) - idx - 1)
		s = s[:idx] + s[idx+1:]
	}
	bi := new(big.Int)
	bi.SetString(s, 10)
	if neg {
		bi.Neg(bi)
	}
	return bi, prec
}

// Helper to convert a big.Int with given precision to a Decimal
func bigIntToDecimalWithPrec(bi *big.Int, prec int32) Decimal {
	if prec <= 0 {
		s := bi.String()
		if fixed, ok := parseFixed(s); ok {
			return Decimal{fixed: fixed}
		}
		fb, err := udecimal.Parse(s)
		if err != nil {
			return newFromFallback(udecimal.Zero)
		}
		return newFromFallback(fb)
	}

	neg := bi.Sign() < 0
	abs := new(big.Int).Abs(bi)
	s := abs.String()

	for int32(len(s)) <= prec {
		s = "0" + s
	}
	intPart := s[:len(s)-int(prec)]
	fracPart := s[len(s)-int(prec):]
	fracPart = strings.TrimRight(fracPart, "0")

	var result string
	if fracPart == "" {
		result = intPart
	} else {
		result = intPart + "." + fracPart
	}
	if neg {
		result = "-" + result
	}

	if fixed, ok := parseFixed(result); ok {
		return Decimal{fixed: fixed}
	}
	fb, err := udecimal.Parse(result)
	if err != nil {
		return newFromFallback(udecimal.Zero)
	}
	return newFromFallback(fb)
}

// roundHAZBigInt implements half-away-from-zero rounding for negative places using big.Int
func roundHAZBigInt(fb udecimal.Decimal, places int32) Decimal {
	s := fb.String()
	bi, prec := parseToBigIntAndPrec(s)

	negPlaces := -places
	totalScale := int64(negPlaces) + int64(prec)
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(totalScale), nil)

	q, r := new(big.Int).QuoRem(bi, divisor, new(big.Int))
	half := new(big.Int).Quo(divisor, big.NewInt(2))

	ar := new(big.Int).Abs(r)
	if ar.Cmp(half) >= 0 {
		if bi.Sign() >= 0 {
			q.Add(q, big.NewInt(1))
		} else {
			q.Sub(q, big.NewInt(1))
		}
	}
	resultDivisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(negPlaces)), nil)
	q.Mul(q, resultDivisor)

	result := q.String()
	if fixed, ok := parseFixed(result); ok {
		return Decimal{fixed: fixed}
	}
	fb2, err := udecimal.Parse(result)
	if err != nil {
		return newFromFallback(udecimal.Zero)
	}
	return newFromFallback(fb2)
}

// roundBankBigInt implements banker's rounding for negative places using big.Int
func roundBankBigInt(fb udecimal.Decimal, places int32) Decimal {
	s := fb.String()
	bi, prec := parseToBigIntAndPrec(s)

	// Scale: work at precision `prec` so we don't lose fractional info.
	// divisor = 10^(-places + prec), so that bi / divisor gives us the integer quotient
	// at the rounding boundary.
	negPlaces := -places
	totalScale := int64(negPlaces) + int64(prec)
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(totalScale), nil)

	q, r := new(big.Int).QuoRem(bi, divisor, new(big.Int))
	half := new(big.Int).Quo(divisor, big.NewInt(2))

	ar := new(big.Int).Abs(r)
	cmp := ar.Cmp(half)
	if cmp > 0 || (cmp == 0 && new(big.Int).Abs(q).Bit(0) == 1) {
		if bi.Sign() >= 0 {
			q.Add(q, big.NewInt(1))
		} else {
			q.Sub(q, big.NewInt(1))
		}
	}
	// Result = q * 10^negPlaces
	resultDivisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(negPlaces)), nil)
	q.Mul(q, resultDivisor)

	result := q.String()
	if fixed, ok := parseFixed(result); ok {
		return Decimal{fixed: fixed}
	}
	fb2, err := udecimal.Parse(result)
	if err != nil {
		return newFromFallback(udecimal.Zero)
	}
	return newFromFallback(fb2)
}

// roundDownBigInt implements truncation for negative places using big.Int
func roundDownBigInt(fb udecimal.Decimal, places int32) Decimal {
	s := fb.String()
	bi, prec := parseToBigIntAndPrec(s)

	// For toward-zero truncation, losing fractional digits first is fine
	if prec > 0 {
		bi.Quo(bi, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(prec)), nil))
	}

	negPlaces := -places
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(negPlaces)), nil)

	q := new(big.Int).Quo(bi, divisor)
	q.Mul(q, divisor)

	result := q.String()
	if fixed, ok := parseFixed(result); ok {
		return Decimal{fixed: fixed}
	}
	fb2, err := udecimal.Parse(result)
	if err != nil {
		return newFromFallback(udecimal.Zero)
	}
	return newFromFallback(fb2)
}

// roundUpBigInt implements round-away-from-zero for negative places using big.Int
func roundUpBigInt(fb udecimal.Decimal, places int32) Decimal {
	s := fb.String()
	bi, prec := parseToBigIntAndPrec(s)

	negPlaces := -places
	totalScale := int64(negPlaces) + int64(prec)
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(totalScale), nil)

	q, r := new(big.Int).QuoRem(bi, divisor, new(big.Int))

	if r.Sign() != 0 {
		if bi.Sign() >= 0 {
			q.Add(q, big.NewInt(1))
		} else {
			q.Sub(q, big.NewInt(1))
		}
	}
	resultDivisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(negPlaces)), nil)
	q.Mul(q, resultDivisor)

	result := q.String()
	if fixed, ok := parseFixed(result); ok {
		return Decimal{fixed: fixed}
	}
	fb2, err := udecimal.Parse(result)
	if err != nil {
		return newFromFallback(udecimal.Zero)
	}
	return newFromFallback(fb2)
}

// truncateNegativePrecision handles Truncate with negative precision
func truncateNegativePrecision(d Decimal, precision int32) Decimal {
	fb := d.asFallback()
	s := fb.String()
	bi, prec := parseToBigIntAndPrec(s)

	if prec > 0 {
		bi.Quo(bi, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(prec)), nil))
	}

	negPrec := -precision
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(negPrec)), nil)
	q := new(big.Int).Quo(bi, divisor)
	q.Mul(q, divisor)

	result := q.String()
	if fixed, ok := parseFixed(result); ok {
		return Decimal{fixed: fixed}
	}
	fb2, err := udecimal.Parse(result)
	if err != nil {
		return newFromFallback(udecimal.Zero)
	}
	return newFromFallback(fb2)
}
