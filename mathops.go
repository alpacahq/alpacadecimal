package alpacadecimal

// Native port of shopspring/decimal v1.4.0's Pow machinery (PowBigInt, Ln,
// ExpTaylor) operating on an exact arbitrary-precision decimal, so that
// fractional exponents produce digit-identical results to shopspring without
// depending on it. The algorithms are ported faithfully — including precision
// choices — from shopspring/decimal (MIT licensed).

import (
	"errors"
	"math"
	"math/big"
)

// bigDec is an exact decimal: value * 10^exp. Unlike Decimal it never
// truncates, which the iterative algorithms below rely on.
type bigDec struct {
	value *big.Int
	exp   int32
}

var (
	bdOneInt  = big.NewInt(1)
	bdFiveInt = big.NewInt(5)
	bdTenInt  = big.NewInt(10)
)

func bdNew(value int64, exp int32) bigDec {
	return bigDec{value: big.NewInt(value), exp: exp}
}

// canonicalBigDec converts d to a bigDec with a trailing-zero-trimmed
// coefficient — the representation shopspring users get from NewFromString,
// which shopspring's Pow uses to pick working precisions.
func canonicalBigDec(d Decimal) bigDec {
	coef, exp := d.toBigParts()
	if coef.Sign() != 0 {
		var r big.Int
		for exp < 0 {
			var q big.Int
			q.QuoRem(coef, bdTenInt, &r)
			if r.Sign() != 0 {
				break
			}
			coef.Set(&q)
			exp++
		}
	} else {
		exp = 0
	}
	return bigDec{value: coef, exp: exp}
}

// rescale mirrors shopspring's rescale: truncating when exp grows.
func (d bigDec) rescale(exp int32) bigDec {
	if d.exp == exp {
		return bigDec{value: new(big.Int).Set(d.value), exp: exp}
	}
	diff := int64(exp) - int64(d.exp)
	if diff < 0 {
		diff = -diff
	}
	value := new(big.Int).Set(d.value)
	expScale := new(big.Int).Exp(bdTenInt, big.NewInt(diff), nil)
	if exp > d.exp {
		value.Quo(value, expScale)
	} else {
		value.Mul(value, expScale)
	}
	return bigDec{value: value, exp: exp}
}

func bdRescalePair(d1, d2 bigDec) (bigDec, bigDec) {
	if d1.exp < d2.exp {
		return d1, d2.rescale(d1.exp)
	} else if d1.exp > d2.exp {
		return d1.rescale(d2.exp), d2
	}
	return d1, d2
}

func (d bigDec) add(d2 bigDec) bigDec {
	rd, rd2 := bdRescalePair(d, d2)
	return bigDec{value: new(big.Int).Add(rd.value, rd2.value), exp: rd.exp}
}

func (d bigDec) sub(d2 bigDec) bigDec {
	rd, rd2 := bdRescalePair(d, d2)
	return bigDec{value: new(big.Int).Sub(rd.value, rd2.value), exp: rd.exp}
}

func (d bigDec) mul(d2 bigDec) bigDec {
	return bigDec{
		value: new(big.Int).Mul(d.value, d2.value),
		exp:   d.exp + d2.exp,
	}
}

func (d bigDec) abs() bigDec {
	return bigDec{value: new(big.Int).Abs(d.value), exp: d.exp}
}

func (d bigDec) cmp(d2 bigDec) int {
	rd, rd2 := bdRescalePair(d, d2)
	return rd.value.Cmp(rd2.value)
}

func (d bigDec) sign() int {
	return d.value.Sign()
}

func (d bigDec) isZero() bool {
	return d.value.Sign() == 0
}

// round ports shopspring's Round (half away from zero).
func (d bigDec) round(places int32) bigDec {
	if d.exp == -places {
		return d
	}
	ret := d.rescale(-places - 1)
	if ret.value.Sign() < 0 {
		ret.value.Sub(ret.value, bdFiveInt)
	} else {
		ret.value.Add(ret.value, bdFiveInt)
	}
	var m big.Int
	ret.value.DivMod(ret.value, bdTenInt, &m)
	ret.exp++
	if ret.value.Sign() < 0 && m.Sign() != 0 {
		ret.value.Add(ret.value, bdOneInt)
	}
	return ret
}

// quoRem ports shopspring's QuoRem.
func (d bigDec) quoRem(d2 bigDec, precision int32) (bigDec, bigDec) {
	q, r, scalerest := quoRemPartsRaw(d.value, int64(d.exp), d2.value, int64(d2.exp), precision)
	return bigDec{value: q, exp: -precision}, bigDec{value: r, exp: int32(scalerest)}
}

// divRound ports shopspring's DivRound (half away from zero at precision).
func (d bigDec) divRound(d2 bigDec, precision int32) bigDec {
	q, r := d.quoRem(d2, precision)
	var rv2 big.Int
	rv2.Abs(r.value)
	rv2.Lsh(&rv2, 1)
	r2 := bigDec{value: &rv2, exp: r.exp + precision}
	c := r2.cmp(d2.abs())
	if c < 0 {
		return q
	}
	if d.value.Sign()*d2.value.Sign() < 0 {
		return bigDec{value: new(big.Int).Sub(q.value, bdOneInt), exp: q.exp}
	}
	return bigDec{value: new(big.Int).Add(q.value, bdOneInt), exp: q.exp}
}

// numDigits ports shopspring's NumDigits verbatim — including its float fast
// path, which undercounts exact powers of ten (NumDigits(10^15) == 15): Pow
// derives working precisions from it, so the quirk must match for digit
// parity.
func (d bigDec) numDigits() int {
	if d.value.IsInt64() {
		i64 := d.value.Int64()
		// restrict the fast path to integers with exact float64 conversion
		if i64 <= (1<<53) && i64 >= -(1<<53) {
			if i64 == 0 {
				return 1
			}
			return int(math.Log10(math.Abs(float64(i64)))) + 1
		}
	}

	estimatedNumDigits := int(float64(d.value.BitLen()) / math.Log2(10))

	// estimatedNumDigits (lg10) may be off by 1, need to verify
	digitsBigInt := big.NewInt(int64(estimatedNumDigits))
	errorCorrectionUnit := digitsBigInt.Exp(bdTenInt, digitsBigInt, nil)

	if d.value.CmpAbs(errorCorrectionUnit) >= 0 {
		return estimatedNumDigits + 1
	}
	return estimatedNumDigits
}

func (d bigDec) inexactFloat64() float64 {
	denomExp := int64(-d.exp)
	var rat *big.Rat
	if denomExp >= 0 {
		rat = new(big.Rat).SetFrac(d.value, bigPow10(denomExp))
	} else {
		num := new(big.Int).Mul(d.value, bigPow10(-denomExp))
		rat = new(big.Rat).SetFrac(num, bdOneInt)
	}
	f, _ := rat.Float64()
	return f
}

// bdFromFloat converts a float64 to its shortest-representation bigDec
// (shopspring's NewFromFloat), used for Ln's initial estimate.
func bdFromFloat(val float64) bigDec {
	if val == 0 {
		return bdNew(0, 0)
	}
	bits := math.Float64bits(val)
	exp := int(bits>>float64info.mantbits) & (1<<float64info.expbits - 1)
	mant := bits & (uint64(1)<<float64info.mantbits - 1)
	if exp == 0 {
		exp++
	} else {
		mant |= uint64(1) << float64info.mantbits
	}
	exp += float64info.bias

	var dd decimal
	dd.Assign(mant)
	dd.Shift(exp - int(float64info.mantbits))
	dd.neg = bits>>(float64info.expbits+float64info.mantbits) != 0

	roundShortest(&dd, mant, exp, &float64info)
	var m int64
	for i := 0; i < dd.nd; i++ {
		m = m*10 + int64(dd.d[i]-'0')
	}
	if dd.neg {
		m = -m
	}
	return bdNew(m, int32(dd.dp)-int32(dd.nd))
}

// powBigInt ports shopspring's powBigIntWithPrecision: exact exponentiation
// by squaring; negative exponents invert with DivRound at precision.
func (d bigDec) powBigInt(exp *big.Int, precision int32) bigDec {
	tmpExp := new(big.Int).Set(exp)
	isExpNeg := exp.Sign() < 0
	if isExpNeg {
		tmpExp.Abs(tmpExp)
	}
	n, result := d, bdNew(1, 0)
	for tmpExp.Sign() > 0 {
		if tmpExp.Bit(0) == 1 {
			result = result.mul(n)
		}
		tmpExp.Rsh(tmpExp, 1)
		if tmpExp.Sign() > 0 {
			n = n.mul(n)
		}
	}
	if isExpNeg {
		return bdNew(1, 0).divRound(result, precision)
	}
	return result
}

// expTaylor ports shopspring's ExpTaylor.
func (d bigDec) expTaylor(precision int32) (bigDec, error) {
	if d.isZero() {
		return bdNew(1, 0).round(precision), nil
	}

	var epsilon bigDec
	var divPrecision int32
	if precision < 0 {
		epsilon = bdNew(1, -1)
		divPrecision = 8
	} else {
		epsilon = bdNew(1, -precision-1)
		divPrecision = precision + 1
	}

	decAbs := d.abs()
	pow := d.abs()
	factorial := bdNew(1, 0)
	result := bdNew(1, 0)

	for i := int64(1); ; {
		step := pow.divRound(factorial, divPrecision)
		result = result.add(step)

		// stop the Taylor series when the current step is below epsilon
		if step.cmp(epsilon) < 0 {
			break
		}

		pow = pow.mul(decAbs)
		i++
		// factorial(i) computed incrementally — identical values to
		// shopspring's cached factorials
		factorial = factorial.mul(bdNew(i, 0))
	}

	if d.sign() < 0 {
		result = bdNew(1, 0).divRound(result, precision+1)
	}
	return result.round(precision), nil
}

var errLnNegative = errors.New("cannot calculate natural logarithm for negative decimals")
var errLnZero = errors.New("cannot represent natural logarithm of 0, result: -infinity")

// ln ports shopspring's Ln.
func (d bigDec) ln(precision int32) (bigDec, error) {
	if d.sign() < 0 {
		return bigDec{}, errLnNegative
	}
	if d.isZero() {
		return bigDec{}, errLnZero
	}

	calcPrecision := precision + 2
	z := bigDec{value: new(big.Int).Set(d.value), exp: d.exp}

	var comp1, comp2, comp3, comp4 bigDec
	reduceAdjust := bdNew(0, 0)
	comp1 = z.sub(bdNew(1, 0))
	comp3 = bdNew(1, -1)

	// for decimals in range [0.9, 1.1] where ln(d) is close to 0
	usePowerSeries := false

	if comp1.abs().cmp(comp3) <= 0 {
		usePowerSeries = true
	} else {
		// reduce the input to the range [0.1, 1)
		expDelta := int32(z.numDigits()) + z.exp
		z.exp -= expDelta

		// compensate later with ln(10^expDelta) = expDelta * ln(10)
		ln10approx := ln10Approx(calcPrecision)
		reduceAdjust = bdNew(int64(expDelta), 0).mul(ln10approx)

		comp1 = z.sub(bdNew(1, 0))

		if comp1.abs().cmp(comp3) <= 0 {
			usePowerSeries = true
		} else {
			// initial estimate using floats
			comp1 = bdFromFloat(math.Log(z.inexactFloat64()))
		}
	}

	epsilon := bdNew(1, -calcPrecision)

	if usePowerSeries {
		// power series: ln(z+1) = 2 sum [ 1/(2n+1) * (z/(z+2))^(2n+1) ]
		comp2 = comp1.add(bdNew(2, 0))
		comp3 = comp1.divRound(comp2, calcPrecision)
		comp1 = comp3.add(comp3)
		comp2 = comp1

		for n := 1; ; n++ {
			comp2 = comp2.mul(comp3).mul(comp3)
			comp4 = comp2.divRound(bdNew(int64(2*n+1), 0), calcPrecision)
			comp1 = comp1.add(comp4)
			if comp4.abs().cmp(epsilon) <= 0 {
				break
			}
		}
	} else {
		// Halley's iteration: a_(n+1) = a_n - 2(exp(a_n)-z)/(exp(a_n)+z)
		prevStep := bdNew(0, 0)
		maxIters := calcPrecision*2 + 10

		for i := int32(0); i < maxIters; i++ {
			comp3, _ = comp1.expTaylor(calcPrecision)
			comp2 = comp3.sub(z)
			comp2 = comp2.add(comp2)
			comp4 = comp3.add(z)
			comp3 = comp2.divRound(comp4, calcPrecision)
			comp1 = comp1.sub(comp3)

			if prevStep.add(comp3).isZero() {
				// oscillating steps: return early to prevent an infinite loop
				break
			}
			if comp3.abs().cmp(epsilon) <= 0 {
				break
			}
			prevStep = comp3
		}
	}

	comp1 = comp1.add(reduceAdjust)
	return comp1.round(precision), nil
}

// powFrac ports shopspring's Pow fractional-exponent path. The caller has
// already handled zero bases/exponents, integer exponents that fit int32, and
// negative bases.
func powFrac(d, d2 Decimal) Decimal {
	base := canonicalBigDec(d)
	exp := canonicalBigDec(d2)

	expIntPart, expFracPart := exp.quoRem(bdNew(1, 0), 0)
	intPartPow := base.powBigInt(expIntPart.value, int32(PowPrecisionNegativeExponent))

	// integer exponent: no fractional factor needed
	if expFracPart.sign() == 0 {
		return decimalFromBigParts(intPartPow.value, int64(intPartPow.exp))
	}

	digitsBase := base.numDigits()
	digitsExponent := exp.numDigits()
	precision := digitsBase
	if digitsExponent > precision {
		precision += digitsExponent
	}
	precision += 6

	// x ** frac(y) = exp(ln(x) * frac(y))
	fracPartPow, err := base.abs().ln(-base.exp + int32(precision))
	if err != nil {
		return Decimal{}
	}
	fracPartPow = fracPartPow.mul(expFracPart)
	fracPartPow, err = fracPartPow.expTaylor(-base.exp + int32(precision))
	if err != nil {
		return Decimal{}
	}

	res := intPartPow.mul(fracPartPow)
	return decimalFromBigParts(res.value, int64(res.exp))
}
