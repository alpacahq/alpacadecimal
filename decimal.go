// Package alpacadecimal provides a high-performance, drop-in replacement for
// github.com/shopspring/decimal.
//
// Values whose magnitude is at most 9,223,372 with at most 12 fractional
// digits are stored in a single int64 ("fixed" representation, the optimized
// 99% case). Everything else falls back to github.com/quagmt/udecimal, which
// supports up to 19 fractional digits with a 128-bit (or larger) coefficient.
//
// Compatibility notes vs shopspring/decimal:
//   - Exponent, Coefficient, CoefficientInt64 and NumDigits return different
//     (but valid) representations of equal values.
//   - Fractional digits beyond 19 are truncated (or rejected after
//     SetDefaultParseModeError); shopspring keeps arbitrary precision.
package alpacadecimal

import (
	"database/sql/driver"
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"regexp"
	"strconv"
	"strings"

	"github.com/quagmt/udecimal"
)

// currently support 12 precision, this is tunnable,
// more precision => smaller maxInt
// less precision => bigger maxInt
const (
	precision       = 12
	scale           = 1e12
	maxInt    int64 = int64(math.MaxInt64) / scale
	minInt    int64 = int64(math.MinInt64) / scale

	maxIntInFixed int64 = maxInt * scale
	minIntInFixed int64 = minInt * scale

	a1000InFixed    int64 = 1000 * scale
	aNeg1000InFixed int64 = -1000 * scale
	aCentInFixed    int64 = scale / 100
)

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

// SetDefaultParseModeError configures parsing to return an error when
// fractional digits exceed the supported 19 digit precision.
// This should be called once at startup before any parsing occurs.
func SetDefaultParseModeError() {
	parseModeError = true
	udecimal.SetDefaultParseMode(udecimal.ParseModeError)
}

// SetDefaultParseModeTrunc configures parsing to silently truncate extra
// fractional digits instead of returning an error (the default).
// This should be called once at startup before any parsing occurs.
func SetDefaultParseModeTrunc() {
	parseModeError = false
	udecimal.SetDefaultParseMode(udecimal.ParseModeTrunc)
}

// SetDefaultPrecision sets the fallback engine's default precision (maximum
// fractional digits). The precision must be between 1 and 19. This should be
// called once at startup.
func SetDefaultPrecision(prec uint8) {
	udecimal.SetDefaultPrecision(prec)
}

// Variables (matching shopspring/decimal)
var (
	// DivisionPrecision is the number of decimal places in the result when it
	// doesn't divide exactly.
	DivisionPrecision = 16

	// PowPrecisionNegativeExponent specifies the maximum precision of the
	// result (digits after the decimal point) when calculating a decimal to a
	// negative power.
	PowPrecisionNegativeExponent = 16

	// MarshalJSONWithoutQuotes should be set to true if you want the decimal
	// to be JSON marshaled as a number, instead of as a string.
	MarshalJSONWithoutQuotes = false

	// Zero constant, to make computations faster.
	Zero = Decimal{fixed: 0}
)

// Decimal represents a fixed-point decimal number, API-compatible with
// shopspring/decimal. The zero value is 0.
type Decimal struct {
	// fixed holds the value scaled by 10^12 when fallback is nil:
	// 1.23 is stored as fixed = 1_230_000_000_000.
	// max supported fixed value is 9_223_372.000_000_000_000
	// min supported fixed value is -9_223_372.000_000_000_000
	fixed int64

	// fallback holds out-of-fixed-range values. nil means the fixed
	// representation is authoritative. The pointee is never mutated.
	fallback *udecimal.Decimal
}

// APIs are marked as either "optimized" or "fallbacked"
// where "optimized" means that it's specially optimized
// where "fallback" means that it's not optimized and falls back to
// udecimal.Decimal / big.Int arithmetic.

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
// Sum returns the combined total of the provided first and rest Decimals
func Sum(first Decimal, rest ...Decimal) Decimal {
	result := first
	for _, item := range rest {
		result = result.Add(item)
	}
	return result
}

// optimized:
// New returns a new fixed-point decimal, value * 10 ^ exp.
func New(value int64, exp int32) Decimal {
	if exp >= -12 && exp <= 6 {
		// fast bounds check for the common exponent range
		e := int(exp) + precision
		s := pow10Table[e]
		if value <= maxIntInFixed/s && value >= minIntInFixed/s {
			return Decimal{fixed: value * s}
		}
	}
	return newFromInt64Exp(value, int64(exp))
}

// optimized:
// NewFromBigInt returns a new Decimal from a big.Int, value * 10 ^ exp
func NewFromBigInt(value *big.Int, exp int32) Decimal {
	if value.IsInt64() {
		return New(value.Int64(), exp)
	}
	return decimalFromBigParts(value, int64(exp))
}

// fallback:
// NewFromBigRat returns a new Decimal from a big.Rat. The numerator and
// denominator are divided and rounded to the given precision.
func NewFromBigRat(value *big.Rat, precision int32) Decimal {
	num := NewFromBigInt(value.Num(), 0)
	denom := NewFromBigInt(value.Denom(), 0)
	return num.DivRound(denom, precision)
}

// optimized:
// NewFromFloat converts a float64 to Decimal. Like shopspring, the result
// contains the minimal number of digits that round-trip through float64.
//
// NOTE: this will panic on NaN, +/-inf
func NewFromFloat(f float64) Decimal {
	af := math.Abs(f)
	// Fast path: for |f| < 8192, ulp(f) <= 2^-40 < 1e-12, so at most one
	// 12-fractional-digit decimal round-trips to f; if one exists it equals
	// the shortest representation that the slow path would parse.
	if af < 8192 {
		n := math.Floor(af * 1e12) // exact: af*1e12 < 2^53
		if n/1e12 != af {
			n++
		}
		if n/1e12 == af { // correctly-rounded division == ParseFloat round-trip
			fixed := int64(n)
			if math.Signbit(f) {
				fixed = -fixed
			}
			return Decimal{fixed: fixed}
		}
	}
	return newFromFloatSlow(f)
}

func newFromFloatSlow(f float64) Decimal {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		panic(fmt.Sprintf("Cannot create a Decimal from %v", f))
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
// NewFromFloat32 converts a float32 to Decimal, using the same
// shortest-representation algorithm as shopspring (it differs from strconv's
// in rare 1-ulp cases, and parity wins here).
//
// NOTE: this will panic on NaN, +/-inf
func NewFromFloat32(f float32) Decimal {
	if f == 0 {
		return Zero
	}
	// XOR is a workaround for https://github.com/golang/go/issues/26285
	a := math.Float32bits(f) ^ 0x80808080
	return newFromFloatBits(float64(f), uint64(a)^0x80808080, &float32info)
}

// newFromFloatBits ports shopspring's newFromFloat: the shortest decimal
// representation that reconstructs the float exactly.
func newFromFloatBits(val float64, bits uint64, flt *floatInfo) Decimal {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		panic(fmt.Sprintf("Cannot create a Decimal from %v", val))
	}
	exp := int(bits>>flt.mantbits) & (1<<flt.expbits - 1)
	mant := bits & (uint64(1)<<flt.mantbits - 1)

	switch exp {
	case 0:
		// denormalized
		exp++
	default:
		// add implicit top bit
		mant |= uint64(1) << flt.mantbits
	}
	exp += flt.bias

	var dd decimal
	dd.Assign(mant)
	dd.Shift(exp - int(flt.mantbits))
	dd.neg = bits>>(flt.expbits+flt.mantbits) != 0

	roundShortest(&dd, mant, exp, flt)
	// floats have at most 17 significant digits, so the mantissa fits int64
	var m int64
	for i := 0; i < dd.nd; i++ {
		m = m*10 + int64(dd.d[i]-'0')
	}
	if dd.neg {
		m = -m
	}
	return newFromInt64Exp(m, int64(dd.dp)-int64(dd.nd))
}

// fallback:
// NewFromFloatWithExponent converts a float64 to Decimal, with an arbitrary
// number of fractional digits. The implementation mirrors shopspring's exactly
// (binary expansion of the float, rounded half away from zero at 10^exp).
//
// Example:
//
//	NewFromFloatWithExponent(123.456, -2).String() // output: "123.46"
func NewFromFloatWithExponent(value float64, exp int32) Decimal {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		panic(fmt.Sprintf("Cannot create a Decimal from %v", value))
	}

	bits64 := math.Float64bits(value)
	mant := bits64 & (1<<52 - 1)
	exp2 := int32((bits64 >> 52) & (1<<11 - 1))
	sign := bits64 >> 63

	if exp2 == 0 {
		if mant == 0 {
			return Decimal{}
		}
		// subnormal
		exp2++
	} else {
		// normal
		mant |= 1 << 52
	}

	exp2 -= 1023 + 52

	// normalizing base-2 values
	for mant&1 == 0 {
		mant = mant >> 1
		exp2++
	}

	// maximum number of fractional base-10 digits to represent 2^N exactly
	// cannot be more than -N if N<0
	if exp < 0 && exp < exp2 {
		if exp2 < 0 {
			exp = exp2
		} else {
			exp = 0
		}
	}

	// representing 10^M * 2^N as 5^M * 2^(M+N)
	exp2 -= exp

	temp := big.NewInt(1)
	dMant := big.NewInt(int64(mant))

	// applying 5^M
	five := big.NewInt(5)
	if exp > 0 {
		temp = temp.SetInt64(int64(exp))
		temp = temp.Exp(five, temp, nil)
	} else if exp < 0 {
		temp = temp.SetInt64(-int64(exp))
		temp = temp.Exp(five, temp, nil)
		dMant = dMant.Mul(dMant, temp)
		temp = temp.SetUint64(1)
	}

	// applying 2^(M+N)
	if exp2 > 0 {
		dMant = dMant.Lsh(dMant, uint(exp2))
	} else if exp2 < 0 {
		temp = temp.Lsh(temp, uint(-exp2))
	}

	// rounding and downscaling
	if exp > 0 || exp2 < 0 {
		halfDown := new(big.Int).Rsh(temp, 1)
		dMant = dMant.Add(dMant, halfDown)
		dMant = dMant.Quo(dMant, temp)
	}

	if sign == 1 {
		dMant = dMant.Neg(dMant)
	}

	return decimalFromBigParts(dMant, int64(exp))
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
	fb, _ := udecimal.NewFromInt64(x, 0)
	return newFromFallback(fb)
}

// optimized:
// NewFromInt32 converts a int32 to Decimal.
func NewFromInt32(value int32) Decimal {
	return NewFromInt(int64(value))
}

// optimized:
// NewFromUint64 converts an uint64 to Decimal.
func NewFromUint64(value uint64) Decimal {
	if value <= uint64(maxInt) {
		return Decimal{fixed: int64(value) * scale}
	}
	fb, _ := udecimal.NewFromUint64(value, 0)
	return newFromFallback(fb)
}

// optimized:
// NewFromString returns a new Decimal from a string representation.
// Scientific notation ("1.23e4") is supported, like shopspring.
func NewFromString(value string) (Decimal, error) {
	if fixed, ok := parseFixed(value); ok {
		return Decimal{fixed: fixed}, nil
	}
	return parseSlow(value)
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
// Abs returns the absolute value of the decimal.
func (d Decimal) Abs() Decimal {
	if d.fallback == nil {
		if d.fixed >= 0 {
			return d
		}
		return Decimal{fixed: -d.fixed}
	}
	return d.absSlow()
}

func (d Decimal) absSlow() Decimal {
	return newFromFallback(d.fallback.Abs())
}

// optimized:
// Add returns d + d2.
func (d Decimal) Add(d2 Decimal) Decimal {
	// if result of add does not overflow,
	// we can keep result in fixed form as well.
	// otherwise, we need to fall back to udecimal.Decimal
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
		// overflow: result is out of fixed range for sure
		return newFromFallback(d.asFallback().Add(d2.asFallback()))
	}
	// mixed operands: the result may fit the fixed range again
	return NewFromUDecimal(d.asFallback().Add(d2.asFallback()))
}

// optimized:
// Sub returns d - d2.
func (d Decimal) Sub(d2 Decimal) Decimal {
	if d.fallback == nil && d2.fallback == nil {
		if d2.fixed > 0 {
			if d.fixed >= minIntInFixed+d2.fixed {
				return Decimal{fixed: d.fixed - d2.fixed}
			}
		} else {
			if d.fixed <= maxIntInFixed+d2.fixed {
				return Decimal{fixed: d.fixed - d2.fixed}
			}
		}
		return newFromFallback(d.asFallback().Sub(d2.asFallback()))
	}
	return NewFromUDecimal(d.asFallback().Sub(d2.asFallback()))
}

// optimized:
// Mul returns d * d2
func (d Decimal) Mul(d2 Decimal) Decimal {
	if d.fallback == nil && d2.fallback == nil {
		if fixed, ok := mul(d.fixed, d2.fixed); ok {
			return Decimal{fixed: fixed}
		}
	}
	return NewFromUDecimal(d.asFallback().Mul(d2.asFallback()))
}

// optimized:
// Neg returns -d
func (d Decimal) Neg() Decimal {
	if d.fallback == nil {
		return Decimal{fixed: -d.fixed}
	}
	return d.negSlow()
}

func (d Decimal) negSlow() Decimal {
	return newFromFallback(d.fallback.Neg())
}

// optimized:
// Div returns d / d2. If it doesn't divide exactly, the result will have
// DivisionPrecision digits after the decimal point.
func (d Decimal) Div(d2 Decimal) Decimal {
	if d.fallback == nil && d2.fallback == nil {
		if d2.fixed == 0 {
			panic("decimal division by 0")
		}
		dp := DivisionPrecision
		// the exact quotient has at most 12 fractional digits, so it equals
		// the DivRound(dp) result only when dp covers all of them
		if dp >= 12 {
			if fixed, ok := div(d.fixed, d2.fixed); ok {
				return Decimal{fixed: fixed}
			}
		}
		if dp >= 0 && dp <= 19 {
			if coef, neg, ok := divRoundBits(d.fixed, d2.fixed, dp); ok {
				if u, err := udecimal.NewFromHiLo(neg, 0, coef, uint8(dp)); err == nil {
					return NewFromUDecimal(u)
				}
			}
		}
	}
	return d.DivRound(d2, int32(DivisionPrecision))
}

// optimized:
// DivRound divides and rounds to a given precision
// i.e. to an integer multiple of 10^(-precision)
//
//	for a positive quotient digit 5 is rounded up, away from 0
//	if the quotient is negative then digit 5 is rounded down, away from 0
//
// Note that precision<0 is allowed as input.
func (d Decimal) DivRound(d2 Decimal, precision int32) Decimal {
	if d.fallback == nil && d2.fallback == nil {
		if d2.fixed == 0 {
			panic("decimal division by 0")
		}
		if precision >= 0 && precision <= 19 {
			if coef, neg, ok := divRoundBits(d.fixed, d2.fixed, int(precision)); ok {
				if u, err := udecimal.NewFromHiLo(neg, 0, coef, uint8(precision)); err == nil {
					return NewFromUDecimal(u)
				}
			}
		}
	} else if precision >= 0 && precision <= 18 {
		// udecimal.Div truncates the quotient at 19 digits, so rounding half
		// away from zero at <= 18 digits sees the exact deciding digits.
		fb2 := d2.asFallback()
		if fb2.IsZero() {
			panic("decimal division by 0")
		}
		q, err := d.asFallback().Div(fb2)
		if err == nil {
			return NewFromUDecimal(q.RoundHAZ(uint8(precision)))
		}
	}
	return divRoundBig(d, d2, precision)
}

// optimized:
// Mod returns d % d2.
func (d Decimal) Mod(d2 Decimal) Decimal {
	if d.fallback == nil && d2.fallback == nil {
		if d2.fixed == 0 {
			panic("decimal division by 0")
		}
		// remainder of the scaled integers; always representable since
		// |d.fixed % d2.fixed| < |d2.fixed|
		return Decimal{fixed: d.fixed % d2.fixed}
	}
	// The remainder needs at most max(prec1, prec2) <= 19 fractional digits,
	// so udecimal computes it exactly.
	fb2 := d2.asFallback()
	if fb2.IsZero() {
		panic("decimal division by 0")
	}
	r, err := d.asFallback().Mod(fb2)
	if err == nil {
		return NewFromUDecimal(r)
	}
	_, rr := quoRemBig(d, d2, 0)
	return rr
}

// optimized:
// QuoRem does division with remainder
// d.QuoRem(d2,precision) returns quotient q and remainder r such that
//
//	d = d2 * q + r, q an integer multiple of 10^(-precision)
//	0 <= r < abs(d2) * 10 ^(-precision) if d>=0
//	0 >= r > -abs(d2) * 10 ^(-precision) if d<0
//
// Note that precision<0 is allowed as input.
func (d Decimal) QuoRem(d2 Decimal, precision int32) (Decimal, Decimal) {
	if d.fallback == nil && d2.fallback == nil {
		if d2.fixed == 0 {
			panic("decimal division by 0")
		}
		if precision == 0 {
			q := d.fixed / d2.fixed
			if q <= maxInt && q >= minInt {
				return Decimal{fixed: q * scale}, Decimal{fixed: d.fixed % d2.fixed}
			}
		} else if precision > 0 && precision <= 7 {
			if q, r, ok := quoRemFixed(d.fixed, d2.fixed, int(precision)); ok {
				return q, r
			}
		}
	} else if q, r, ok := quoRemFallback(d, d2, precision); ok {
		return q, r
	}
	return quoRemBig(d, d2, precision)
}

// quoRemFallback computes QuoRem through udecimal when every intermediate is
// exactly representable: max(prec1, prec2) + precision <= 19.
func quoRemFallback(d, d2 Decimal, precision int32) (Decimal, Decimal, bool) {
	if precision < 0 || precision > 18 {
		return Decimal{}, Decimal{}, false
	}
	fb1 := d.asFallback()
	fb2 := d2.asFallback()
	if fb2.IsZero() {
		panic("decimal division by 0")
	}
	maxPrec := int32(fb1.PrecUint())
	if p2 := int32(fb2.PrecUint()); p2 > maxPrec {
		maxPrec = p2
	}
	if maxPrec+precision > 19 {
		return Decimal{}, Decimal{}, false
	}
	// scaled = d * 10^precision (exact); intQ integer, intR exact remainder
	scaled := fb1.Mul(udecPow10[precision])
	intQ, intR, err := scaled.QuoRem(fb2)
	if err != nil {
		return Decimal{}, Decimal{}, false
	}
	// q = intQ / 10^precision: exact, gains precision fractional digits.
	// r = intR / 10^precision: exact since intR.prec + precision <= 19.
	q, err1 := intQ.Div(udecPow10[precision])
	r, err2 := intR.Div(udecPow10[precision])
	if err1 != nil || err2 != nil {
		return Decimal{}, Decimal{}, false
	}
	return NewFromUDecimal(q), NewFromUDecimal(r), true
}

// optimized:
// Shift shifts the decimal in base 10. It shifts left when shift is positive
// and right if shift is negative. In simpler terms, the given value for shift
// is added to the exponent of the decimal.
func (d Decimal) Shift(shift int32) Decimal {
	if shift == 0 {
		return d
	}
	if d.fallback == nil {
		if fixed, ok := shiftFixed(d.fixed, shift); ok {
			return Decimal{fixed: fixed}
		}
	}
	coef, exp := d.toBigParts()
	return decimalFromBigParts(coef, int64(exp)+int64(shift))
}

func shiftFixed(fixed int64, shift int32) (int64, bool) {
	if shift > 0 {
		if shift > 6 {
			return 0, false
		}
		s := pow10Table[shift]
		if fixed > maxIntInFixed/s || fixed < minIntInFixed/s {
			return 0, false
		}
		return fixed * s, true
	}
	if shift < -18 {
		return 0, false
	}
	s := pow10Table[-shift]
	if fixed%s != 0 {
		return 0, false
	}
	return fixed / s, true
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
// Compare compares the numbers represented by d and d2 and returns:
//
//	-1 if d <  d2
//	 0 if d == d2
//	+1 if d >  d2
func (d Decimal) Compare(d2 Decimal) int {
	return d.Cmp(d2)
}

// optimized:
// Equal returns whether the numbers represented by d and d2 are equal.
func (d Decimal) Equal(d2 Decimal) bool {
	if d.fallback == nil && d2.fallback == nil {
		return d.fixed == d2.fixed
	}
	return d.asFallback().Equal(d2.asFallback())
}

// Deprecated: Equals is deprecated, please use Equal method instead
func (d Decimal) Equals(d2 Decimal) bool {
	return d.Equal(d2)
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
	return d.signSlow()
}

func (d Decimal) signSlow() int {
	return d.fallback.Sign()
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
	return d.negativeSlow()
}

func (d Decimal) negativeSlow() bool {
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
	return d.positiveSlow()
}

func (d Decimal) positiveSlow() bool {
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

// internal implementation

func newFromFallback(u udecimal.Decimal) Decimal {
	return Decimal{fallback: &u}
}

func (d Decimal) asFallback() udecimal.Decimal {
	if d.fallback == nil {
		r, _ := udecimal.NewFromInt64(d.fixed, precision)
		return r
	}
	return *d.fallback
}

// mul multiplies two fixed values exactly. ok is false when the result is not
// representable in fixed form (out of range or needs >12 fractional digits).
func mul(x, y int64) (int64, bool) {
	xf := x % scale
	yf := y % scale
	if xf|yf == 0 {
		// both integers: single 64-bit multiply
		a := x / scale
		b := y / scale
		p := a * b // |a|,|b| <= 9_223_372 so the product cannot overflow
		if p > maxInt || p < minInt {
			return 0, false
		}
		return p * scale, true
	}

	neg := (x < 0) != (y < 0)
	hi, lo := bits.Mul64(uabs(x), uabs(y))
	if hi >= 1e12 {
		// quotient would exceed 64 bits: out of range for sure
		return 0, false
	}
	q, r := divu128By1e12(hi, lo)
	if r != 0 || q > uint64(maxIntInFixed) {
		return 0, false
	}
	if neg {
		return -int64(q), true
	}
	return int64(q), true
}

// div divides two fixed values exactly using 128-bit arithmetic.
// ok is false when x/y is not exactly representable in fixed form.
func div(x, y int64) (int64, bool) {
	if x == 0 {
		return 0, y != 0
	}
	if y == 0 {
		return 0, false
	}
	neg := (x < 0) != (y < 0)
	uy := uabs(y)
	hi, lo := bits.Mul64(uabs(x), scale)
	if hi >= uy {
		// quotient exceeds 64 bits: cannot fit
		return 0, false
	}
	q, r := bits.Div64(hi, lo, uy)
	if r != 0 || q > uint64(maxIntInFixed) {
		return 0, false
	}
	if neg {
		return -int64(q), true
	}
	return int64(q), true
}

// divRoundBits computes round-half-away-from-zero(x/y * 10^prec) using 128/64
// division. Returns the rounded coefficient at scale 10^prec plus its sign.
// ok is false when the quotient needs more than 64 bits (caller falls back).
func divRoundBits(x, y int64, prec int) (coef uint64, neg bool, ok bool) {
	if y == 0 {
		return 0, false, false
	}
	if x == 0 {
		return 0, false, true
	}
	neg = (x < 0) != (y < 0)
	uy := uabs(y)
	hi, lo := bits.Mul64(uabs(x), pow10u[prec])
	if hi >= uy {
		return 0, false, false
	}
	q, r := bits.Div64(hi, lo, uy)
	// round half away from zero; r*2 cannot overflow because uy < 2^63
	if r*2 >= uy {
		if q == math.MaxUint64 {
			return 0, false, false
		}
		q++
	}
	return q, neg, true
}

// quoRemFixed computes QuoRem on fixed values for 0 < prec <= 7.
// q = trunc(x/y at prec digits), r = x - q*y (exact).
func quoRemFixed(x, y int64, prec int) (Decimal, Decimal, bool) {
	neg := (x < 0) != (y < 0)
	uy := uabs(y)
	hi, lo := bits.Mul64(uabs(x), pow10u[prec])
	if hi >= uy {
		return Decimal{}, Decimal{}, false
	}
	qi, ri := bits.Div64(hi, lo, uy)
	// q = qi * 10^-prec; in fixed units q = qi * 10^(12-prec)
	qs := uint64(pow10Table[precision-prec])
	if qi > uint64(maxIntInFixed)/qs {
		return Decimal{}, Decimal{}, false
	}
	qf := int64(qi * qs)
	if neg {
		qf = -qf
	}
	// r = ri * 10^-prec in fixed units; sign follows the dividend.
	// ri < uy <= maxIntInFixed in magnitude.
	var rd Decimal
	p := pow10u[prec]
	if ri%p == 0 {
		rf := int64(ri / p)
		if x < 0 {
			rf = -rf
		}
		rd = Decimal{fixed: rf}
	} else {
		// needs 12+prec (<= 19) fractional digits: exact in udecimal
		fb, err := udecimal.NewFromInt64(signed64(ri, x < 0), uint8(precision+prec))
		if err != nil {
			return Decimal{}, Decimal{}, false
		}
		rd = newFromFallback(fb)
	}
	return Decimal{fixed: qf}, rd, true
}

func signed64(u uint64, neg bool) int64 {
	if neg {
		return -int64(u)
	}
	return int64(u)
}

// quoRemBig implements shopspring's exact QuoRem algorithm on big.Int parts.
// It is the general path covering fallback operands and any precision,
// including negative precision.
func quoRemBig(d, d2 Decimal, precision int32) (Decimal, Decimal) {
	dCoef, dExp := d.toBigParts()
	d2Coef, d2Exp := d2.toBigParts()
	q, r, scalerest := quoRemPartsRaw(dCoef, int64(dExp), d2Coef, int64(d2Exp), precision)
	return decimalFromBigParts(q, -int64(precision)), decimalFromBigParts(r, scalerest)
}

// quoRemPartsRaw computes q = trunc(a/b at precision) and the exact remainder
// r (as coefficient and exponent), mirroring shopspring's QuoRem.
func quoRemPartsRaw(dCoef *big.Int, dExp int64, d2Coef *big.Int, d2Exp int64, precision int32) (q, r *big.Int, scalerest int64) {
	if d2Coef.Sign() == 0 {
		panic("decimal division by 0")
	}
	scale := -int64(precision)
	e := dExp - d2Exp - scale
	// d = a 10^ea, d2 = b 10^eb
	var aa, bb big.Int
	if e < 0 {
		aa.Set(dCoef)
		bb.Mul(d2Coef, bigPow10(-e))
		scalerest = dExp
	} else {
		aa.Mul(dCoef, bigPow10(e))
		bb.Set(d2Coef)
		scalerest = scale + d2Exp
	}
	q, r = new(big.Int), new(big.Int)
	q.QuoRem(&aa, &bb, r)
	return q, r, scalerest
}

// divRoundParts implements shopspring's exact DivRound on big.Int parts:
// half-away-from-zero rounding of (aCoef 10^aExp) / (bCoef 10^bExp) at
// 10^-precision.
func divRoundParts(aCoef *big.Int, aExp int64, bCoef *big.Int, bExp int64, precision int32) Decimal {
	if bCoef.Sign() == 0 {
		panic("decimal division by 0")
	}
	scale := -int64(precision)
	e := aExp - bExp - scale
	var aa, bb big.Int
	if e < 0 {
		aa.Set(aCoef)
		bb.Mul(bCoef, bigPow10(-e))
	} else {
		aa.Mul(aCoef, bigPow10(e))
		bb.Set(bCoef)
	}
	var q, r big.Int
	q.QuoRem(&aa, &bb, &r)
	// round away from zero when 2|r| >= |b|
	r.Abs(&r)
	r.Lsh(&r, 1)
	if r.CmpAbs(&bb) >= 0 {
		if aCoef.Sign()*bCoef.Sign() < 0 {
			q.Sub(&q, oneBig)
		} else {
			q.Add(&q, oneBig)
		}
	}
	return decimalFromBigParts(&q, scale)
}

var oneBig = big.NewInt(1)

// divRoundBig is the Decimal-level wrapper over divRoundParts.
func divRoundBig(d, d2 Decimal, precision int32) Decimal {
	dCoef, dExp := d.toBigParts()
	d2Coef, d2Exp := d2.toBigParts()
	return divRoundParts(dCoef, int64(dExp), d2Coef, int64(d2Exp), precision)
}
