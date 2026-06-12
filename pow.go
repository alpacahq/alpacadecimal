package alpacadecimal

import (
	"math/big"
)

// optimized:
// Pow returns d to the power of d2, matching shopspring/decimal v1.4 semantics.
// When the exponent is negative the result has a maximum precision of
// PowPrecisionNegativeExponent places after the decimal point.
//
// Pow returns 0 (zero-value of Decimal) instead of an error for power
// operation edge cases:
//   - 0 ** 0 => undefined value
//   - 0 ** y, where y < 0 => infinity
//   - x ** y, where x < 0 and y is non-integer decimal => imaginary value
//
// Results requiring more than 19 fractional digits are truncated (package
// precision limit); shopspring keeps arbitrary precision.
func (d Decimal) Pow(d2 Decimal) Decimal {
	baseSign := d.Sign()
	expSign := d2.Sign()

	if baseSign == 0 {
		// 0**0 and 0**negative are undefined (0); 0**positive is 0
		return Decimal{}
	}
	if expSign == 0 {
		return NewFromInt(1)
	}

	if d2.IsInteger() {
		if n, ok := d2.intPartInt32(); ok {
			return d.powInt32(n)
		}
		// astronomically large integer exponent: powFrac's integer path
		// handles it with the same exact arithmetic (and the same cost)
		// as shopspring
	} else if baseSign == -1 {
		// negative base with non-integer exponent: imaginary
		return Decimal{}
	}

	return powFrac(d, d2)
}

// intPartInt32 returns the integer value of d when d is an integer that fits
// in an int32.
func (d Decimal) intPartInt32() (int32, bool) {
	if d.fallback == nil {
		// |fixed/scale| <= 9_223_372 always fits int32
		return int32(d.fixed / scale), true
	}
	v, err := d.fallback.Int64()
	if err != nil || v > 1<<31-1 || v < -(1<<31) {
		return 0, false
	}
	return int32(v), true
}

// powInt32 computes d**n for a nonzero integer exponent with shopspring
// PowInt32 semantics: exact exponentiation for n > 0; for n < 0 the exact
// power is inverted with DivRound at PowPrecisionNegativeExponent.
func (d Decimal) powInt32(n int32) Decimal {
	// fixed fast path: exact exponentiation by squaring within the fixed range
	if d.fallback == nil && n > 0 && n <= 64 {
		if f, ok := powFixed(d.fixed, uint32(n)); ok {
			return Decimal{fixed: f}
		}
	}

	m := int64(n)
	if m < 0 {
		m = -m
	}
	coef, exp := d.toBigParts()
	rcoef := new(big.Int).Exp(coef, big.NewInt(m), nil)
	rexp := int64(exp) * m
	if n > 0 {
		return decimalFromBigParts(rcoef, rexp)
	}
	// 1 / d**|n|, rounded at PowPrecisionNegativeExponent. Performed on exact
	// big parts so no precision is lost before the final rounding.
	return divRoundParts(big.NewInt(1), 0, rcoef, rexp, int32(PowPrecisionNegativeExponent))
}

// powFixed computes x**n in fixed representation via exponentiation by
// squaring, bailing out on any inexact or out-of-range intermediate.
func powFixed(x int64, n uint32) (int64, bool) {
	result := int64(scale) // 1.0
	base := x
	for {
		if n&1 == 1 {
			var ok bool
			result, ok = mul(result, base)
			if !ok {
				return 0, false
			}
		}
		n >>= 1
		if n == 0 {
			return result, true
		}
		var ok bool
		base, ok = mul(base, base)
		if !ok {
			return 0, false
		}
	}
}
