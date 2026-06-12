package alpacadecimal

import (
	"fmt"
	"math/big"
	"math/bits"

	zerodecimal "github.com/AlexandrosKyriakakis/zerodecimal"
)

// pow10Table[i] = 10^i as int64, i in [0, 18].
var pow10Table = [19]int64{
	1e0, 1e1, 1e2, 1e3, 1e4,
	1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14,
	1e15, 1e16, 1e17, 1e18,
}

// pow10u[i] = 10^i as uint64, i in [0, 19].
var pow10u = [20]uint64{
	1e0, 1e1, 1e2, 1e3, 1e4,
	1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14,
	1e15, 1e16, 1e17, 1e18, 1e19,
}

// zdPow10[i] = 10^i as zerodecimal.Decimal, i in [0, 19].
var zdPow10 = [20]zerodecimal.Decimal{}

func init() {
	for i := range zdPow10 {
		zdPow10[i] = zerodecimal.MustNew(1, int32(i))
	}
}

func uabs(x int64) uint64 {
	if x < 0 {
		return uint64(-x)
	}
	return uint64(x)
}

// digitCount returns the number of decimal digits in v (digitCount(0) == 1).
func digitCount(v uint64) int {
	n := 1
	if v >= 1e16 {
		v /= 1e16
		n += 16
	}
	if v >= 1e8 {
		v /= 1e8
		n += 8
	}
	if v >= 1e4 {
		v /= 1e4
		n += 4
	}
	if v >= 1e2 {
		v /= 1e2
		n += 2
	}
	if v >= 1e1 {
		n++
	}
	return n
}

// Constant-divisor division of a 128-bit value by 1e12 (Möller–Granlund 2-by-1
// division with a precomputed reciprocal; faster than bits.Div64 on arm64 and
// portable). Requires hi < 1e12.
const (
	div1e12Norm  uint64 = 1e12 << 24          // 1e12 normalized to have the top bit set
	div1e12Recip uint64 = 1835665529942118807 // floor((2^128-1)/div1e12Norm) - 2^64
	div1e12Shift        = 24                  // leading zeros of 1e12
)

// divu128By1e12 returns the quotient and remainder of (hi:lo) / 1e12.
// Requires hi < 1e12 so the quotient fits in 64 bits.
func divu128By1e12(hi, lo uint64) (q, r uint64) {
	uh := hi<<div1e12Shift | lo>>(64-div1e12Shift)
	ul := lo << div1e12Shift
	qh, ql := bits.Mul64(div1e12Recip, uh)
	ql, c := bits.Add64(ql, ul, 0)
	qh, _ = bits.Add64(qh, uh, c)
	qh++
	r = ul - qh*div1e12Norm
	if r > ql {
		qh--
		r += div1e12Norm
	}
	if r >= div1e12Norm {
		qh++
		r -= div1e12Norm
	}
	return qh, r >> div1e12Shift
}

// div128by64 divides the 128-bit value (hi:lo) by y, allowing hi >= y
// (i.e. quotients wider than 64 bits). Returns the 128-bit quotient and the
// remainder.
func div128by64(hi, lo, y uint64) (qhi, qlo, r uint64) {
	qhi = hi / y
	qlo, r = bits.Div64(hi%y, lo, y)
	return qhi, qlo, r
}

// bigFromHiLo builds a big.Int from a 128-bit magnitude and a sign.
func bigFromHiLo(neg bool, hi, lo uint64) *big.Int {
	var bi *big.Int
	if hi == 0 {
		bi = new(big.Int).SetUint64(lo)
	} else {
		bi = new(big.Int).SetUint64(hi)
		bi.Lsh(bi, 64)
		bi.Or(bi, new(big.Int).SetUint64(lo))
	}
	if neg {
		bi.Neg(bi)
	}
	return bi
}

// outOfDomainMsg is the panic message for values that cannot be represented
// at all: zerodecimal's coefficient is a 128-bit integer, so |value| >= 2^128
// (~3.4e38) is out of domain even at precision 0. Infallible API paths panic
// with this message; parse/decode paths surface errOutOfDomain instead.
const outOfDomainMsg = "alpacadecimal: value exceeds the 128-bit decimal domain (|value| >= 2^128)"

var errOutOfDomain = fmt.Errorf("value exceeds the 128-bit decimal domain (|value| >= 2^128)")

// zdFromBigOK constructs a zerodecimal.Decimal with value coef * 10^-prec
// where coef is an arbitrary-magnitude big.Int and prec <= 19. When the
// coefficient exceeds 128 bits, fractional digits are truncated toward zero
// (degrading prec) until it fits — extending the package convention of
// truncating fractional digits beyond 19. ok is false when even the integer
// part does not fit a 128-bit coefficient.
func zdFromBigOK(coef *big.Int, prec uint8) (zerodecimal.Decimal, bool) {
	neg := coef.Sign() < 0
	abs := coef
	if neg {
		abs = new(big.Int).Neg(coef)
	}
	if abs.BitLen() > 128 {
		if prec == 0 {
			return zerodecimal.Decimal{}, false
		}
		if abs == coef {
			abs = new(big.Int).Set(coef) // do not mutate the caller's value
		}
		ten := big.NewInt(10)
		for abs.BitLen() > 128 && prec > 0 {
			abs.Quo(abs, ten)
			prec--
		}
		if abs.BitLen() > 128 {
			return zerodecimal.Decimal{}, false
		}
	}
	var hi, lo uint64
	words := abs.Bits()
	if len(words) > 0 {
		lo = uint64(words[0])
	}
	if len(words) > 1 {
		hi = uint64(words[1])
	}
	d, err := zerodecimal.NewFromHiLo(neg, hi, lo, prec)
	if err != nil {
		return zerodecimal.Decimal{}, false
	}
	return d, true
}

// zdFromBig is zdFromBigOK for infallible call paths: it panics when the
// value is out of the 128-bit domain.
func zdFromBig(coef *big.Int, prec uint8) zerodecimal.Decimal {
	d, ok := zdFromBigOK(coef, prec)
	if !ok {
		panic(outOfDomainMsg)
	}
	return d
}

// toBigParts returns the exact value of d as coef * 10^exp.
func (d Decimal) toBigParts() (coef *big.Int, exp int32) {
	if d.fallback == nil {
		return big.NewInt(d.fixed), -precision
	}
	neg, hi, lo, prec := d.fallback.ToHiLo()
	return bigFromHiLo(neg, hi, lo), -int32(prec)
}

// decimalFromBigParts converts coef * 10^exp into a Decimal, truncating
// fractional digits beyond the fallback's 19-digit limit (consistent with the
// package-wide truncation convention for out-of-range precision). It panics
// when the value exceeds the 128-bit coefficient domain; parse and decode
// paths use decimalFromBigPartsErr instead.
func decimalFromBigParts(coef *big.Int, exp int64) Decimal {
	d, err := decimalFromBigPartsErr(coef, exp)
	if err != nil {
		panic(outOfDomainMsg)
	}
	return d
}

// decimalFromBigPartsErr is decimalFromBigParts returning errOutOfDomain
// instead of panicking when the value cannot be represented.
func decimalFromBigPartsErr(coef *big.Int, exp int64) (Decimal, error) {
	if coef.Sign() == 0 {
		return Zero, nil
	}
	// Fixed fast path for small coefficients.
	if coef.IsInt64() {
		return newFromInt64ExpErr(coef.Int64(), exp)
	}
	if exp > 0 {
		shifted := new(big.Int).Mul(coef, bigPow10(exp))
		if zd, ok := zdFromBigOK(shifted, 0); ok {
			return NewFromDecimal(zd), nil
		}
		return Decimal{}, errOutOfDomain
	}
	if exp >= -19 {
		if zd, ok := zdFromBigOK(coef, uint8(-exp)); ok {
			return NewFromDecimal(zd), nil
		}
		return Decimal{}, errOutOfDomain
	}
	// Truncate digits beyond 19 fractional places.
	shift := -exp - 19
	// 10^shift certainly exceeds |coef| when shift > digits(coef); bail out
	// before materializing a potentially enormous power of ten.
	if shift > int64(coef.BitLen()/3)+2 {
		return Zero, nil
	}
	truncated := new(big.Int).Quo(coef, bigPow10(shift))
	if truncated.Sign() == 0 {
		return Zero, nil
	}
	if zd, ok := zdFromBigOK(truncated, 19); ok {
		return NewFromDecimal(zd), nil
	}
	return Decimal{}, errOutOfDomain
}

// maxMaterializedExp bounds eager materialization of 10^n coefficients
// (Shift, New with huge exponents, decoding). shopspring stores exponents
// lazily so it has no such limit; a 100k-digit coefficient (~42 KB,
// microseconds to build) is far beyond any sane decimal while still
// preventing a single call from pinning a CPU for minutes.
const maxMaterializedExp = 100_000

// bigPow10 returns 10^n as a big.Int for n >= 0.
func bigPow10(n int64) *big.Int {
	if n <= 18 {
		return big.NewInt(pow10Table[n])
	}
	if n > maxMaterializedExp {
		panic(fmt.Sprintf("alpacadecimal: exponent %d exceeds the supported materialization limit %d", n, maxMaterializedExp))
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(n), nil)
}

// newFromInt64Exp converts v * 10^exp into a Decimal for any exponent,
// truncating digits beyond 19 fractional places. It panics when exp exceeds
// maxMaterializedExp or the value exceeds the 128-bit coefficient domain.
func newFromInt64Exp(v int64, exp int64) Decimal {
	d, err := newFromInt64ExpErr(v, exp)
	if err != nil {
		panic(outOfDomainMsg)
	}
	return d
}

// newFromInt64ExpErr is newFromInt64Exp returning errOutOfDomain instead of
// panicking when the value cannot be represented.
func newFromInt64ExpErr(v int64, exp int64) (Decimal, error) {
	if v == 0 {
		return Zero, nil
	}
	if fixed, ok := tryFixedInt64Exp(v, exp); ok {
		return Decimal{fixed: fixed}, nil
	}
	if exp > 0 {
		if exp <= 18 {
			// Exact: |v| * 10^18 < 2^64 * 10^18 < 2^128 always fits.
			return newFromFallback(zerodecimal.MustNew(v, int32(exp))), nil
		}
		shifted := new(big.Int).Mul(big.NewInt(v), bigPow10(exp))
		if zd, ok := zdFromBigOK(shifted, 0); ok {
			return newFromFallback(zd), nil
		}
		return Decimal{}, errOutOfDomain
	}
	if exp >= -19 {
		fb, _ := zerodecimal.New(v, int32(exp))
		return newFromFallback(fb), nil
	}
	// exp < -19: truncate digits beyond 19 fractional places.
	shift := -exp - 19
	if shift > 18 {
		return Zero, nil
	}
	v /= pow10Table[shift]
	if v == 0 {
		return Zero, nil
	}
	fb, _ := zerodecimal.New(v, -19)
	return newFromFallback(fb), nil
}

// tryFixedInt64Exp reports whether v * 10^exp is representable in the fixed
// (12 fractional digit) form, and returns that form.
func tryFixedInt64Exp(v int64, exp int64) (int64, bool) {
	if v == 0 {
		return 0, true
	}
	e := exp + precision // power of ten applied to v in fixed units
	switch {
	case e < 0:
		// Representable only when v is divisible by 10^-e.
		if e < -18 {
			return 0, false
		}
		s := pow10Table[-e]
		if v%s != 0 {
			return 0, false
		}
		return v / s, true
	case e <= 18:
		s := pow10Table[e]
		if v > maxIntInFixed/s || v < minIntInFixed/s {
			return 0, false
		}
		return v * s, true
	default:
		return 0, false
	}
}
