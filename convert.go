package alpacadecimal

import (
	"math"
	"math/big"

	zerodecimal "github.com/AlexandrosKyriakakis/zerodecimal"
)

// optimized:
// IntPart returns the integer component of the decimal.
func (d Decimal) IntPart() int64 {
	if d.fallback == nil {
		return d.fixed / scale
	}
	v, err := d.fallback.IntPart()
	if err == nil {
		return v
	}
	// Out of int64 range. shopspring's behavior (big.Int.Int64) is documented
	// as undefined here; return the truncated low 64 bits without panicking.
	neg, hi, lo, prec := d.fallback.ToHiLo()
	_, qlo, _ := div128by64(hi, lo, pow10u[prec])
	return signed64(qlo, neg)
}

// optimized:
// BigInt returns integer component of the decimal as a BigInt.
func (d Decimal) BigInt() *big.Int {
	if d.fallback == nil {
		return big.NewInt(d.fixed / scale)
	}
	coef, exp := d.toBigParts()
	if exp >= 0 {
		return coef
	}
	return coef.Quo(coef, bigPow10(int64(-exp)))
}

// fallback:
// BigFloat returns decimal as BigFloat.
// Be aware that casting decimal to BigFloat might cause a loss of precision.
func (d Decimal) BigFloat() *big.Float {
	f := &big.Float{}
	f.SetString(d.String())
	return f
}

// optimized:
// Rat returns a rational number representation of the decimal.
func (d Decimal) Rat() *big.Rat {
	if d.fallback == nil {
		return new(big.Rat).SetFrac64(d.fixed, scale)
	}
	coef, exp := d.toBigParts()
	if exp >= 0 {
		num := coef.Mul(coef, bigPow10(int64(exp)))
		return new(big.Rat).SetFrac(num, big.NewInt(1))
	}
	return new(big.Rat).SetFrac(coef, bigPow10(int64(-exp)))
}

// fiveTo12 = 5^12; fixed/10^12 is a dyadic rational (exactly representable in
// binary floating point) iff 5^12 divides fixed, since the remaining odd part
// fixed/5^12 always fits far below 2^53.
const fiveTo12 int64 = 244140625

// optimized:
// Float64 returns the nearest float64 value for d and a bool indicating
// whether f represents d exactly.
// For more details, see the documentation for big.Rat.Float64
func (d Decimal) Float64() (f float64, exact bool) {
	if d.fallback == nil {
		af := uabs(d.fixed)
		if af <= 1<<53 {
			// single correct rounding, identical to big.Rat.Float64
			return float64(d.fixed) / scale, d.fixed%fiveTo12 == 0
		}
	}
	return d.Rat().Float64()
}

// optimized:
// InexactFloat64 returns the nearest float64 value for d.
// It doesn't indicate if the returned value represents d exactly.
func (d Decimal) InexactFloat64() float64 {
	if d.fallback == nil {
		af := uabs(d.fixed)
		if af <= 1<<53 {
			// float64(d.fixed) is exact and the division is correctly
			// rounded: a single rounding, same as big.Rat.Float64.
			return float64(d.fixed) / scale
		}
	} else {
		neg, hi, lo, prec := d.fallback.ToHiLo()
		if hi == 0 && lo <= 1<<53 {
			// both operands convert exactly (10^prec <= 10^19 = 2^19*5^19,
			// 5^19 < 2^53), so the division is a single correct rounding
			f := float64(lo) / float64(pow10u[prec])
			if neg {
				return -f
			}
			return f
		}
	}
	f, _ := d.Float64()
	return f
}

// optimized:
// NumDigits returns the number of digits of the decimal coefficient (d.Value)
// Note: this is the number of digits of the internal coefficient, which (like
// Exponent and Coefficient) differs from shopspring for optimized values.
func (d Decimal) NumDigits() int {
	if d.fallback == nil {
		return digitCount(uabs(d.fixed))
	}
	_, hi, lo, _ := d.fallback.ToHiLo()
	if hi == 0 {
		return digitCount(lo)
	}
	coef, _ := d.toBigParts()
	if coef.Sign() == 0 {
		return 1
	}
	return len(new(big.Int).Abs(coef).String())
}

// optimized:
// Exponent returns the exponent, or scale component of the decimal.
func (d Decimal) Exponent() int32 {
	if d.fallback == nil {
		return -precision
	}
	return -int32(d.fallback.Prec())
}

// optimized:
// Coefficient returns the coefficient of the decimal. It is scaled by 10^Exponent()
func (d Decimal) Coefficient() *big.Int {
	if d.fallback == nil {
		return big.NewInt(d.fixed)
	}
	neg, hi, lo, _ := d.fallback.ToHiLo()
	return bigFromHiLo(neg, hi, lo)
}

// optimized:
// CoefficientInt64 returns the coefficient of the decimal as int64. It is scaled by 10^Exponent()
// If the coefficient cannot be represented in an int64, the result is undefined.
func (d Decimal) CoefficientInt64() int64 {
	if d.fallback == nil {
		return d.fixed
	}
	neg, hi, lo, _ := d.fallback.ToHiLo()
	if hi == 0 && lo <= math.MaxInt64 {
		return signed64(lo, neg)
	}
	return d.Coefficient().Int64()
}

// optimized:
// IsInteger returns true when decimal can be represented as an integer value, otherwise, it returns false.
func (d Decimal) IsInteger() bool {
	if d.fallback == nil {
		return d.fixed%scale == 0
	}
	return d.isIntegerSlow()
}

func (d Decimal) isIntegerSlow() bool {
	_, hi, lo, prec := d.fallback.ToHiLo()
	if prec == 0 {
		return true
	}
	_, _, r := div128by64(hi, lo, pow10u[prec])
	return r == 0
}

// optimized:
// Copy returns a copy of the decimal. Decimals are immutable, so unlike
// shopspring no new backing storage is required and the receiver is returned
// as-is.
func (d Decimal) Copy() Decimal {
	return d
}

// Rescale attempts to convert a fallback representation back to the optimized
// fixed representation when the value fits.
func (d Decimal) Rescale() Decimal {
	if d.fallback == nil {
		return d
	}
	return NewFromDecimal(*d.fallback)
}

// Extra API to support get internal state.
// e.g. might be useful for flatbuffers encode / decode.
func (d Decimal) GetFixed() int64 {
	return d.fixed
}

func (d Decimal) GetFallback() *zerodecimal.Decimal {
	if d.fallback == nil {
		return nil
	}
	fb := *d.fallback
	return &fb
}

func (d Decimal) IsOptimized() bool {
	return d.fallback == nil
}

// optimized:
// NewFromDecimal creates a new alpacadecimal.Decimal from a
// zerodecimal.Decimal, using the optimized fixed representation when the
// value fits.
func NewFromDecimal(u zerodecimal.Decimal) Decimal {
	if fixed, ok := tryFixedFromZD(u); ok {
		return Decimal{fixed: fixed}
	}
	return newFromFallback(u)
}

func tryFixedFromZD(u zerodecimal.Decimal) (fixed int64, ok bool) {
	neg, hi, lo, prec := u.ToHiLo()
	if hi != 0 {
		return 0, false
	}

	if lo == 0 {
		return 0, true
	}

	coef := lo
	p := int32(prec)

	// If precision is higher than the optimized precision, we can still
	// optimize when the extra trailing digits are zeros.
	if p > precision {
		divisor := pow10u[p-precision]
		if coef%divisor != 0 {
			return 0, false
		}
		coef /= divisor
		p = precision
	}

	multiplier := pow10u[precision-p]
	if coef > uint64(maxIntInFixed)/multiplier {
		return 0, false
	}

	fixed = int64(coef * multiplier)
	if neg {
		fixed = -fixed
	}
	return fixed, true
}
