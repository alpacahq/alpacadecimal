package alpacadecimal

// Native port of shopspring/decimal v1.4.0's trigonometric functions (Atan,
// Sin, Cos, Tan) onto the exact bigDec arithmetic from mathops.go. The
// algorithms — float-derived coefficient tables, exact Mul/Add/Sub and Div at
// DivisionPrecision — are ported verbatim so results are digit-identical to
// shopspring. Results requiring more than 19 fractional digits are truncated
// (package precision limit); shopspring keeps arbitrary precision.

// Float constants are converted with bdFromFloat, which is shopspring's
// NewFromFloat (shortest round-tripping decimal representation), so every
// table entry matches shopspring's package-level tables exactly.
var (
	bdOneConst  = bdFromFloat(1.0)
	bdTwoConst  = bdFromFloat(2.0)
	bdFourConst = bdFromFloat(4.0)
	bdHalfConst = bdFromFloat(0.5)
	bdNegOne    = bdFromFloat(-1.0)

	bdPointSixSix = bdFromFloat(0.66)
	bd1em14       = bdFromFloat(1e-14)

	bdMorebits = bdFromFloat(6.123233995736765886130e-17) // pi/2 = PIO2 + Morebits
	bdTan3pio8 = bdFromFloat(2.41421356237309504880)      // tan(3*pi/8)
	bdPi       = bdFromFloat(3.14159265358979323846264338327950288419716939937510582097494459)

	bdPI4A = bdFromFloat(7.85398125648498535156e-1)                             // 0x3fe921fb40000000, Pi/4 split into three parts
	bdPI4B = bdFromFloat(3.77489470793079817668e-8)                             // 0x3e64442d00000000,
	bdPI4C = bdFromFloat(2.69515142907905952645e-15)                            // 0x3ce8469898cc5170,
	bdM4PI = bdFromFloat(1.273239544735162542821171882678754627704620361328125) // 4/pi

	// xatan rational approximation coefficients
	bdAtanP = [...]bigDec{
		bdFromFloat(-8.750608600031904122785e-01),
		bdFromFloat(-1.615753718733365076637e+01),
		bdFromFloat(-7.500855792314704667340e+01),
		bdFromFloat(-1.228866684490136173410e+02),
		bdFromFloat(-6.485021904942025371773e+01),
	}
	bdAtanQ = [...]bigDec{
		bdFromFloat(2.485846490142306297962e+01),
		bdFromFloat(1.650270098316988542046e+02),
		bdFromFloat(4.328810604912902668951e+02),
		bdFromFloat(4.853903996359136964868e+02),
		bdFromFloat(1.945506571482613964425e+02),
	}

	// sin coefficients
	bdSinCoef = [...]bigDec{
		bdFromFloat(1.58962301576546568060e-10), // 0x3de5d8fd1fd19ccd
		bdFromFloat(-2.50507477628578072866e-8), // 0xbe5ae5e5a9291f5d
		bdFromFloat(2.75573136213857245213e-6),  // 0x3ec71de3567d48a1
		bdFromFloat(-1.98412698295895385996e-4), // 0xbf2a01a019bfdf03
		bdFromFloat(8.33333333332211858878e-3),  // 0x3f8111111110f7d0
		bdFromFloat(-1.66666666666666307295e-1), // 0xbfc5555555555548
	}

	// cos coefficients
	bdCosCoef = [...]bigDec{
		bdFromFloat(-1.13585365213876817300e-11), // 0xbda8fa49a0861a9b
		bdFromFloat(2.08757008419747316778e-9),   // 0x3e21ee9d7b4e3f05
		bdFromFloat(-2.75573141792967388112e-7),  // 0xbe927e4f7eac4bc6
		bdFromFloat(2.48015872888517045348e-5),   // 0x3efa01a019c844f5
		bdFromFloat(-1.38888888888730564116e-3),  // 0xbf56c16c16c14f91
		bdFromFloat(4.16666666666665929218e-2),   // 0x3fa555555555554b
	}

	bdTanP = [...]bigDec{
		bdFromFloat(-1.30936939181383777646e+4), // 0xc0c992d8d24f3f38
		bdFromFloat(1.15351664838587416140e+6),  // 0x413199eca5fc9ddd
		bdFromFloat(-1.79565251976484877988e+7), // 0xc1711fead3299176
	}
	bdTanQ = [...]bigDec{
		bdFromFloat(1.00000000000000000000e+0),
		bdFromFloat(1.36812963470692954678e+4),  // 0x40cab8a5eeb36572
		bdFromFloat(-1.32089234440210967447e+6), // 0xc13427bc582abc96
		bdFromFloat(2.50083801823357915839e+7),  // 0x4177d98fc2ead8ef
		bdFromFloat(-5.38695755929454629881e+7), // 0xc189afe03cbe5a31
	}
)

// bdDiv is shopspring's Div: DivRound at the DivisionPrecision package var
// (read at call time).
func bdDiv(a, b bigDec) bigDec {
	return a.divRound(b, int32(DivisionPrecision))
}

// fallback:
// Atan returns the arctangent, in radians, of x.
//
// Results requiring more than 19 fractional digits are truncated (package
// precision limit); shopspring keeps arbitrary precision.
func (d Decimal) Atan() Decimal {
	sign := d.Sign()
	if sign == 0 {
		return d
	}
	x := canonicalBigDec(d)
	if sign > 0 {
		return bdToDecimal(bdSatan(x))
	}
	return bdToDecimal(bdSatan(x.neg()).neg())
}

func bdXatan(d bigDec) bigDec {
	z := d.mul(d)
	b1 := bdAtanP[0].mul(z).add(bdAtanP[1]).mul(z).add(bdAtanP[2]).mul(z).add(bdAtanP[3]).mul(z).add(bdAtanP[4]).mul(z)
	b2 := z.add(bdAtanQ[0]).mul(z).add(bdAtanQ[1]).mul(z).add(bdAtanQ[2]).mul(z).add(bdAtanQ[3]).mul(z).add(bdAtanQ[4])
	z = bdDiv(b1, b2)
	z = d.mul(z).add(d)
	return z
}

// bdSatan reduces its argument (known to be positive)
// to the range [0, 0.66] and calls bdXatan.
func bdSatan(d bigDec) bigDec {
	if d.cmp(bdPointSixSix) <= 0 {
		return bdXatan(d)
	}
	if d.cmp(bdTan3pio8) > 0 {
		return bdDiv(bdPi, bdTwoConst).sub(bdXatan(bdDiv(bdOneConst, d))).add(bdMorebits)
	}
	return bdDiv(bdPi, bdFourConst).add(bdXatan(bdDiv(d.sub(bdOneConst), d.add(bdOneConst)))).add(bdHalfConst.mul(bdMorebits))
}

// fallback:
// Sin returns the sine of the radian argument x.
//
// Results requiring more than 19 fractional digits are truncated (package
// precision limit); shopspring keeps arbitrary precision.
func (d Decimal) Sin() Decimal {
	if d.Sign() == 0 {
		return d
	}
	x := canonicalBigDec(d)

	// make argument positive but save the sign
	sign := false
	if x.sign() < 0 {
		x = x.neg()
		sign = true
	}

	j := x.mul(bdM4PI).intPart() // integer part of x/(Pi/4), as integer for tests on the phase angle
	y := bdFromFloat(float64(j)) // integer part of x/(Pi/4), as float

	// map zeros to origin
	if j&1 == 1 {
		j++
		y = y.add(bdOneConst)
	}
	j &= 7 // octant modulo 2Pi radians (360 degrees)
	// reflect in x axis
	if j > 3 {
		sign = !sign
		j -= 4
	}
	z := x.sub(y.mul(bdPI4A)).sub(y.mul(bdPI4B)).sub(y.mul(bdPI4C)) // Extended precision modular arithmetic
	zz := z.mul(z)

	if j == 1 || j == 2 {
		w := zz.mul(zz).mul(bdCosCoef[0].mul(zz).add(bdCosCoef[1]).mul(zz).add(bdCosCoef[2]).mul(zz).add(bdCosCoef[3]).mul(zz).add(bdCosCoef[4]).mul(zz).add(bdCosCoef[5]))
		y = bdOneConst.sub(bdHalfConst.mul(zz)).add(w)
	} else {
		y = z.add(z.mul(zz).mul(bdSinCoef[0].mul(zz).add(bdSinCoef[1]).mul(zz).add(bdSinCoef[2]).mul(zz).add(bdSinCoef[3]).mul(zz).add(bdSinCoef[4]).mul(zz).add(bdSinCoef[5])))
	}
	if sign {
		y = y.neg()
	}
	return bdToDecimal(y)
}

// fallback:
// Cos returns the cosine of the radian argument x.
//
// Results requiring more than 19 fractional digits are truncated (package
// precision limit); shopspring keeps arbitrary precision.
func (d Decimal) Cos() Decimal {
	x := canonicalBigDec(d)

	// make argument positive
	sign := false
	if x.sign() < 0 {
		x = x.neg()
	}

	j := x.mul(bdM4PI).intPart() // integer part of x/(Pi/4), as integer for tests on the phase angle
	y := bdFromFloat(float64(j)) // integer part of x/(Pi/4), as float

	// map zeros to origin
	if j&1 == 1 {
		j++
		y = y.add(bdOneConst)
	}
	j &= 7 // octant modulo 2Pi radians (360 degrees)
	// reflect in x axis
	if j > 3 {
		sign = !sign
		j -= 4
	}
	if j > 1 {
		sign = !sign
	}

	z := x.sub(y.mul(bdPI4A)).sub(y.mul(bdPI4B)).sub(y.mul(bdPI4C)) // Extended precision modular arithmetic
	zz := z.mul(z)

	if j == 1 || j == 2 {
		y = z.add(z.mul(zz).mul(bdSinCoef[0].mul(zz).add(bdSinCoef[1]).mul(zz).add(bdSinCoef[2]).mul(zz).add(bdSinCoef[3]).mul(zz).add(bdSinCoef[4]).mul(zz).add(bdSinCoef[5])))
	} else {
		w := zz.mul(zz).mul(bdCosCoef[0].mul(zz).add(bdCosCoef[1]).mul(zz).add(bdCosCoef[2]).mul(zz).add(bdCosCoef[3]).mul(zz).add(bdCosCoef[4]).mul(zz).add(bdCosCoef[5]))
		y = bdOneConst.sub(bdHalfConst.mul(zz)).add(w)
	}
	if sign {
		y = y.neg()
	}
	return bdToDecimal(y)
}

// fallback:
// Tan returns the tangent of the radian argument x.
//
// Results requiring more than 19 fractional digits are truncated (package
// precision limit); shopspring keeps arbitrary precision.
func (d Decimal) Tan() Decimal {
	if d.Sign() == 0 {
		return d
	}
	x := canonicalBigDec(d)

	// make argument positive but save the sign
	sign := false
	if x.sign() < 0 {
		x = x.neg()
		sign = true
	}

	j := x.mul(bdM4PI).intPart() // integer part of x/(Pi/4), as integer for tests on the phase angle
	y := bdFromFloat(float64(j)) // integer part of x/(Pi/4), as float

	// map zeros to origin
	if j&1 == 1 {
		j++
		y = y.add(bdOneConst)
	}

	z := x.sub(y.mul(bdPI4A)).sub(y.mul(bdPI4B)).sub(y.mul(bdPI4C)) // Extended precision modular arithmetic
	zz := z.mul(z)

	if zz.cmp(bd1em14) > 0 {
		w := zz.mul(bdTanP[0].mul(zz).add(bdTanP[1]).mul(zz).add(bdTanP[2]))
		xq := zz.add(bdTanQ[1]).mul(zz).add(bdTanQ[2]).mul(zz).add(bdTanQ[3]).mul(zz).add(bdTanQ[4])
		y = z.add(z.mul(bdDiv(w, xq)))
	} else {
		y = z
	}
	if j&2 == 2 {
		if y.isZero() {
			// shopspring's Div panics likewise
			panic("decimal division by 0")
		}
		y = bdDiv(bdNegOne, y)
	}
	if sign {
		y = y.neg()
	}
	return bdToDecimal(y)
}
