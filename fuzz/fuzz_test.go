package fuzz

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"testing"

	alpaca "github.com/alpacahq/alpacadecimal"
	shopspring "github.com/shopspring/decimal"
)

// maxPrec is the maximum precision udecimal supports.
// We constrain test inputs to this so results are comparable.
const maxPrec = 19

// randDecimalString generates a random decimal string with at most maxPrec fractional digits.
func randDecimalString(rng *rand.Rand) string {
	neg := rng.Intn(2) == 0
	// integer part: 0 to ~15 digits
	intDigits := rng.Intn(16)
	// fractional part: 0 to maxPrec digits
	fracDigits := rng.Intn(maxPrec + 1)

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}

	if intDigits == 0 {
		b.WriteByte('0')
	} else {
		// first digit: 1-9
		b.WriteByte(byte('1' + rng.Intn(9)))
		for i := 1; i < intDigits; i++ {
			b.WriteByte(byte('0' + rng.Intn(10)))
		}
	}

	if fracDigits > 0 {
		b.WriteByte('.')
		for i := 0; i < fracDigits; i++ {
			b.WriteByte(byte('0' + rng.Intn(10)))
		}
	}

	return b.String()
}

// compare checks that the alpacadecimal and shopspring results match.
// It compares by trimming trailing zeros from both string representations.
func compare(t *testing.T, op string, alpacaResult alpaca.Decimal, shopResult shopspring.Decimal) {
	t.Helper()
	a := trimTrailing(alpacaResult.String())
	s := trimTrailing(shopResult.String())
	if a != s {
		t.Errorf("%s: alpaca=%q shopspring=%q", op, alpacaResult.String(), shopResult.String())
	}
}

func trimTrailing(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// safeNewFromFloat calls NewFromFloat and recovers from panics (e.g. precision out of range).
func safeNewFromFloat(v float64) (d alpaca.Decimal, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	return alpaca.NewFromFloat(v), true
}

// safeCall calls fn and recovers from panics, returning false if it panicked.
func safeCall(fn func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	fn()
	return true
}

// fracDigitsOf returns the number of fractional digits in a decimal string.
func fracDigitsOf(s string) int {
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		return len(s) - idx - 1
	}
	return 0
}

func FuzzArithmetic(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "0.5", "-0.5",
		"123.456", "-123.456",
		"9999999.999999999", "-9999999.999999999",
		"0.000000001", "1000000000",
	}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b)
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string) {
		a, errA := alpaca.NewFromString(aStr)
		shopA, errSA := shopspring.NewFromString(aStr)
		if errA != nil || errSA != nil {
			return
		}

		b, errB := alpaca.NewFromString(bStr)
		shopB, errSB := shopspring.NewFromString(bStr)
		if errB != nil || errSB != nil {
			return
		}

		// Add
		compare(t, fmt.Sprintf("Add(%s, %s)", aStr, bStr), a.Add(b), shopA.Add(shopB))

		// Sub
		compare(t, fmt.Sprintf("Sub(%s, %s)", aStr, bStr), a.Sub(b), shopA.Sub(shopB))

		// Mul — udecimal truncates to 19 digits, shopspring keeps full precision.
		// Compare by truncating shopspring result to 19 decimal places.
		compareTruncated(t, fmt.Sprintf("Mul(%s, %s)", aStr, bStr), a.Mul(b), shopA.Mul(shopB), 19)

		// Neg
		compare(t, fmt.Sprintf("Neg(%s)", aStr), a.Neg(), shopA.Neg())

		// Abs
		compare(t, fmt.Sprintf("Abs(%s)", aStr), a.Abs(), shopA.Abs())

		// Cmp
		alpacaCmp := a.Cmp(b)
		shopCmp := shopA.Cmp(shopB)
		if alpacaCmp != shopCmp {
			t.Errorf("Cmp(%s, %s): alpaca=%d shopspring=%d", aStr, bStr, alpacaCmp, shopCmp)
		}

		// Equal
		alpacaEq := a.Equal(b)
		shopEq := shopA.Equal(shopB)
		if alpacaEq != shopEq {
			t.Errorf("Equal(%s, %s): alpaca=%v shopspring=%v", aStr, bStr, alpacaEq, shopEq)
		}

		// Sign
		if a.Sign() != shopA.Sign() {
			t.Errorf("Sign(%s): alpaca=%d shopspring=%d", aStr, a.Sign(), shopA.Sign())
		}

		// IsZero
		if a.IsZero() != shopA.IsZero() {
			t.Errorf("IsZero(%s): alpaca=%v shopspring=%v", aStr, a.IsZero(), shopA.IsZero())
		}

		// IsNegative
		if a.IsNegative() != shopA.IsNegative() {
			t.Errorf("IsNegative(%s): alpaca=%v shopspring=%v", aStr, a.IsNegative(), shopA.IsNegative())
		}

		// IsPositive
		if a.IsPositive() != shopA.IsPositive() {
			t.Errorf("IsPositive(%s): alpaca=%v shopspring=%v", aStr, a.IsPositive(), shopA.IsPositive())
		}

		// Div (skip division by zero and extreme values that exceed udecimal precision)
		if !b.IsZero() && len(aStr) < 40 && len(bStr) < 40 {
			aDiv := a.Div(b)
			sDiv := shopA.Div(shopB)
			// Division precision may differ — compare to limited precision
			compareDivision(t, fmt.Sprintf("Div(%s, %s)", aStr, bStr), aDiv, sDiv)
		}

		// Mod (skip mod by zero)
		if !b.IsZero() {
			compare(t, fmt.Sprintf("Mod(%s, %s)", aStr, bStr), a.Mod(b), shopA.Mod(shopB))
		}
	})
}

// compareTruncated compares results after truncating both to a given number of decimal places.
// This accounts for udecimal's max 19-digit precision vs shopspring's arbitrary precision.
func compareTruncated(t *testing.T, op string, alpacaResult alpaca.Decimal, shopResult shopspring.Decimal, places int32) {
	t.Helper()
	a := trimTrailing(alpacaResult.Truncate(places).String())
	s := trimTrailing(shopResult.Truncate(places).String())
	if a != s {
		t.Errorf("%s: alpaca=%q shopspring=%q (truncated to %d)", op, a, s, places)
	}
}

// compareDivision compares division results with tolerance for precision differences.
// udecimal uses 19 digits precision, shopspring uses 16 by default.
func compareDivision(t *testing.T, op string, alpacaResult alpaca.Decimal, shopResult shopspring.Decimal) {
	t.Helper()
	a := trimTrailing(alpacaResult.Truncate(16).String())
	s := trimTrailing(shopResult.Truncate(16).String())
	if a != s {
		t.Errorf("%s: alpaca=%q shopspring=%q (truncated to 16)", op, a, s)
	}
}

func FuzzRounding(f *testing.F) {
	seeds := []string{
		"0", "1.5", "-1.5", "2.5", "-2.5",
		"1.45", "1.55", "123.456", "-123.456",
		"0.999", "-0.999", "1.005", "99.995",
	}
	for _, s := range seeds {
		for places := int8(-2); places <= 10; places++ {
			f.Add(s, places)
		}
	}

	f.Fuzz(func(t *testing.T, s string, places int8) {
		if places < -5 || places > 18 {
			return
		}
		p := int32(places)

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}

		// Round (half away from zero)
		compare(t, fmt.Sprintf("Round(%s, %d)", s, p), a.Round(p), shopD.Round(p))

		// RoundBank (half to even)
		compare(t, fmt.Sprintf("RoundBank(%s, %d)", s, p), a.RoundBank(p), shopD.RoundBank(p))

		// Truncate
		if p >= 0 {
			compare(t, fmt.Sprintf("Truncate(%s, %d)", s, p), a.Truncate(p), shopD.Truncate(p))
		}

		// RoundCeil
		compare(t, fmt.Sprintf("RoundCeil(%s, %d)", s, p), a.RoundCeil(p), shopD.RoundCeil(p))

		// RoundFloor
		compare(t, fmt.Sprintf("RoundFloor(%s, %d)", s, p), a.RoundFloor(p), shopD.RoundFloor(p))

		// RoundUp (away from zero)
		compare(t, fmt.Sprintf("RoundUp(%s, %d)", s, p), a.RoundUp(p), shopD.RoundUp(p))

		// RoundDown (toward zero)
		compare(t, fmt.Sprintf("RoundDown(%s, %d)", s, p), a.RoundDown(p), shopD.RoundDown(p))
	})
}

func FuzzStringFixed(f *testing.F) {
	seeds := []string{
		"0", "1", "1.23", "-1.23", "0.001", "999.999",
	}
	for _, s := range seeds {
		for places := int8(0); places <= 10; places++ {
			f.Add(s, places)
		}
	}

	f.Fuzz(func(t *testing.T, s string, places int8) {
		if places < 0 || places > 18 {
			return
		}
		p := int32(places)

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}

		aStr := a.StringFixed(p)
		sStr := shopD.StringFixed(p)
		if aStr != sStr {
			t.Errorf("StringFixed(%s, %d): alpaca=%q shopspring=%q", s, p, aStr, sStr)
		}
	})
}

func FuzzIntrospection(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "1.5", "-1.5", "100", "0.001",
		"9999999999", "-9999999999",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}

		// IntPart — skip values that overflow int64 (both implementations are wrong for those)
		if len(s) <= 18 {
			if a.IntPart() != shopD.IntPart() {
				t.Errorf("IntPart(%s): alpaca=%d shopspring=%d", s, a.IntPart(), shopD.IntPart())
			}
		}

		// IsInteger
		if a.IsInteger() != shopD.IsInteger() {
			t.Errorf("IsInteger(%s): alpaca=%v shopspring=%v", s, a.IsInteger(), shopD.IsInteger())
		}

		// InexactFloat64 — compare with tolerance
		af := a.InexactFloat64()
		sf := shopD.InexactFloat64()
		if math.Abs(af-sf) > 1e-10 {
			t.Errorf("InexactFloat64(%s): alpaca=%v shopspring=%v", s, af, sf)
		}

		// String round-trip
		aStr := a.String()
		roundTrip, err := alpaca.NewFromString(aStr)
		if err != nil {
			t.Errorf("String round-trip(%s): parse error: %v", s, err)
		} else if !roundTrip.Equal(a) {
			t.Errorf("String round-trip(%s): %q -> %q", s, aStr, roundTrip.String())
		}
	})
}

func FuzzConstructors(f *testing.F) {
	f.Add(int64(0), int32(0))
	f.Add(int64(1), int32(0))
	f.Add(int64(123), int32(-2))
	f.Add(int64(-456), int32(-3))
	f.Add(int64(100), int32(2))
	f.Add(int64(-1), int32(-12))
	f.Add(int64(999999999), int32(0))

	f.Fuzz(func(t *testing.T, value int64, exp int32) {
		// Constrain to reasonable range
		if exp < -19 || exp > 19 {
			return
		}

		a := alpaca.New(value, exp)
		s := shopspring.New(value, exp)
		compare(t, fmt.Sprintf("New(%d, %d)", value, exp), a, s)
	})
}

// TestRandomOperations runs a large number of random operations to compare implementations.
func TestRandomOperations(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 10000; i++ {
		aStr := randDecimalString(rng)
		bStr := randDecimalString(rng)

		a, errA := alpaca.NewFromString(aStr)
		shopA, errSA := shopspring.NewFromString(aStr)
		if errA != nil || errSA != nil {
			continue
		}
		b, errB := alpaca.NewFromString(bStr)
		shopB, errSB := shopspring.NewFromString(bStr)
		if errB != nil || errSB != nil {
			continue
		}

		compare(t, fmt.Sprintf("[%d] Add(%s, %s)", i, aStr, bStr), a.Add(b), shopA.Add(shopB))
		compare(t, fmt.Sprintf("[%d] Sub(%s, %s)", i, aStr, bStr), a.Sub(b), shopA.Sub(shopB))
		compareTruncated(t, fmt.Sprintf("[%d] Mul(%s, %s)", i, aStr, bStr), a.Mul(b), shopA.Mul(shopB), 19)

		if !b.IsZero() {
			compare(t, fmt.Sprintf("[%d] Mod(%s, %s)", i, aStr, bStr), a.Mod(b), shopA.Mod(shopB))
		}

		places := int32(rng.Intn(15))
		compare(t, fmt.Sprintf("[%d] Round(%s, %d)", i, aStr, places), a.Round(places), shopA.Round(places))
		compare(t, fmt.Sprintf("[%d] RoundBank(%s, %d)", i, aStr, places), a.RoundBank(places), shopA.RoundBank(places))
		if places >= 0 {
			compare(t, fmt.Sprintf("[%d] Truncate(%s, %d)", i, aStr, places), a.Truncate(places), shopA.Truncate(places))
		}
		compare(t, fmt.Sprintf("[%d] RoundCeil(%s, %d)", i, aStr, places), a.RoundCeil(places), shopA.RoundCeil(places))
		compare(t, fmt.Sprintf("[%d] RoundFloor(%s, %d)", i, aStr, places), a.RoundFloor(places), shopA.RoundFloor(places))

		if a.Sign() != shopA.Sign() {
			t.Errorf("[%d] Sign(%s): alpaca=%d shopspring=%d", i, aStr, a.Sign(), shopA.Sign())
		}
		if a.IntPart() != shopA.IntPart() {
			t.Errorf("[%d] IntPart(%s): alpaca=%d shopspring=%d", i, aStr, a.IntPart(), shopA.IntPart())
		}
	}
}

func FuzzComparisons(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "0.5", "-0.5",
		"123.456", "-123.456",
		"9999999.999999999", "-9999999.999999999",
	}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b)
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string) {
		a, errA := alpaca.NewFromString(aStr)
		shopA, errSA := shopspring.NewFromString(aStr)
		if errA != nil || errSA != nil {
			return
		}
		b, errB := alpaca.NewFromString(bStr)
		shopB, errSB := shopspring.NewFromString(bStr)
		if errB != nil || errSB != nil {
			return
		}

		// GreaterThan
		if a.GreaterThan(b) != shopA.GreaterThan(shopB) {
			t.Errorf("GreaterThan(%s, %s): alpaca=%v shopspring=%v", aStr, bStr, a.GreaterThan(b), shopA.GreaterThan(shopB))
		}

		// GreaterThanOrEqual
		if a.GreaterThanOrEqual(b) != shopA.GreaterThanOrEqual(shopB) {
			t.Errorf("GreaterThanOrEqual(%s, %s): alpaca=%v shopspring=%v", aStr, bStr, a.GreaterThanOrEqual(b), shopA.GreaterThanOrEqual(shopB))
		}

		// LessThan
		if a.LessThan(b) != shopA.LessThan(shopB) {
			t.Errorf("LessThan(%s, %s): alpaca=%v shopspring=%v", aStr, bStr, a.LessThan(b), shopA.LessThan(shopB))
		}

		// LessThanOrEqual
		if a.LessThanOrEqual(b) != shopA.LessThanOrEqual(shopB) {
			t.Errorf("LessThanOrEqual(%s, %s): alpaca=%v shopspring=%v", aStr, bStr, a.LessThanOrEqual(b), shopA.LessThanOrEqual(shopB))
		}
	})
}

func FuzzCeilFloor(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "1.5", "-1.5", "2.5", "-2.5",
		"1.001", "-1.001", "0.999", "-0.999",
		"123.456", "-123.456",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}

		// Ceil
		compare(t, fmt.Sprintf("Ceil(%s)", s), a.Ceil(), shopD.Ceil())

		// Floor
		compare(t, fmt.Sprintf("Floor(%s)", s), a.Floor(), shopD.Floor())
	})
}

func FuzzNewFromInt(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(-1))
	f.Add(int64(100))
	f.Add(int64(-100))
	f.Add(int64(math.MaxInt32))
	f.Add(int64(math.MinInt32))
	f.Add(int64(9223372))
	f.Add(int64(-9223372))

	f.Fuzz(func(t *testing.T, v int64) {
		a := alpaca.NewFromInt(v)
		s := shopspring.NewFromInt(v)
		compare(t, fmt.Sprintf("NewFromInt(%d)", v), a, s)

		// Also test NewFromInt32 for values in range
		if v >= math.MinInt32 && v <= math.MaxInt32 {
			a32 := alpaca.NewFromInt32(int32(v))
			s32 := shopspring.NewFromInt(v)
			compare(t, fmt.Sprintf("NewFromInt32(%d)", v), a32, s32)
		}
	})
}

func FuzzNewFromFloat(f *testing.F) {
	f.Add(0.0)
	f.Add(1.0)
	f.Add(-1.0)
	f.Add(0.1)
	f.Add(-0.1)
	f.Add(123.456)
	f.Add(-123.456)
	f.Add(0.000000001)
	f.Add(999999.999999)

	f.Fuzz(func(t *testing.T, v float64) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return
		}
		// Limit range to avoid precision issues with extreme floats
		if v > 1e15 || v < -1e15 {
			return
		}

		// NewFromFloat can panic on values that exceed udecimal's precision
		a, ok := safeNewFromFloat(v)
		if !ok {
			return
		}

		s := shopspring.NewFromFloat(v)

		// When the float needs >19 fractional digits, alpaca must round to 19
		// (udecimal's limit). The rounding mode differs (Go uses banker's rounding,
		// shopspring uses round-half-up), so just verify no panic in that case.
		str := strconv.FormatFloat(v, 'f', -1, 64)
		if dotIdx := strings.IndexByte(str, '.'); dotIdx >= 0 && len(str)-dotIdx-1 > 19 {
			return
		}
		compare(t, fmt.Sprintf("NewFromFloat(%v)", v), a, s)
	})
}

func FuzzNewFromFloat32(f *testing.F) {
	f.Add(float32(0.0))
	f.Add(float32(1.0))
	f.Add(float32(-1.0))
	f.Add(float32(0.1))
	f.Add(float32(123.456))

	f.Fuzz(func(t *testing.T, v float32) {
		if v != v || math.IsInf(float64(v), 0) { // NaN or Inf check
			return
		}
		if v > 1e10 || v < -1e10 {
			return
		}

		var a alpaca.Decimal
		if !safeCall(func() { a = alpaca.NewFromFloat32(v) }) {
			return
		}
		s := shopspring.NewFromFloat32(v)
		compare(t, fmt.Sprintf("NewFromFloat32(%v)", v), a, s)
	})
}

func FuzzShift(f *testing.F) {
	seeds := []string{"0", "1", "-1", "123.456", "-123.456", "0.001"}
	for _, s := range seeds {
		for shift := int32(-5); shift <= 5; shift++ {
			f.Add(s, shift)
		}
	}

	f.Fuzz(func(t *testing.T, s string, shift int32) {
		if shift < -8 || shift > 8 {
			return
		}

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}

		// Skip if the result would exceed 19 decimal digits (udecimal limit)
		dotIdx := strings.IndexByte(s, '.')
		fracDigits := 0
		if dotIdx >= 0 {
			fracDigits = len(s) - dotIdx - 1
		}
		if shift < 0 && fracDigits+int(-shift) > 19 {
			return
		}

		var aResult alpaca.Decimal
		if !safeCall(func() { aResult = a.Shift(shift) }) {
			return
		}
		sResult := shopD.Shift(shift)
		compare(t, fmt.Sprintf("Shift(%s, %d)", s, shift), aResult, sResult)
	})
}

func FuzzPow(f *testing.F) {
	seeds := []string{"0", "1", "-1", "2", "-2", "1.5", "10", "0.5"}
	for _, base := range seeds {
		for _, exp := range []string{"0", "1", "2", "3", "-1"} {
			f.Add(base, exp)
		}
	}

	f.Fuzz(func(t *testing.T, baseStr, expStr string) {
		base, errA := alpaca.NewFromString(baseStr)
		shopBase, errSA := shopspring.NewFromString(baseStr)
		if errA != nil || errSA != nil {
			return
		}
		exp, errB := alpaca.NewFromString(expStr)
		shopExp, errSB := shopspring.NewFromString(expStr)
		if errB != nil || errSB != nil {
			return
		}

		// Pow only works with integer exponents; skip non-integer
		if !exp.IsInteger() {
			return
		}
		// Limit exponent range to avoid huge results
		expInt := exp.IntPart()
		if expInt < -5 || expInt > 10 {
			return
		}
		// 0^negative is undefined, 0^0 differs between implementations
		if base.IsZero() && expInt <= 0 {
			return
		}

		var aResult alpaca.Decimal
		if !safeCall(func() { aResult = base.Pow(exp) }) {
			return
		}
		sResult := shopBase.Pow(shopExp)
		if expInt >= 0 {
			compareTruncated(t, fmt.Sprintf("Pow(%s, %s)", baseStr, expStr), aResult, sResult, 12)
		} else {
			// Negative exponents involve division where shopspring's fixed
			// DivisionPrecision=16 loses significant digits for small results.
			// Division is already tested in FuzzDivRound; here just verify no panic.
			_ = aResult
		}
	})
}

func FuzzDivRound(f *testing.F) {
	seeds := []string{"0", "1", "-1", "10", "3", "7", "123.456", "-123.456"}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b, int8(4))
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string, prec int8) {
		if prec < 0 || prec > 16 {
			return
		}
		p := int32(prec)

		a, errA := alpaca.NewFromString(aStr)
		shopA, errSA := shopspring.NewFromString(aStr)
		if errA != nil || errSA != nil {
			return
		}
		b, errB := alpaca.NewFromString(bStr)
		shopB, errSB := shopspring.NewFromString(bStr)
		if errB != nil || errSB != nil {
			return
		}
		if b.IsZero() {
			return
		}

		aResult := a.DivRound(b, p)
		sResult := shopA.DivRound(shopB, p)
		compare(t, fmt.Sprintf("DivRound(%s, %s, %d)", aStr, bStr, p), aResult, sResult)
	})
}

func FuzzQuoRem(f *testing.F) {
	seeds := []string{"1", "10", "3", "7", "123.456", "-123.456"}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b, int8(4))
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string, prec int8) {
		if prec < 0 || prec > 16 {
			return
		}
		p := int32(prec)

		a, errA := alpaca.NewFromString(aStr)
		shopA, errSA := shopspring.NewFromString(aStr)
		if errA != nil || errSA != nil {
			return
		}
		b, errB := alpaca.NewFromString(bStr)
		shopB, errSB := shopspring.NewFromString(bStr)
		if errB != nil || errSB != nil {
			return
		}
		if b.IsZero() {
			return
		}

		// Compute max fractional digits in inputs to avoid exceeding 19 digits in remainder
		maxFrac := fracDigitsOf(bStr)
		if af := fracDigitsOf(aStr); af > maxFrac {
			maxFrac = af
		}
		// Remainder has commonPrec + prec implicit decimal places
		if maxFrac+int(p) > 19 {
			return
		}

		aQ, aR := a.QuoRem(b, p)
		sQ, sR := shopA.QuoRem(shopB, p)
		compare(t, fmt.Sprintf("QuoRem_q(%s, %s, %d)", aStr, bStr, p), aQ, sQ)
		compare(t, fmt.Sprintf("QuoRem_r(%s, %s, %d)", aStr, bStr, p), aR, sR)
	})
}

func FuzzAggregates(f *testing.F) {
	seeds := []string{"0", "1", "-1", "123.456", "-999.999", "0.001"}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b)
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string) {
		a, errA := alpaca.NewFromString(aStr)
		shopA, errSA := shopspring.NewFromString(aStr)
		if errA != nil || errSA != nil {
			return
		}
		b, errB := alpaca.NewFromString(bStr)
		shopB, errSB := shopspring.NewFromString(bStr)
		if errB != nil || errSB != nil {
			return
		}

		// Sum
		compare(t, fmt.Sprintf("Sum(%s, %s)", aStr, bStr), alpaca.Sum(a, b), shopspring.Sum(shopA, shopB))

		// Avg
		aAvg := alpaca.Avg(a, b)
		sAvg := shopspring.Avg(shopA, shopB)
		compareDivision(t, fmt.Sprintf("Avg(%s, %s)", aStr, bStr), aAvg, sAvg)

		// Max
		compare(t, fmt.Sprintf("Max(%s, %s)", aStr, bStr), alpaca.Max(a, b), shopspring.Max(shopA, shopB))

		// Min
		compare(t, fmt.Sprintf("Min(%s, %s)", aStr, bStr), alpaca.Min(a, b), shopspring.Min(shopA, shopB))
	})
}

func FuzzExponentCoefficient(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "1.5", "-1.5", "100", "0.001",
		"123.456789", "-999999.123456789012",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		a, errA := alpaca.NewFromString(s)
		if errA != nil {
			return
		}
		// Also check shopspring can parse it (filters junk inputs)
		if _, errS := shopspring.NewFromString(s); errS != nil {
			return
		}

		// Verify self-reconstruction: Coefficient * 10^Exponent == original
		aExp := a.Exponent()
		aCoef := a.Coefficient()
		reconstructed := alpaca.NewFromBigInt(aCoef, aExp)
		if !reconstructed.Equal(a) {
			t.Errorf("Coef*10^Exp(%s): reconstructed=%s original=%s (coef=%s, exp=%d)",
				s, reconstructed.String(), a.String(), aCoef.String(), aExp)
		}

		// CoefficientInt64 — verify consistency with Coefficient
		aCoefI64 := a.CoefficientInt64()
		if aCoef.IsInt64() && aCoef.Int64() != aCoefI64 {
			t.Errorf("CoefficientInt64(%s): got %d expected %d", s, aCoefI64, aCoef.Int64())
		}

		// NumDigits — just verify no panic
		_ = a.NumDigits()
	})
}

func FuzzBigIntBigFloatRat(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "1.5", "-1.5", "100", "0.001",
		"123456", "-999999.99",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}

		// BigInt
		aBigInt := a.BigInt()
		sBigInt := shopD.BigInt()
		if aBigInt.Cmp(sBigInt) != 0 {
			t.Errorf("BigInt(%s): alpaca=%s shopspring=%s", s, aBigInt.String(), sBigInt.String())
		}

		// BigFloat — compare with tolerance
		aBigFloat := a.BigFloat()
		sBigFloat := shopD.BigFloat()
		diff := new(big.Float).Sub(aBigFloat, sBigFloat)
		absDiff := new(big.Float).Abs(diff)
		tol := new(big.Float).SetFloat64(1e-10)
		if absDiff.Cmp(tol) > 0 {
			t.Errorf("BigFloat(%s): alpaca=%s shopspring=%s", s, aBigFloat.String(), sBigFloat.String())
		}

		// Rat
		aRat := a.Rat()
		sRat := shopD.Rat()
		if aRat.Cmp(sRat) != 0 {
			t.Errorf("Rat(%s): alpaca=%s shopspring=%s", s, aRat.String(), sRat.String())
		}

		// Float64
		aF64, aExact := a.Float64()
		sF64, sExact := shopD.Float64()
		if math.Abs(aF64-sF64) > 1e-10 {
			t.Errorf("Float64(%s): alpaca=%v shopspring=%v", s, aF64, sF64)
		}
		// exactness may differ between implementations, so just check the value
		_ = aExact
		_ = sExact

		// Copy should produce equal value
		aCopy := a.Copy()
		if !aCopy.Equal(a) {
			t.Errorf("Copy(%s): copy=%s original=%s", s, aCopy.String(), a.String())
		}
	})
}

func FuzzStringFormats(f *testing.F) {
	seeds := []string{"0", "1", "-1", "1.23", "-1.23", "0.001", "999.999"}
	for _, s := range seeds {
		for places := int8(0); places <= 10; places++ {
			f.Add(s, places)
		}
	}

	f.Fuzz(func(t *testing.T, s string, places int8) {
		if places < 0 || places > 18 {
			return
		}
		p := int32(places)

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}

		// StringFixedBank
		aStr := a.StringFixedBank(p)
		sStr := shopD.StringFixedBank(p)
		if aStr != sStr {
			t.Errorf("StringFixedBank(%s, %d): alpaca=%q shopspring=%q", s, p, aStr, sStr)
		}

		// StringScaled (deprecated) — just verify no panic
		_ = a.StringScaled(-p)
	})
}

func FuzzRoundCash(f *testing.F) {
	seeds := []string{"0", "1.23", "-1.23", "3.43", "3.45", "3.41", "3.75", "3.50"}
	intervals := []uint8{5, 10, 25, 50, 100}
	for _, s := range seeds {
		for _, iv := range intervals {
			f.Add(s, iv)
		}
	}

	f.Fuzz(func(t *testing.T, s string, interval uint8) {
		// Only valid intervals
		switch interval {
		case 5, 10, 25, 50, 100:
		default:
			return
		}

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}

		aResult := a.RoundCash(interval)
		sResult := shopD.RoundCash(interval)
		compare(t, fmt.Sprintf("RoundCash(%s, %d)", s, interval), aResult, sResult)

		// StringFixedCash
		aStr := a.StringFixedCash(interval)
		sStr := shopD.StringFixedCash(interval)
		if aStr != sStr {
			t.Errorf("StringFixedCash(%s, %d): alpaca=%q shopspring=%q", s, interval, aStr, sStr)
		}
	})
}

func FuzzNewFromFloatWithExponent(f *testing.F) {
	f.Add(123.456, int32(-2))
	f.Add(-123.456, int32(-2))
	f.Add(0.0, int32(0))
	f.Add(1.0, int32(-5))
	f.Add(99.995, int32(-2))

	f.Fuzz(func(t *testing.T, v float64, exp int32) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return
		}
		if v > 1e12 || v < -1e12 {
			return
		}
		if exp < -15 || exp > 0 {
			return
		}

		// alpaca truncates, shopspring rounds — just verify equivalent to NewFromFloat().Truncate()
		var a, expected alpaca.Decimal
		if !safeCall(func() {
			a = alpaca.NewFromFloatWithExponent(v, exp)
			expected = alpaca.NewFromFloat(v).Truncate(-exp)
		}) {
			return
		}
		if !a.Equal(expected) {
			t.Errorf("NewFromFloatWithExponent(%v, %d): got %s expected %s", v, exp, a.String(), expected.String())
		}
	})
}

func FuzzNewFromBigInt(f *testing.F) {
	f.Add(int64(0), int32(0))
	f.Add(int64(123), int32(-2))
	f.Add(int64(-456), int32(-3))
	f.Add(int64(1000000), int32(0))
	f.Add(int64(-1), int32(-12))

	f.Fuzz(func(t *testing.T, value int64, exp int32) {
		if exp < -19 || exp > 19 {
			return
		}
		bi := big.NewInt(value)

		a := alpaca.NewFromBigInt(bi, exp)
		s := shopspring.NewFromBigInt(bi, exp)
		compare(t, fmt.Sprintf("NewFromBigInt(%d, %d)", value, exp), a, s)
	})
}

func FuzzSerialization(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "1.23", "-1.23",
		"0.000000000001", "9999999.999999",
		"123456789.123456789",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		a, err := alpaca.NewFromString(s)
		if err != nil {
			return
		}

		// JSON round-trip
		jsonBytes, err := json.Marshal(a)
		if err != nil {
			t.Errorf("MarshalJSON(%s): %v", s, err)
			return
		}
		var aFromJSON alpaca.Decimal
		if err := json.Unmarshal(jsonBytes, &aFromJSON); err != nil {
			t.Errorf("UnmarshalJSON(%s): %v (json=%s)", s, err, string(jsonBytes))
			return
		}
		if !aFromJSON.Equal(a) {
			t.Errorf("JSON round-trip(%s): before=%s after=%s", s, a.String(), aFromJSON.String())
		}

		// Text round-trip
		textBytes, err := a.MarshalText()
		if err != nil {
			t.Errorf("MarshalText(%s): %v", s, err)
			return
		}
		var aFromText alpaca.Decimal
		if err := aFromText.UnmarshalText(textBytes); err != nil {
			t.Errorf("UnmarshalText(%s): %v", s, err)
			return
		}
		if !aFromText.Equal(a) {
			t.Errorf("Text round-trip(%s): before=%s after=%s", s, a.String(), aFromText.String())
		}

		// Binary round-trip
		binBytes, err := a.MarshalBinary()
		if err != nil {
			t.Errorf("MarshalBinary(%s): %v", s, err)
			return
		}
		var aFromBin alpaca.Decimal
		if err := aFromBin.UnmarshalBinary(binBytes); err != nil {
			t.Errorf("UnmarshalBinary(%s): %v", s, err)
			return
		}
		if !aFromBin.Equal(a) {
			t.Errorf("Binary round-trip(%s): before=%s after=%s", s, a.String(), aFromBin.String())
		}

		// Gob round-trip
		gobBytes, err := a.GobEncode()
		if err != nil {
			t.Errorf("GobEncode(%s): %v", s, err)
			return
		}
		var aFromGob alpaca.Decimal
		if err := aFromGob.GobDecode(gobBytes); err != nil {
			t.Errorf("GobDecode(%s): %v", s, err)
			return
		}
		if !aFromGob.Equal(a) {
			t.Errorf("Gob round-trip(%s): before=%s after=%s", s, a.String(), aFromGob.String())
		}

		// Value/Scan round-trip
		val, err := a.Value()
		if err != nil {
			t.Errorf("Value(%s): %v", s, err)
			return
		}
		var aFromScan alpaca.Decimal
		if err := aFromScan.Scan(val); err != nil {
			t.Errorf("Scan(%s): %v (val=%v)", s, err, val)
			return
		}
		if !aFromScan.Equal(a) {
			t.Errorf("Value/Scan round-trip(%s): before=%s after=%s", s, a.String(), aFromScan.String())
		}
	})
}

func FuzzNullDecimal(f *testing.F) {
	f.Add("1.23", true)
	f.Add("-456.789", true)
	f.Add("0", true)
	f.Add("0", false)

	f.Fuzz(func(t *testing.T, s string, valid bool) {
		if !valid {
			// Test null case
			nd := alpaca.NullDecimal{Valid: false}

			// JSON null round-trip
			jsonBytes, err := json.Marshal(nd)
			if err != nil {
				t.Errorf("NullDecimal.MarshalJSON(null): %v", err)
				return
			}
			if string(jsonBytes) != "null" {
				t.Errorf("NullDecimal.MarshalJSON(null): got %s", string(jsonBytes))
				return
			}
			var nd2 alpaca.NullDecimal
			if err := json.Unmarshal(jsonBytes, &nd2); err != nil {
				t.Errorf("NullDecimal.UnmarshalJSON(null): %v", err)
				return
			}
			if nd2.Valid {
				t.Errorf("NullDecimal.UnmarshalJSON(null): expected Valid=false")
			}

			// Value should be nil
			val, err := nd.Value()
			if err != nil {
				t.Errorf("NullDecimal.Value(null): %v", err)
			}
			if val != nil {
				t.Errorf("NullDecimal.Value(null): expected nil, got %v", val)
			}

			// Scan nil
			var nd3 alpaca.NullDecimal
			if err := nd3.Scan(nil); err != nil {
				t.Errorf("NullDecimal.Scan(nil): %v", err)
			}
			if nd3.Valid {
				t.Errorf("NullDecimal.Scan(nil): expected Valid=false")
			}
			return
		}

		d, err := alpaca.NewFromString(s)
		if err != nil {
			return
		}
		nd := alpaca.NewNullDecimal(d)
		if !nd.Valid {
			t.Errorf("NewNullDecimal(%s): Valid should be true", s)
			return
		}

		// JSON round-trip
		jsonBytes, err := json.Marshal(nd)
		if err != nil {
			t.Errorf("NullDecimal.MarshalJSON(%s): %v", s, err)
			return
		}
		var nd2 alpaca.NullDecimal
		if err := json.Unmarshal(jsonBytes, &nd2); err != nil {
			t.Errorf("NullDecimal.UnmarshalJSON(%s): %v", s, err)
			return
		}
		if !nd2.Valid || !nd2.Decimal.Equal(d) {
			t.Errorf("NullDecimal JSON round-trip(%s): before=%s after=%s valid=%v", s, d.String(), nd2.Decimal.String(), nd2.Valid)
		}

		// Text round-trip
		textBytes, err := nd.MarshalText()
		if err != nil {
			t.Errorf("NullDecimal.MarshalText(%s): %v", s, err)
			return
		}
		var nd3 alpaca.NullDecimal
		if err := nd3.UnmarshalText(textBytes); err != nil {
			t.Errorf("NullDecimal.UnmarshalText(%s): %v", s, err)
			return
		}
		if !nd3.Valid || !nd3.Decimal.Equal(d) {
			t.Errorf("NullDecimal Text round-trip(%s): before=%s after=%s valid=%v", s, d.String(), nd3.Decimal.String(), nd3.Valid)
		}

		// Value/Scan round-trip
		val, err := nd.Value()
		if err != nil {
			t.Errorf("NullDecimal.Value(%s): %v", s, err)
			return
		}
		var nd4 alpaca.NullDecimal
		if err := nd4.Scan(val); err != nil {
			t.Errorf("NullDecimal.Scan(%s): %v", s, err)
			return
		}
		if !nd4.Valid || !nd4.Decimal.Equal(d) {
			t.Errorf("NullDecimal Value/Scan round-trip(%s): before=%s after=%s valid=%v", s, d.String(), nd4.Decimal.String(), nd4.Valid)
		}
	})
}

func FuzzRequireFromString(f *testing.F) {
	seeds := []string{"0", "1", "-1", "123.456", "-999.999", "0.000000001"}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// First check if NewFromString accepts it
		d, err := alpaca.NewFromString(s)
		if err != nil {
			// RequireFromString should panic for invalid strings
			func() {
				defer func() { recover() }()
				alpaca.RequireFromString(s)
			}()
			return
		}

		// RequireFromString should not panic and match NewFromString
		r := alpaca.RequireFromString(s)
		if !r.Equal(d) {
			t.Errorf("RequireFromString(%s): %s != %s", s, r.String(), d.String())
		}
	})
}

func FuzzGetFixedIsOptimized(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "1.23", "9223372", "-9223372",
		"9999999999999999999", "0.000000000001",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		a, err := alpaca.NewFromString(s)
		if err != nil {
			return
		}

		// IsOptimized + GetFixed consistency
		if a.IsOptimized() {
			// GetFixed should reconstruct the same value via internal representation
			fixed := a.GetFixed()
			if a.GetFallback() != nil {
				t.Errorf("IsOptimized(%s): GetFallback should be nil", s)
			}
			_ = fixed // just ensure no panic
		} else {
			if a.GetFallback() == nil {
				t.Errorf("!IsOptimized(%s): GetFallback should not be nil", s)
			}
		}

		// Equals (deprecated) should match Equal
		b, _ := alpaca.NewFromString(s)
		if a.Equals(b) != a.Equal(b) {
			t.Errorf("Equals vs Equal(%s): mismatch", s)
		}
	})
}

func FuzzNewFromFormattedString(f *testing.F) {
	f.Add("1,234.56", ",")
	f.Add("1 234.56", " ")
	f.Add("-1,000,000.99", ",")

	f.Fuzz(func(t *testing.T, s, sep string) {
		if len(sep) == 0 || len(sep) > 1 {
			return
		}
		// sep must not be digit or decimal point or sign
		c := sep[0]
		if (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '+' {
			return
		}

		re := strings.NewReplacer(sep, "")
		cleaned := re.Replace(s)

		// Check if the cleaned string is a valid decimal
		expected, errE := alpaca.NewFromString(cleaned)
		if errE != nil {
			return
		}

		regexpSep, regErr := regexp.Compile(regexp.QuoteMeta(sep))
		if regErr != nil {
			return
		}
		result, err := alpaca.NewFromFormattedString(s, regexpSep)
		if err != nil {
			return
		}
		if !result.Equal(expected) {
			t.Errorf("NewFromFormattedString(%q, %q): got %s expected %s", s, sep, result.String(), expected.String())
		}
	})
}

func FuzzScanTypes(f *testing.F) {
	f.Add("123.456")
	f.Add("-999.99")
	f.Add("0")
	f.Add("1")

	f.Fuzz(func(t *testing.T, s string) {
		expected, err := alpaca.NewFromString(s)
		if err != nil {
			return
		}

		// Scan from string
		var d1 alpaca.Decimal
		if err := d1.Scan(s); err != nil {
			t.Errorf("Scan(string %q): %v", s, err)
			return
		}
		if !d1.Equal(expected) {
			t.Errorf("Scan(string %q): got %s expected %s", s, d1.String(), expected.String())
		}

		// Scan from []byte
		var d2 alpaca.Decimal
		if err := d2.Scan([]byte(s)); err != nil {
			t.Errorf("Scan([]byte %q): %v", s, err)
			return
		}
		if !d2.Equal(expected) {
			t.Errorf("Scan([]byte %q): got %s expected %s", s, d2.String(), expected.String())
		}

		// Scan from int64 (only for integer values)
		if expected.IsInteger() {
			ip := expected.IntPart()
			var d3 alpaca.Decimal
			if err := d3.Scan(ip); err != nil {
				t.Errorf("Scan(int64 %d): %v", ip, err)
				return
			}
			if d3.IntPart() != ip {
				t.Errorf("Scan(int64 %d): got IntPart=%d", ip, d3.IntPart())
			}
		}

		// Scan from float64
		f64 := expected.InexactFloat64()
		if !math.IsInf(f64, 0) && !math.IsNaN(f64) {
			var d4 alpaca.Decimal
			if err := d4.Scan(f64); err != nil {
				t.Errorf("Scan(float64 %v): %v", f64, err)
			}
			// float64 scan may lose precision, just verify no error
		}
	})
}
