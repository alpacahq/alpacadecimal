package alpacadecimal

// Native ports of shopspring/decimal v1.4.0's PowWithPrecision, PowInt32,
// PowBigInt, ExpHullAbrham, ExpTaylor, Ln and RescalePair, orchestrated on the
// exact bigDec arithmetic from mathops.go so the results are digit-identical
// to shopspring without depending on it. Results requiring more than 19
// fractional digits are truncated (package precision limit); shopspring keeps
// arbitrary precision.

import (
	"errors"
	"math"
	"math/big"
)

// ExpMaxIterations specifies the maximum number of iterations needed to calculate
// precise natural exponent value using ExpHullAbrham method.
var ExpMaxIterations = 1000

var (
	errUndefinedZeroPowZero = errors.New("cannot represent undefined value of 0**0")
	errInfinityZeroPowNeg   = errors.New("cannot represent infinity value of 0 ** y, where y < 0")
	errImaginaryNegPowFrac  = errors.New("cannot represent imaginary value of x ** y, where x < 0 and y is non-integer decimal")
	errExpOverUnderflow     = errors.New("over/underflow threshold, exp(x) cannot be calculated precisely")
	errExpMaxIterations     = errors.New("exact value cannot be calculated in <=ExpMaxIterations iterations")
)

// bdToDecimal converts a bigDec result back to a Decimal at the public
// boundary, truncating beyond 19 fractional digits.
func bdToDecimal(d bigDec) Decimal {
	return decimalFromBigParts(d.value, int64(d.exp))
}

// neg returns -d.

// intPart ports shopspring's IntPart: rescale to exponent 0 (truncating toward
// zero) and take the int64 value. Like shopspring, the result is undefined
// when the integer part does not fit in an int64 (big.Int.Int64 semantics).
func (d bigDec) neg() bigDec {
	return bigDec{value: new(big.Int).Neg(d.value), exp: d.exp}
}

func (d bigDec) intPart() int64 {
	return d.rescale(0).value.Int64()
}

// bdAbs32 ports shopspring's int32 abs helper.
func bdAbs32(n int32) int32 {
	if n < 0 {
		return -n
	}
	return n
}

// fallback:
// PowWithPrecision returns d to the power of d2.
// Precision parameter specifies minimum precision of the result (digits after decimal point).
// Returned decimal is not rounded to 'precision' places after decimal point.
//
// PowWithPrecision returns error when:
//   - 0 is raised to the power of 0
//   - 0 is raised to a negative power
//   - a negative number is raised to a non-integer power
//
// Results requiring more than 19 fractional digits are truncated (package
// precision limit); shopspring keeps arbitrary precision.
func (d Decimal) PowWithPrecision(d2 Decimal, precision int32) (Decimal, error) {
	baseSign := d.Sign()
	expSign := d2.Sign()

	if baseSign == 0 {
		if expSign == 0 {
			return Decimal{}, errUndefinedZeroPowZero
		}
		if expSign == 1 {
			return Zero, nil
		}
		if expSign == -1 {
			return Decimal{}, errInfinityZeroPowNeg
		}
	}

	if expSign == 0 {
		return NewFromInt(1), nil
	}

	base := canonicalBigDec(d)
	exp := canonicalBigDec(d2)

	expIntPart, expFracPart := exp.quoRem(bdNew(1, 0), 0)

	if baseSign == -1 && expFracPart.sign() != 0 {
		return Decimal{}, errImaginaryNegPowFrac
	}

	intPartPow := base.powBigInt(expIntPart.value, precision)

	// if exponent is an integer we don't need to calculate d1**frac(d2)
	if expFracPart.sign() == 0 {
		return bdToDecimal(intPartPow), nil
	}

	digitsBase := base.numDigits()
	digitsExponent := exp.numDigits()

	if int32(digitsBase) > precision {
		precision = int32(digitsBase)
	}
	if int32(digitsExponent) > precision {
		precision += int32(digitsExponent)
	}
	// increase precision by 10 to compensate for errors in further calculations
	precision += 10

	// Calculate x ** frac(y), where
	// x ** frac(y) = exp(ln(x ** frac(y)) = exp(ln(x) * frac(y))
	fracPartPow, err := base.abs().ln(precision)
	if err != nil {
		return Decimal{}, err
	}

	fracPartPow = fracPartPow.mul(expFracPart)

	fracPartPow, err = fracPartPow.expTaylor(precision)
	if err != nil {
		return Decimal{}, err
	}

	// Join integer and fractional part,
	// base ** (expBase + expFrac) = base ** expBase * base ** expFrac
	res := intPartPow.mul(fracPartPow)

	return bdToDecimal(res), nil
}

// fallback:
// PowInt32 returns d to the power of exp, where exp is int32.
// Only returns error when d and exp is 0, thus result is undefined.
//
// When exponent is negative the returned decimal will have maximum precision
// of PowPrecisionNegativeExponent places after decimal point.
func (d Decimal) PowInt32(exp int32) (Decimal, error) {
	if d.IsZero() && exp == 0 {
		return Decimal{}, errUndefinedZeroPowZero
	}
	if exp == math.MinInt32 {
		// shopspring's abs(int32) overflows for MinInt32, leaving the exponent
		// negative: its loop never runs and 1/1 is returned.
		return NewFromInt(1), nil
	}
	if exp == 0 {
		return NewFromInt(1), nil
	}
	if d.IsZero() && exp < 0 {
		// 1/0**|exp|: shopspring's DivRound panics likewise
		panic("decimal division by 0")
	}
	return d.powInt32(exp), nil
}

// fallback:
// PowBigInt returns d to the power of exp, where exp is big.Int.
// Only returns error when d and exp is 0, thus result is undefined.
//
// When exponent is negative the returned decimal will have maximum precision
// of PowPrecisionNegativeExponent places after decimal point.
func (d Decimal) PowBigInt(exp *big.Int) (Decimal, error) {
	if d.IsZero() && exp.Sign() == 0 {
		return Decimal{}, errUndefinedZeroPowZero
	}
	if d.IsZero() && exp.Sign() < 0 {
		// 1/0**|exp|: shopspring's DivRound panics likewise
		panic("decimal division by 0")
	}
	res := canonicalBigDec(d).powBigInt(exp, int32(PowPrecisionNegativeExponent))
	return bdToDecimal(res), nil
}

// fallback:
// ExpHullAbrham calculates the natural exponent of decimal (e to the power of d) using Hull-Abraham algorithm.
// OverallPrecision argument specifies the overall precision of the result (integer part + decimal part).
//
// ExpHullAbrham is faster than ExpTaylor for small precision values, but it is much slower for large precision values.
func (d Decimal) ExpHullAbrham(overallPrecision uint32) (Decimal, error) {
	// Algorithm based on Variable precision exponential function.
	// ACM Transactions on Mathematical Software by T. E. Hull & A. Abrham.
	if d.IsZero() {
		return NewFromInt(1), nil
	}

	dc := canonicalBigDec(d)
	currentPrecision := overallPrecision

	// Algorithm does not work if currentPrecision * 23 < |x|.
	// Precision is automatically increased in such cases, so the value can be calculated precisely.
	// If newly calculated precision is higher than ExpMaxIterations the currentPrecision will not be changed.
	f := dc.abs().inexactFloat64()
	if ncp := f / 23; ncp > float64(currentPrecision) && ncp < float64(ExpMaxIterations) {
		currentPrecision = uint32(math.Ceil(ncp))
	}

	// fail if abs(d) beyond an over/underflow threshold
	overflowThreshold := bdNew(23*int64(currentPrecision), 0)
	if dc.abs().cmp(overflowThreshold) > 0 {
		return Decimal{}, errExpOverUnderflow
	}

	// Return 1 if abs(d) small enough; this also avoids later over/underflow
	overflowThreshold2 := bdNew(9, -int32(currentPrecision)-1)
	if dc.abs().cmp(overflowThreshold2) <= 0 {
		// shopspring quirk ported faithfully: it returns Decimal{1, d.exp},
		// i.e. 10^d.exp rather than 1.
		return bdToDecimal(bdNew(1, dc.exp)), nil
	}

	// t is the smallest integer >= 0 such that the corresponding abs(d/k) < 1
	t := dc.exp + int32(dc.numDigits()) // Add d.NumDigits because the paper assumes that d.value [0.1, 1)

	if t < 0 {
		t = 0
	}

	k := bdNew(1, t)                                                // reduction factor
	r := bigDec{value: new(big.Int).Set(dc.value), exp: dc.exp - t} // reduced argument
	p := int32(currentPrecision) + t + 2                            // precision for calculating the sum

	// Determine n, the number of therms for calculating sum
	// use first Newton step (1.435p - 1.182) / log10(p/abs(r))
	// for solving appropriate equation, along with directed
	// roundings and simple rational bound for log10(p/abs(r))
	rf := r.abs().inexactFloat64()
	pf := float64(p)
	nf := math.Ceil((1.453*pf - 1.182) / math.Log10(pf/rf))
	if nf > float64(ExpMaxIterations) || math.IsNaN(nf) {
		return Decimal{}, errExpMaxIterations
	}
	n := int64(nf)

	tmp := bdNew(0, 0)
	sum := bdNew(1, 0)
	one := bdNew(1, 0)
	for i := n - 1; i > 0; i-- {
		tmp.value.SetInt64(i)
		sum = sum.mul(r.divRound(tmp, p))
		sum = sum.add(one)
	}

	ki := k.intPart()
	res := bdNew(1, 0)
	for i := ki; i > 0; i-- {
		res = res.mul(sum)
	}

	resNumDigits := int32(res.numDigits())

	var roundDigits int32
	if resNumDigits > bdAbs32(res.exp) {
		roundDigits = int32(currentPrecision) - resNumDigits - res.exp
	} else {
		roundDigits = int32(currentPrecision)
	}

	res = res.round(roundDigits)

	return bdToDecimal(res), nil
}

// fallback:
// ExpTaylor calculates the natural exponent of decimal (e to the power of d) using Taylor series expansion.
// Precision argument specifies how precise the result must be (number of digits after decimal point).
// Negative precision is allowed.
//
// ExpTaylor is much faster for large precision values than ExpHullAbrham.
//
// Results requiring more than 19 fractional digits are truncated (package
// precision limit); shopspring keeps arbitrary precision.
func (d Decimal) ExpTaylor(precision int32) (Decimal, error) {
	res, err := canonicalBigDec(d).expTaylor(precision)
	if err != nil {
		return Decimal{}, err
	}
	return bdToDecimal(res), nil
}

// fallback:
// Ln calculates natural logarithm of d.
// Precision argument specifies how precise the result must be (number of digits after decimal point).
// Negative precision is allowed.
//
// Results requiring more than 19 fractional digits are truncated (package
// precision limit); shopspring keeps arbitrary precision.
func (d Decimal) Ln(precision int32) (Decimal, error) {
	res, err := canonicalBigDec(d).ln(precision)
	if err != nil {
		return Decimal{}, err
	}
	return bdToDecimal(res), nil
}

// fallback:
// RescalePair rescales two decimals to common exponential value (minimal exp of both decimals).
// The returned decimals are value-equal to the inputs and report the same Exponent().
//
// Note: a zero rescaled to a lower exponent cannot carry that exponent in this
// package's representation (the coefficient is normalized), so zeros are
// returned unchanged.
func RescalePair(d1 Decimal, d2 Decimal) (Decimal, Decimal) {
	e1, e2 := d1.Exponent(), d2.Exponent()
	switch {
	case e1 < e2:
		return d1, rescaleToExp(d2, e1)
	case e1 > e2:
		return rescaleToExp(d1, e2), d2
	default:
		return d1, d2
	}
}

// rescaleToExp returns a Decimal value-equal to d whose Exponent() is e.
// e is the smaller Exponent() of a Decimal pair, so -19 <= e < d.Exponent()
// and the rescale (adding trailing zeros to the coefficient) is exact.
func rescaleToExp(d Decimal, e int32) Decimal {
	if d.IsZero() {
		// zerodecimal normalizes a zero coefficient to precision 0, so a zero
		// cannot carry an arbitrary exponent; return it unchanged.
		return d
	}
	coef, exp := d.toBigParts()
	if shift := int64(exp) - int64(e); shift > 0 {
		coef = new(big.Int).Mul(coef, bigPow10(shift))
	}
	// zdFromBig keeps the requested precision verbatim whenever the rescaled
	// coefficient fits 128 bits; otherwise it degrades precision (truncating
	// exactly the trailing zeros this rescale added, or out-of-domain digits).
	u := zdFromBig(coef, uint8(-e))
	if e == -precision {
		// the fixed representation also reports Exponent() == -12
		return NewFromDecimal(u)
	}
	return newFromFallback(u)
}
