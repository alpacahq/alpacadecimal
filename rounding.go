package alpacadecimal

import (
	"fmt"
	"math/big"
	"math/bits"

	zerodecimal "github.com/AlexandrosKyriakakis/zerodecimal"
)

type roundMode int

const (
	modeHAZ   roundMode = iota // half away from zero (Round)
	modeHTE                    // half to even (RoundBank)
	modeDown                   // toward zero (RoundDown, Truncate)
	modeAway                   // away from zero (RoundUp)
	modeCeil                   // toward +infinity (RoundCeil)
	modeFloor                  // toward -infinity (RoundFloor)
)

// optimized:
// Round rounds the decimal to places decimal places.
// If places < 0, it will round the integer part to the nearest 10^(-places).
func (d Decimal) Round(places int32) Decimal {
	if d.fallback == nil {
		if places >= precision {
			return d
		}
		if places >= 0 {
			// dedicated fast path: no overflow is possible here
			s := pow10Table[precision-places]
			m := d.fixed % s
			if m == 0 {
				return d
			}
			f := d.fixed - m
			if 2*m >= s {
				f += s
			} else if 2*m <= -s {
				f -= s
			}
			return Decimal{fixed: f}
		}
		return roundFixed(d, places, modeHAZ)
	}
	if places >= 0 {
		if places >= int32(d.fallback.Prec()) {
			return d // no fractional digits to round
		}
		return NewFromDecimal(d.fallback.Round(uint8(places)))
	}
	return roundBigGeneric(d, places, modeHAZ)
}

// optimized:
// RoundBank rounds the decimal to places decimal places.
// If the final digit to round is equidistant from the nearest two integers the
// rounded value is taken as the even number
//
// If places < 0, it will round the integer part to the nearest 10^(-places).
func (d Decimal) RoundBank(places int32) Decimal {
	if d.fallback == nil {
		if places >= precision {
			return d
		}
		if places >= 0 {
			s := pow10Table[precision-places]
			m := d.fixed % s
			if m == 0 {
				return d
			}
			f := d.fixed - m
			m2 := 2 * m
			if m2 > s || m2 < -s || ((m2 == s || m2 == -s) && (f/s)%2 != 0) {
				if m > 0 {
					f += s
				} else {
					f -= s
				}
			}
			return Decimal{fixed: f}
		}
		return roundFixed(d, places, modeHTE)
	}
	if places >= 0 {
		if places >= int32(d.fallback.Prec()) {
			return d // no fractional digits to round
		}
		return NewFromDecimal(d.fallback.RoundBank(uint8(places)))
	}
	return roundBigGeneric(d, places, modeHTE)
}

// optimized:
// RoundDown rounds the decimal towards zero.
//
// Example:
//
//	NewFromFloat(545).RoundDown(-2).String()   // output: "500"
//	NewFromFloat(-500).RoundDown(-2).String()  // output: "-500"
//	NewFromFloat(1.1001).RoundDown(2).String() // output: "1.1"
//	NewFromFloat(-1.454).RoundDown(1).String() // output: "-1.4"
func (d Decimal) RoundDown(places int32) Decimal {
	if d.fallback == nil {
		return roundFixed(d, places, modeDown)
	}
	if places >= 0 {
		if places >= int32(d.fallback.Prec()) {
			return d // no fractional digits to round
		}
		return NewFromDecimal(d.fallback.RoundDown(uint8(places)))
	}
	return roundBigGeneric(d, places, modeDown)
}

// optimized:
// RoundUp rounds the decimal away from zero.
//
// Example:
//
//	NewFromFloat(545).RoundUp(-2).String()   // output: "600"
//	NewFromFloat(500).RoundUp(-2).String()   // output: "500"
//	NewFromFloat(1.1001).RoundUp(2).String() // output: "1.11"
//	NewFromFloat(-1.454).RoundUp(1).String() // output: "-1.5"
func (d Decimal) RoundUp(places int32) Decimal {
	if d.fallback == nil {
		return roundFixed(d, places, modeAway)
	}
	if places >= 0 {
		if places >= int32(d.fallback.Prec()) {
			return d // no fractional digits to round
		}
		return NewFromDecimal(d.fallback.RoundUp(uint8(places)))
	}
	return roundBigGeneric(d, places, modeAway)
}

// optimized:
// RoundCeil rounds the decimal towards +infinity.
//
// Example:
//
//	NewFromFloat(545).RoundCeil(-2).String()   // output: "600"
//	NewFromFloat(500).RoundCeil(-2).String()   // output: "500"
//	NewFromFloat(1.1001).RoundCeil(2).String() // output: "1.11"
//	NewFromFloat(-1.454).RoundCeil(1).String() // output: "-1.4"
func (d Decimal) RoundCeil(places int32) Decimal {
	if d.fallback == nil {
		if places >= precision {
			return d
		}
		if places >= 0 {
			s := pow10Table[precision-places]
			m := d.fixed % s
			if m <= 0 {
				return Decimal{fixed: d.fixed - m}
			}
			return Decimal{fixed: d.fixed - m + s}
		}
		return roundFixed(d, places, modeCeil)
	}
	if places >= 0 {
		if places >= int32(d.fallback.Prec()) {
			return d // no fractional digits to round
		}
		return NewFromDecimal(d.fallback.RoundCeil(uint8(places)))
	}
	return roundBigGeneric(d, places, modeCeil)
}

// optimized:
// RoundFloor rounds the decimal towards -infinity.
//
// Example:
//
//	NewFromFloat(545).RoundFloor(-2).String()   // output: "500"
//	NewFromFloat(-500).RoundFloor(-2).String()  // output: "-500"
//	NewFromFloat(1.1001).RoundFloor(2).String() // output: "1.1"
//	NewFromFloat(-1.454).RoundFloor(1).String() // output: "-1.5"
func (d Decimal) RoundFloor(places int32) Decimal {
	if d.fallback == nil {
		if places >= precision {
			return d
		}
		if places >= 0 {
			s := pow10Table[precision-places]
			m := d.fixed % s
			if m >= 0 {
				return Decimal{fixed: d.fixed - m}
			}
			return Decimal{fixed: d.fixed - m - s}
		}
		return roundFixed(d, places, modeFloor)
	}
	if places >= 0 {
		if places >= int32(d.fallback.Prec()) {
			return d // no fractional digits to round
		}
		return NewFromDecimal(d.fallback.RoundFloor(uint8(places)))
	}
	return roundBigGeneric(d, places, modeFloor)
}

// optimized:
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
		panic(fmt.Sprintf("Decimal does not support this Cash rounding interval `%d`. Supported: 5, 10, 25, 50, 100", interval))
	}
	return d.Mul(multiplier).Round(0).Div(multiplier).Truncate(2)
}

// optimized:
// Ceil returns the nearest integer value greater than or equal to d.
func (d Decimal) Ceil() Decimal {
	if d.fallback == nil {
		m := d.fixed % scale
		if m == 0 {
			return d
		}
		if m > 0 {
			return Decimal{fixed: d.fixed - m + scale}
		}
		return Decimal{fixed: d.fixed - m}
	}
	return NewFromDecimal(d.fallback.Ceil())
}

// optimized:
// Floor returns the nearest integer value less than or equal to d.
func (d Decimal) Floor() Decimal {
	if d.fallback == nil {
		m := d.fixed % scale
		if m == 0 {
			return d
		}
		if m > 0 {
			return Decimal{fixed: d.fixed - m}
		}
		return Decimal{fixed: d.fixed - m - scale}
	}
	return NewFromDecimal(d.fallback.Floor())
}

// optimized:
// Truncate truncates off digits from the number, without rounding.
//
// NOTE: precision is the last digit that will not be truncated (must be >= 0;
// negative precision is a no-op, like shopspring).
func (d Decimal) Truncate(precision int32) Decimal {
	if d.fallback == nil {
		if precision >= 12 || precision < 0 {
			return d
		}
		s := pow10Table[12-precision]
		return Decimal{fixed: d.fixed - d.fixed%s}
	}
	if precision < 0 || precision >= int32(d.fallback.Prec()) {
		return d
	}
	return NewFromDecimal(d.fallback.Truncate(uint8(precision)))
}

// roundFixed rounds a fixed-representation decimal at 10^-places in the given
// mode. Handles every places value without overflow.
func roundFixed(d Decimal, places int32, mode roundMode) Decimal {
	if places >= precision {
		return d
	}
	x := d.fixed
	if places >= -6 {
		s := pow10Table[precision-places]
		m := x % s
		if m == 0 {
			return d
		}
		base := x - m
		var away bool
		switch mode {
		case modeHAZ:
			away = 2*m >= s || 2*m <= -s
		case modeHTE:
			m2 := 2 * m
			if m2 > s || m2 < -s {
				away = true
			} else if m2 == s || m2 == -s {
				away = (base/s)%2 != 0 // tie: round to even multiple
			}
		case modeDown:
			away = false
		case modeAway:
			away = true
		case modeCeil:
			away = m > 0
		case modeFloor:
			away = m < 0
		}
		if !away {
			return Decimal{fixed: base}
		}
		q := base / s
		if m > 0 {
			q++
		} else {
			q--
		}
		// q * s can exceed the fixed range only for places < 0
		if q <= maxIntInFixed/s && q >= minIntInFixed/s {
			return Decimal{fixed: q * s}
		}
		return newFromInt64Exp(q, -int64(places))
	}

	// places <= -7: |d| < 10^7 <= 10^-places, so the result is 0 or ±10^-places.
	if x == 0 {
		return d
	}
	var away bool
	switch mode {
	case modeHAZ:
		// |x| >= 0.5 * 10^(12-places) ⇔ 2|x| >= 10^(12-places)
		away = places == -7 && 2*uabs(x) >= pow10u[19]
	case modeHTE:
		// the tie (2|x| == 10^19) rounds to the even multiple, which is 0
		away = places == -7 && 2*uabs(x) > pow10u[19]
	case modeDown:
		away = false
	case modeAway:
		away = true
	case modeCeil:
		away = x > 0
	case modeFloor:
		away = x < 0
	}
	if !away {
		return Zero
	}
	if x > 0 {
		return newFromInt64Exp(1, -int64(places))
	}
	return newFromInt64Exp(-1, -int64(places))
}

// roundFallbackU128 rounds a fallback value at negative places using a single
// 128-by-64 division when the divisor 10^(prec - places) fits in a uint64.
func roundFallbackU128(d Decimal, places int32, mode roundMode) (Decimal, bool) {
	neg, hi, lo, prec := d.fallback.ToHiLo()
	s := int32(prec) - places // digits to clear off the coefficient
	if s <= 0 {
		return d, true
	}
	if s > 19 {
		return Decimal{}, false
	}
	p := pow10u[s]
	qhi, qlo, r := div128by64(hi, lo, p)
	if r == 0 {
		return d, true
	}
	var away bool
	switch mode {
	case modeHAZ:
		away = r >= (p+1)/2 // p is even here (a power of ten >= 10)
	case modeHTE:
		half := p / 2
		away = r > half || (r == half && qlo&1 != 0)
	case modeDown:
		away = false
	case modeAway:
		away = true
	case modeCeil:
		away = !neg
	case modeFloor:
		away = neg
	}
	if away {
		var c uint64
		qlo, c = bits.Add64(qlo, 1, 0)
		qhi += c
	}
	// scale back up: q * 10^s as a 128-bit product
	mhi, mlo := bits.Mul64(qlo, p)
	if qhi != 0 {
		// qhi * p must not overflow the high word
		h2, l2 := bits.Mul64(qhi, p)
		if h2 != 0 {
			return Decimal{}, false
		}
		var c uint64
		mhi, c = bits.Add64(mhi, l2, 0)
		if c != 0 {
			return Decimal{}, false
		}
	}
	u, err := zerodecimal.NewFromHiLo(neg, mhi, mlo, prec)
	if err != nil {
		return Decimal{}, false
	}
	return NewFromDecimal(u), true
}

// roundBigGeneric rounds at 10^-places (places typically < 0 here) using
// exact big.Int arithmetic, mirroring shopspring semantics for all modes.
// A u128 fast path covers fallback values whose total shift fits 10^19.
func roundBigGeneric(d Decimal, places int32, mode roundMode) Decimal {
	if d.fallback != nil {
		if r, ok := roundFallbackU128(d, places, mode); ok {
			return r
		}
	}
	coef, exp := d.toBigParts()
	shift := -int64(places) - int64(exp) // digits to drop
	if shift <= 0 || coef.Sign() == 0 {
		return d
	}
	p := bigPow10(shift)
	q, r := new(big.Int).QuoRem(coef, p, new(big.Int))
	if r.Sign() == 0 {
		return d
	}
	sign := coef.Sign()
	var away bool
	switch mode {
	case modeHAZ, modeHTE:
		r2 := new(big.Int).Abs(r)
		r2.Lsh(r2, 1)
		c := r2.Cmp(p)
		if c > 0 {
			away = true
		} else if c == 0 {
			if mode == modeHAZ {
				away = true
			} else {
				away = q.Bit(0) != 0
			}
		}
	case modeDown:
		away = false
	case modeAway:
		away = true
	case modeCeil:
		away = sign > 0
	case modeFloor:
		away = sign < 0
	}
	if away {
		if sign > 0 {
			q.Add(q, big.NewInt(1))
		} else {
			q.Sub(q, big.NewInt(1))
		}
	}
	return decimalFromBigParts(q, -int64(places))
}
