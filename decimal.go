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

var pow10Table = [19]int64{
	1e0, 1e1, 1e2, 1e3, 1e4,
	1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14,
	1e15, 1e16, 1e17, 1e18,
}

var pow10 = [19]udecimal.Decimal{
	udecimal.MustFromInt64(1e0, 0),  // 10^0
	udecimal.MustFromInt64(1e1, 0),  // 10^1
	udecimal.MustFromInt64(1e2, 0),  // 10^2
	udecimal.MustFromInt64(1e3, 0),  // 10^3
	udecimal.MustFromInt64(1e4, 0),  // 10^4
	udecimal.MustFromInt64(1e5, 0),  // 10^5
	udecimal.MustFromInt64(1e6, 0),  // 10^6
	udecimal.MustFromInt64(1e7, 0),  // 10^7
	udecimal.MustFromInt64(1e8, 0),  // 10^8
	udecimal.MustFromInt64(1e9, 0),  // 10^9
	udecimal.MustFromInt64(1e10, 0), // 10^10
	udecimal.MustFromInt64(1e11, 0), // 10^11
	udecimal.MustFromInt64(1e12, 0), // 10^12
	udecimal.MustFromInt64(1e13, 0), // 10^13
	udecimal.MustFromInt64(1e14, 0), // 10^14
	udecimal.MustFromInt64(1e15, 0), // 10^15
	udecimal.MustFromInt64(1e16, 0), // 10^16
	udecimal.MustFromInt64(1e17, 0), // 10^17
	udecimal.MustFromInt64(1e18, 0), // 10^18
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
	// configure udecimal with our desired defaults
	udecimal.SetDefaultParseMode(udecimal.ParseModeTrunc)

	// init cache
	for i := 0; i < cacheSize; i++ {
		str := strconv.FormatFloat(float64(i-cacheOffset)/100, 'f', -1, 64)

		valueCache[i] = str
		stringCache[i] = str
	}
}

// Global configuration

// SetDefaultParseModeError configures the underlying decimal engine to return
// an error when fractional digits exceed the default precision.
// This should be called once at startup before any parsing occurs.
func SetDefaultParseModeError() {
	udecimal.SetDefaultParseMode(udecimal.ParseModeError)
}

// SetDefaultParseModeTrunc configures the underlying decimal engine to silently
// truncate extra fractional digits instead of returning an error.
// This should be called once at startup before any parsing occurs.
func SetDefaultParseModeTrunc() {
	udecimal.SetDefaultParseMode(udecimal.ParseModeTrunc)
}

// SetDefaultPrecision sets the default precision (maximum fractional digits).
// The precision must be between 1 and 19. This should be called once at startup.
func SetDefaultPrecision(prec uint8) {
	udecimal.SetDefaultPrecision(prec)
}

// API

// APIs are marked as either "optimized" or "fallbacked"
// where "optimized" means that it's specially optimized
// where "fallback" means that it's not optimized and fallback from udecimal.Decimal
// mostly due to lack of usage in Alpaca. we should be able to move "fallback" to "optimized" as needed.

// Variables
var (
	DivisionPrecision        int32 = 16
	MarshalJSONWithoutQuotes       = false
	Zero                           = Decimal{fixed: 0}
)

type Decimal struct {
	// fallback to udecimal.Decimal if necessary
	fallback udecimal.Decimal

	// represent decimal with 12 precision, 1.23 will have `fixed = 1_230_000_000_000`
	// max support decimal is 9_223_372.000_000_000_000
	// min support decimal is -9_223_372.000_000_000_000
	fixed int64

	// hasFallback indicates whether the Decimal is using fallback representation.
	hasFallback bool
}

// optimized:
// Avg returns the average value of the provided first and rest Decimals
func Avg(first Decimal, rest ...Decimal) Decimal {
	divisor := NewFromInt(int64(1 + len(rest)))
	sum := Sum(first, rest...)
	return sum.Div(divisor)
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

	if exp > 0 {
		return newFromFallback(udecimal.MustFromInt64(value, 0).Mul(pow10[exp]))
	}

	// exp in [-19, 0] maps directly to NewFromInt64(value, -exp)
	fb, err := udecimal.NewFromInt64(value, uint8(-exp))
	if err != nil {
		panic(fmt.Sprintf("alpacadecimal.New: %v", err))
	}
	return newFromFallback(fb)
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

	// fallback
	d, err := udecimal.Parse(value)
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
	if !d.hasFallback {
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
	if !d.hasFallback && !d2.hasFallback {
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
	s := d.Truncate(0).StringFixed(0)
	bi := new(big.Int)
	bi.SetString(s, 10)
	return bi
}

// optimized:
// Ceil returns the nearest integer value greater than or equal to d.
func (d Decimal) Ceil() Decimal {
	if !d.hasFallback {
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
	if !d.hasFallback && !d2.hasFallback {
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
	if !d.hasFallback {
		return big.NewInt(d.fixed)
	}

	neg, hi, lo, _, ok := d.fallback.ToHiLo()
	if ok {
		// u128 path: construct big.Int from hi:lo directly
		bi := new(big.Int)
		if hi == 0 {
			// Fits in a single uint64 — cheapest path
			bi.SetUint64(lo)
		} else {
			// Combine hi and lo into a 128-bit big.Int
			bi.SetUint64(hi)
			bi.Lsh(bi, 64)
			bi.Or(bi, new(big.Int).SetUint64(lo))
		}
		if neg {
			bi.Neg(bi)
		}
		return bi
	}

	// bigInt overflow path — fall back to string parsing
	// (this is the rare case)
	return d.coefficientFromString()
}

func (d Decimal) coefficientFromString() *big.Int {
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
	if !d.hasFallback {
		return d.fixed
	}
	neg, hi, lo, _, ok := d.fallback.ToHiLo()
	if ok && hi == 0 && lo <= math.MaxInt64 {
		if neg {
			return -int64(lo)
		}
		return int64(lo)
	}
	// Fall back to big.Int path only when truly needed
	return d.Coefficient().Int64()
}

func (d Decimal) Rescale() Decimal {
	if !d.hasFallback {
		return d
	}
	return NewFromUDecimal(d.fallback)
}

// optimized:
// Copy returns a copy of decimal with the same value and exponent, but a different pointer to value.
func (d Decimal) Copy() Decimal {
	if !d.hasFallback {
		return Decimal{fixed: d.fixed}
	}
	return Decimal{fallback: d.fallback, hasFallback: true}
}

// optimized:
// Div returns d / d2. If it doesn't divide exactly, the result will have
// DivisionPrecision digits after the decimal point.
func (d Decimal) Div(d2 Decimal) Decimal {
	if !d.hasFallback && !d2.hasFallback {
		fixed, ok := div(d.fixed, d2.fixed)
		if ok {
			return Decimal{fixed: fixed}
		}
	}
	return d.DivRound(d2, DivisionPrecision)
}

// fallback:
// DivRound divides and rounds to a given precision
func (d Decimal) DivRound(d2 Decimal, prec int32) Decimal {
	fb1 := d.asFallback()
	fb2 := d2.asFallback()
	if fb2.IsZero() {
		panic("decimal division by zero")
	}

	// Fast path: prec fits in udecimal's 0..19 range
	if !(prec >= 0 && prec <= 19) {
		panic(fmt.Sprintf("DivRound precision must be between 0 and 19, got %d", prec))
	}

	result, err := fb1.Div(fb2)
	if err != nil {
		panic(fmt.Sprintf("decimal division error: %v", err))
	}
	rounded := result.RoundHAZ(uint8(prec))
	return NewFromUDecimal(rounded)
}

// optimized:
// Equal returns whether the numbers represented by d and d2 are equal.
func (d Decimal) Equal(d2 Decimal) bool {
	if !d.hasFallback && !d2.hasFallback {
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
	if !d.hasFallback {
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
	if !d.hasFallback {
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
	if !d.hasFallback && !d2.hasFallback {
		return d.fixed > d2.fixed
	}
	return d.asFallback().GreaterThan(d2.asFallback())
}

// optimized:
// GreaterThanOrEqual (GTE) returns true when d is greater than or equal to d2.
func (d Decimal) GreaterThanOrEqual(d2 Decimal) bool {
	if !d.hasFallback && !d2.hasFallback {
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
	if !d.hasFallback {
		return d.fixed / scale
	}
	v, err := d.fallback.Int64()
	if err != nil {
		// This should only happen if the integer part exceeds int64 bounds, which is outside our supported range.
		panic(fmt.Sprintf("decimal IntPart out of int64 bounds: %v", err))
	}
	return v
}

// optimized:
// IsInteger returns true when decimal can be represented as an integer value, otherwise, it returns false.
func (d Decimal) IsInteger() bool {
	if !d.hasFallback {
		return d.fixed%scale == 0
	}
	return d.fallback.Trunc(0).Equal(d.fallback)
}

// optimized:
// IsNegative return
//
//	true if d < 0
//	false if d == 0
//	false if d > 0
func (d Decimal) IsNegative() bool {
	if !d.hasFallback {
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
	if !d.hasFallback {
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
	if !d.hasFallback {
		return d.fixed == 0
	}
	return d.fallback.IsZero()
}

// optimized:
// LessThan (LT) returns true when d is less than d2.
func (d Decimal) LessThan(d2 Decimal) bool {
	if !d.hasFallback && !d2.hasFallback {
		return d.fixed < d2.fixed
	}
	return d.asFallback().LessThan(d2.asFallback())
}

// optimized:
// LessThanOrEqual (LTE) returns true when d is less than or equal to d2.
func (d Decimal) LessThanOrEqual(d2 Decimal) bool {
	if !d.hasFallback && !d2.hasFallback {
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
	if !d.hasFallback && !d2.hasFallback {
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
	if !d.hasFallback {
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
// QuoRem does division with remainder.
// The quotient has prec fractional digits and the remainder satisfies
// d == q * d2 + r (matching shopspring/decimal semantics).
func (d Decimal) QuoRem(d2 Decimal, prec int32) (Decimal, Decimal) {
	// Fast path: prec fits in udecimal's 0..19 range
	if !(prec >= 0 && prec <= 19) {
		panic(fmt.Sprintf("QuoRem precision must be between 0 and 19, got %d", prec))
	}

	fb1 := d.asFallback()
	fb2 := d2.asFallback()

	scaled := fb1.Mul(pow10[prec])

	// udecimal QuoRem: intQ is an integer, intR has max(scaled.prec, fb2.prec)
	// fractional digits. Uses optimized u128 arithmetic when possible.
	intQ, intR, err := scaled.QuoRem(fb2)
	if err != nil {
		panic(fmt.Sprintf("decimal QuoRem error: %v", err))
	}

	// Divide both by 10^prec to restore the correct precision.
	// Quotient: intQ / 10^prec has exactly prec fractional digits (exact).
	// Remainder: intR / 10^prec has (commonPrec + prec) fractional digits
	// (exact when commonPrec + prec <= 19).
	qU, _ := intQ.Div(pow10[prec])
	rU, _ := intR.Div(pow10[prec])

	return newFromFallback(qU), newFromFallback(rU)
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
	if !d.hasFallback {
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
	// fallback
	if places >= 0 {
		return NewFromUDecimal(d.asFallback().RoundHAZ(uint8(places)))
	}

	res, _ := d.asFallback().Div(pow10[int(-places)])
	res = res.RoundHAZ(0)
	if res.IsZero() {
		return Zero
	}
	return NewFromUDecimal(res.Mul(pow10[int(-places)]))
}

// fallback:
// RoundBank rounds the decimal to places decimal places.
// If the final digit to round is equidistant from the nearest two integers the
// rounded value is taken as the even number
//
// If places < 0, it will round the integer part to the nearest 10^(-places).
func (d Decimal) RoundBank(places int32) Decimal {
	if d.IsZero() {
		return Zero
	}

	if places >= 0 {
		fb := d.asFallback()
		return newFromFallback(fb.RoundBank(uint8(places)))
	}
	res := d.asFallback()
	if res.Prec()+int(-places) > 19 {
		// If the number of digits to round is greater than 19,
		// we need to round it first before dividing
		// to avoid precision issues in udecimal.
		res = res.RoundAwayFromZero(0)
	}
	res, _ = res.Div(pow10[int(-places)])
	res = res.RoundBank(0)
	return NewFromUDecimal(res.Mul(pow10[int(-places)]))
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
	if d.hasFallback || places <= -7 {
		if places >= 0 {
			return NewFromUDecimal(d.asFallback().Trunc(uint8(places)))
		}

		res, _ := d.asFallback().Div(pow10[int(-places)])
		res = res.Trunc(0)
		if res.IsZero() {
			return Zero
		}
		return NewFromUDecimal(res.Mul(pow10[int(-places)]))
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
	if d.hasFallback ||
		// roundup result is always more than MaxIntInFixed
		places <= -7 ||
		// roundup could cause fallback depending on fixed value
		// i.e. 9_000_000.0 with places=-6 =>  9_000_000 (optimizable)
		// i.e. 9_000_000.1 with places=-6 => 10_000_000 (fallback)
		// i.e. 9_200_000.1 with places=-5 =>  9_300_000 (fallback)
		(places < 0 &&
			(d.fixed > maxRoundUpThresholdInFixed || d.fixed < minRoundUpThresholdInFixed)) {
		// fallback
		if places >= 0 {
			return NewFromUDecimal(d.asFallback().RoundAwayFromZero(uint8(places)))
		}

		// RoundAwayFromZero to the integer part, then multiply back by 10^(-places).
		fb := d.asFallback().RoundAwayFromZero(0)
		res, _ := fb.Div(pow10[int(-places)])
		res = res.RoundAwayFromZero(0)
		return NewFromUDecimal(res.Mul(pow10[int(-places)]))
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
			d.hasFallback = false
			return nil
		}

	case string:
		fixed, ok := parseFixed(v)
		if ok {
			d.fixed = fixed
			d.hasFallback = false
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
		d.fallback = fb
		d.hasFallback = true
		return nil
	}
	fb, err := udecimal.Parse(str)
	if err != nil {
		return fmt.Errorf("can't convert %s to decimal: %w", str, err)
	}
	d.fallback = fb
	d.hasFallback = true
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
	if !d.hasFallback {
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
	if !d.hasFallback {
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
	if !d.hasFallback && !d2.hasFallback {
		if d2.fixed > 0 {
			if d.fixed >= minIntInFixed+d2.fixed {
				return Decimal{fixed: d.fixed - d2.fixed}
			}
		} else {
			if d.fixed <= maxIntInFixed+d2.fixed {
				return Decimal{fixed: d.fixed - d2.fixed}
			}
		}
	}

	return newFromFallback(d.asFallback().Sub(d2.asFallback()))
}

// optimized:
// Truncate truncates off digits from the number, without rounding.
func (d Decimal) Truncate(precision int32) Decimal {
	if !d.hasFallback && precision >= 0 && precision <= 12 {
		s := pow10Table[12-precision]
		return Decimal{fixed: d.fixed / s * s}
	}

	fb := d.asFallback()
	return newFromFallback(fb.Trunc(uint8(precision)))
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
	d.hasFallback = ddd.hasFallback
	return nil
}

// optimized:
// UnmarshalJSON implements the json.Unmarshaler interface.
func (d *Decimal) UnmarshalJSON(decimalBytes []byte) error {
	if fixed, ok := parseFixed(decimalBytes); ok {
		d.fixed = fixed
		d.hasFallback = false
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
	d.hasFallback = result.hasFallback
	return nil
}

// optimized:
// UnmarshalText implements the encoding.TextUnmarshaler interface for XML
// deserialization.
func (d *Decimal) UnmarshalText(text []byte) error {
	if fixed, ok := parseFixed(text); ok {
		d.fixed = fixed
		d.hasFallback = false
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
	d.hasFallback = ddd.hasFallback
	return nil
}

// optimized:
// sql.Valuer interface
func (d Decimal) Value() (driver.Value, error) {
	if !d.hasFallback {
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
	if !d.hasFallback {
		return nil
	}
	fb := d.fallback
	return &fb
}

func (d Decimal) IsOptimized() bool {
	return !d.hasFallback
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
	if fixed, ok, certain := tryFixedFromUDecimal(d); ok {
		return Decimal{fixed: fixed}
	} else if certain {
		return newFromFallback(d)
	}

	// Rare fallback when fast-path introspection can't determine fit.
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
	return Decimal{fallback: d, hasFallback: true}
}

func tryFixedFromUDecimal(d udecimal.Decimal) (fixed int64, ok bool, certain bool) {
	neg, hi, lo, prec, fit := d.ToHiLo()
	if !fit || hi != 0 {
		return 0, false, false
	}

	certain = true
	if lo == 0 {
		return 0, true, true
	}

	coef := lo
	p := int32(prec)

	// If precision is higher than optimized precision, we can still optimize
	// only when the extra trailing digits are zeros.
	if p > precision {
		extra := p - precision
		divisor := uint64(pow10Table[extra])
		if coef%divisor != 0 {
			return 0, false, true
		}
		coef /= divisor
		p = precision
	}

	multiplier := uint64(pow10Table[precision-p])
	maxAbs := uint64(maxIntInFixed - 1)
	if coef > maxAbs/multiplier {
		return 0, false, true
	}

	absFixed := coef * multiplier
	fixed = int64(absFixed)
	if neg {
		fixed = -fixed
	}

	return fixed, true, true
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
	if !d.hasFallback {
		r, _ := udecimal.NewFromInt64(d.fixed, precision)
		return r
	}
	return d.fallback
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
	if places <= 0 {
		return NewFromUDecimal(pow10[int(-places)])
	}
	return New(1, -places)
}
