// Package fuzz provides fuzz tests that validate alpacadecimal against
// shopspring/decimal as a reference implementation.
//
// alpacadecimal uses an optimized fixed-point representation for values where
// |integer part| <= 9,223,372 with up to 12 fractional digits. Larger values
// fall back to udecimal, which supports up to 19 fractional digits and absolute
// values up to approximately 3.4 * 10^19 (34 quintillion).
//
// All fuzz inputs are constrained to this representable range. Where precision
// semantics differ between the two libraries (e.g., multiplication truncates to
// 19 digits, division defaults to 16-digit precision), tests apply appropriate
// tolerances. Inputs that exceed these bounds are skipped, not because they are
// invalid, but because the two libraries would legitimately diverge.
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

	"github.com/quagmt/udecimal"
	shopspring "github.com/shopspring/decimal"

	alpaca "github.com/alpacahq/alpacadecimal"
)

// ---------------------------------------------------------------------------
// Precision constraints
// ---------------------------------------------------------------------------

// maxPrec is the maximum number of fractional digits udecimal supports.
// Inputs exceeding this are skipped because alpacadecimal truncates at 19
// digits while shopspring preserves arbitrary precision.
const maxPrec = 19

// maxAbsValue bounds test inputs to 34 quintillion. This keeps values within
// udecimal's representable range and avoids its 200-character string parse limit.
var maxAbsValue = alpaca.RequireFromString("34000000000000000000")

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// inRange returns true if d is within [-3.4e19, 3.4e19].
func inRange(d alpaca.Decimal) bool {
	return d.Abs().LessThanOrEqual(maxAbsValue)
}

// fracDigitsOf returns the number of characters after the decimal point in s.
func fracDigitsOf(s string) int {
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		return len(s) - idx - 1
	}
	return 0
}

// trimTrailing strips insignificant trailing zeros and the decimal point so
// that "1.230" and "1.23" compare equal.
func trimTrailing(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// compare asserts that alpacadecimal and shopspring produce the same string
// after normalizing trailing zeros.
func compare(t *testing.T, op string, alpacaResult alpaca.Decimal, shopResult shopspring.Decimal) {
	t.Helper()
	a := trimTrailing(alpacaResult.String())
	s := trimTrailing(shopResult.String())
	if a != s {
		t.Errorf("%s: alpaca=%q shopspring=%q", op, alpacaResult.String(), shopResult.String())
	}
}

// compareTruncated truncates both results to the given number of decimal places
// before comparing. This accounts for cases where alpacadecimal and shopspring
// produce different trailing digits (e.g., multiplication).
func compareTruncated(t *testing.T, op string, alpacaResult alpaca.Decimal, shopResult shopspring.Decimal, places int32) {
	t.Helper()
	a := trimTrailing(alpacaResult.Truncate(places).String())
	s := trimTrailing(shopResult.Truncate(places).String())
	if a != s {
		t.Errorf("%s: alpaca=%q shopspring=%q (truncated to %d)", op, a, s, places)
	}
}

// compareDivision truncates both results to 16 decimal places before comparing.
// Both libraries default to DivisionPrecision=16.
func compareDivision(t *testing.T, op string, alpacaResult alpaca.Decimal, shopResult shopspring.Decimal) {
	t.Helper()
	a := trimTrailing(alpacaResult.Truncate(16).String())
	s := trimTrailing(shopResult.Truncate(16).String())
	if a != s {
		t.Errorf("%s: alpaca=%q shopspring=%q (truncated to 16)", op, a, s)
	}
}

// safeCall executes fn and returns false if it panics.
func safeCall(fn func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	fn()
	return true
}

// safeNewFromFloat wraps NewFromFloat, returning false on panic.
func safeNewFromFloat(v float64) (d alpaca.Decimal, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	return alpaca.NewFromFloat(v), true
}

// randDecimalString generates a random decimal string suitable for testing.
// The integer part has 0-15 digits and the fractional part has 0-maxPrec digits.
func randDecimalString(rng *rand.Rand) string {
	neg := rng.Intn(2) == 0
	intDigits := rng.Intn(16)
	fracDigits := rng.Intn(maxPrec + 1)

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	if intDigits == 0 {
		b.WriteByte('0')
	} else {
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

// ---------------------------------------------------------------------------
// Constructor tests
// ---------------------------------------------------------------------------

// FuzzNew tests New(value, exp) and NewFromBigInt(big.Int, exp) against
// shopspring for integer coefficients with various exponents.
func FuzzNew(f *testing.F) {
	for _, v := range []int64{0, 1, -1, 100, -100, 123, -456, 999999999} {
		for _, e := range []int32{-12, -5, -2, 0, 2, 5} {
			f.Add(v, e)
		}
	}

	f.Fuzz(func(t *testing.T, value int64, exp int32) {
		// Exponent beyond [-19, 19] can overflow or exceed udecimal's range.
		if exp < -19 || exp > 19 {
			return
		}

		// New(value, exp)
		var aNew alpaca.Decimal
		if !safeCall(func() { aNew = alpaca.New(value, exp) }) {
			return
		}
		sNew := shopspring.New(value, exp)
		compare(t, fmt.Sprintf("New(%d, %d)", value, exp), aNew, sNew)

		// NewFromBigInt(big.NewInt(value), exp)
		bi := big.NewInt(value)
		var aBig alpaca.Decimal
		if !safeCall(func() { aBig = alpaca.NewFromBigInt(bi, exp) }) {
			return
		}
		sBig := shopspring.NewFromBigInt(bi, exp)
		compare(t, fmt.Sprintf("NewFromBigInt(%d, %d)", value, exp), aBig, sBig)
	})
}

// FuzzNewFromInt tests NewFromInt and NewFromInt32.
func FuzzNewFromInt(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(-1))
	f.Add(int64(100))
	f.Add(int64(-100))
	f.Add(int64(math.MaxInt32))
	f.Add(int64(math.MinInt32))
	f.Add(int64(9223372))  // max optimized integer part
	f.Add(int64(-9223372)) // min optimized integer part
	f.Add(int64(9223373))  // just beyond optimized range

	f.Fuzz(func(t *testing.T, v int64) {
		a := alpaca.NewFromInt(v)
		s := shopspring.NewFromInt(v)
		compare(t, fmt.Sprintf("NewFromInt(%d)", v), a, s)

		if v >= math.MinInt32 && v <= math.MaxInt32 {
			a32 := alpaca.NewFromInt32(int32(v))
			compare(t, fmt.Sprintf("NewFromInt32(%d)", v), a32, s)
		}
	})
}

// FuzzNewFromFloat tests NewFromFloat with float64 inputs.
// Floats requiring >19 fractional digits when formatted are skipped because the
// two libraries use different rounding modes for truncation at the precision limit.
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
	f.Add(9223372.0) // optimized boundary
	f.Add(9223373.0) // fallback
	f.Add(1e12)
	f.Add(-1e12)

	f.Fuzz(func(t *testing.T, v float64) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return
		}
		// Limit range to avoid float64 precision artifacts for extreme values.
		if v > 1e15 || v < -1e15 {
			return
		}

		a, ok := safeNewFromFloat(v)
		if !ok {
			return
		}
		s := shopspring.NewFromFloat(v)

		// Skip values whose minimal float representation exceeds 19 fractional
		// digits, since alpacadecimal and shopspring differ in truncation mode.
		str := strconv.FormatFloat(v, 'f', -1, 64)
		if dotIdx := strings.IndexByte(str, '.'); dotIdx >= 0 && len(str)-dotIdx-1 > 19 {
			return
		}
		compare(t, fmt.Sprintf("NewFromFloat(%v)", v), a, s)
	})
}

// FuzzNewFromFloat32 tests NewFromFloat32 with float32 inputs.
func FuzzNewFromFloat32(f *testing.F) {
	f.Add(float32(0.0))
	f.Add(float32(1.0))
	f.Add(float32(-1.0))
	f.Add(float32(0.1))
	f.Add(float32(-0.1))
	f.Add(float32(123.456))
	f.Add(float32(-123.456))

	f.Fuzz(func(t *testing.T, v float32) {
		if v != v || math.IsInf(float64(v), 0) { // NaN or Inf
			return
		}
		if v > 1e10 || v < -1e10 {
			return
		}

		var a alpaca.Decimal
		if !safeCall(func() { a = alpaca.NewFromFloat32(v) }) {
			return
		}
		// shopspring may produce more fractional digits than udecimal's 19-digit
		// limit; truncate shopspring's result to match.
		s := shopspring.NewFromFloat32(v).Truncate(19)
		compare(t, fmt.Sprintf("NewFromFloat32(%v)", v), a, s)
	})
}

// FuzzNewFromFloatWithExponent verifies that NewFromFloatWithExponent(v, exp)
// is equivalent to NewFromFloat(v).Truncate(-exp). shopspring's version rounds
// instead of truncating, so we compare internally rather than cross-library.
func FuzzNewFromFloatWithExponent(f *testing.F) {
	f.Add(123.456, int32(-2))
	f.Add(-123.456, int32(-2))
	f.Add(0.0, int32(0))
	f.Add(1.0, int32(-5))
	f.Add(99.995, int32(-2))
	f.Add(9223372.5, int32(-1)) // optimized boundary

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

		var a, expected alpaca.Decimal
		if !safeCall(func() {
			a = alpaca.NewFromFloatWithExponent(v, exp)
			expected = alpaca.NewFromFloat(v).Truncate(-exp)
		}) {
			return
		}
		if !a.Equal(expected) {
			t.Errorf("NewFromFloatWithExponent(%v, %d): got %s expected %s",
				v, exp, a.String(), expected.String())
		}
	})
}

// FuzzNewFromString tests NewFromString and RequireFromString for parsing
// consistency and panic/error agreement.
func FuzzNewFromString(f *testing.F) {
	for _, s := range []string{
		"0", "1", "-1", "0.5", "-0.5",
		"123.456", "-123.456",
		"0.000000001", "0.000000000001",
		"9223372", "-9223372",
		"9223373", "-9223373",
		"999999999.999999999",
		"0.1234567890123456789",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		if fracDigitsOf(s) > maxPrec {
			return
		}

		d, err := alpaca.NewFromString(s)
		if err != nil {
			// RequireFromString must panic for inputs that NewFromString rejects.
			panicked := false
			func() {
				defer func() {
					if recover() != nil {
						panicked = true
					}
				}()
				alpaca.RequireFromString(s)
			}()
			if !panicked {
				t.Errorf("RequireFromString(%q) should have panicked", s)
			}
			return
		}
		if !inRange(d) {
			return
		}

		// RequireFromString must return the same value.
		r := alpaca.RequireFromString(s)
		if !r.Equal(d) {
			t.Errorf("RequireFromString(%s) = %s, NewFromString = %s", s, r.String(), d.String())
		}

		// Round-trip: String() -> NewFromString must preserve the value.
		rt, err := alpaca.NewFromString(d.String())
		if err != nil {
			t.Errorf("String round-trip(%s): parse error: %v", s, err)
		} else if !rt.Equal(d) {
			t.Errorf("String round-trip(%s): %q -> %q", s, d.String(), rt.String())
		}

		// Cross-library: shopspring must parse to the same value.
		shopD, errS := shopspring.NewFromString(s)
		if errS != nil {
			return
		}
		compare(t, fmt.Sprintf("NewFromString(%s)", s), d, shopD)
	})
}

// FuzzNewFromFormattedString tests parsing strings with embedded separators.
func FuzzNewFromFormattedString(f *testing.F) {
	f.Add("1,234.56", ",")
	f.Add("1 234.56", " ")
	f.Add("-1,000,000.99", ",")
	f.Add("1_000.50", "_")

	f.Fuzz(func(t *testing.T, s, sep string) {
		if len(sep) != 1 {
			return
		}
		c := sep[0]
		if (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '+' {
			return
		}

		cleaned := strings.NewReplacer(sep, "").Replace(s)
		expected, err := alpaca.NewFromString(cleaned)
		if err != nil {
			return
		}
		if !inRange(expected) {
			return
		}

		re, err := regexp.Compile(regexp.QuoteMeta(sep))
		if err != nil {
			return
		}
		result, err := alpaca.NewFromFormattedString(s, re)
		if err != nil {
			return
		}
		if !result.Equal(expected) {
			t.Errorf("NewFromFormattedString(%q, %q): got %s expected %s",
				s, sep, result.String(), expected.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Arithmetic tests
// ---------------------------------------------------------------------------

// FuzzArithmetic tests binary and unary arithmetic operations along with all
// comparison operators. These are grouped because they share the same two-operand
// fuzz signature and are closely related.
func FuzzArithmetic(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "0.5", "-0.5",
		"123.456", "-123.456",
		"0.000000001", "-0.000000001",
		"0.000000000001",      // 12 fractional digits: optimized boundary
		"9223372", "-9223372", // max optimized integer
		"9223372.000000000001", // max optimized value
		"9223373", "-9223373",  // forces fallback
		"9999999.999999999",         // large fractional
		"1000000000", "-1000000000", // large integer, fallback
	}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b)
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string) {
		if fracDigitsOf(aStr) > maxPrec || fracDigitsOf(bStr) > maxPrec {
			return
		}

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
		if !inRange(a) || !inRange(b) {
			return
		}

		// --- Unary operations on a ---

		compare(t, fmt.Sprintf("Neg(%s)", aStr), a.Neg(), shopA.Neg())
		compare(t, fmt.Sprintf("Abs(%s)", aStr), a.Abs(), shopA.Abs())

		if a.Sign() != shopA.Sign() {
			t.Errorf("Sign(%s): alpaca=%d shopspring=%d", aStr, a.Sign(), shopA.Sign())
		}
		if a.IsZero() != shopA.IsZero() {
			t.Errorf("IsZero(%s): alpaca=%v shopspring=%v", aStr, a.IsZero(), shopA.IsZero())
		}
		if a.IsNegative() != shopA.IsNegative() {
			t.Errorf("IsNegative(%s): alpaca=%v shopspring=%v", aStr, a.IsNegative(), shopA.IsNegative())
		}
		if a.IsPositive() != shopA.IsPositive() {
			t.Errorf("IsPositive(%s): alpaca=%v shopspring=%v", aStr, a.IsPositive(), shopA.IsPositive())
		}

		// --- Binary arithmetic ---

		compare(t, fmt.Sprintf("Add(%s, %s)", aStr, bStr), a.Add(b), shopA.Add(shopB))
		compare(t, fmt.Sprintf("Sub(%s, %s)", aStr, bStr), a.Sub(b), shopA.Sub(shopB))

		// Mul: udecimal truncates the result to 19 fractional digits while
		// shopspring retains arbitrary precision. Truncate both to 19 to compare.
		compareTruncated(t, fmt.Sprintf("Mul(%s, %s)", aStr, bStr),
			a.Mul(b), shopA.Mul(shopB), maxPrec)

		// Div: both libraries default to DivisionPrecision=16. Skip zero divisor
		// and excessively long inputs that may overflow intermediate calculations.
		if !b.IsZero() && len(aStr) < 40 && len(bStr) < 40 {
			compareDivision(t, fmt.Sprintf("Div(%s, %s)", aStr, bStr),
				a.Div(b), shopA.Div(shopB))
		}

		// Mod: skip zero divisor.
		if !b.IsZero() {
			compare(t, fmt.Sprintf("Mod(%s, %s)", aStr, bStr), a.Mod(b), shopA.Mod(shopB))
		}

		// --- Comparison operators ---

		if got, want := a.Cmp(b), shopA.Cmp(shopB); got != want {
			t.Errorf("Cmp(%s, %s): alpaca=%d shopspring=%d", aStr, bStr, got, want)
		}
		if got, want := a.Equal(b), shopA.Equal(shopB); got != want {
			t.Errorf("Equal(%s, %s): alpaca=%v shopspring=%v", aStr, bStr, got, want)
		}
		if got, want := a.GreaterThan(b), shopA.GreaterThan(shopB); got != want {
			t.Errorf("GreaterThan(%s, %s): alpaca=%v shopspring=%v", aStr, bStr, got, want)
		}
		if got, want := a.GreaterThanOrEqual(b), shopA.GreaterThanOrEqual(shopB); got != want {
			t.Errorf("GreaterThanOrEqual(%s, %s): alpaca=%v shopspring=%v", aStr, bStr, got, want)
		}
		if got, want := a.LessThan(b), shopA.LessThan(shopB); got != want {
			t.Errorf("LessThan(%s, %s): alpaca=%v shopspring=%v", aStr, bStr, got, want)
		}
		if got, want := a.LessThanOrEqual(b), shopA.LessThanOrEqual(shopB); got != want {
			t.Errorf("LessThanOrEqual(%s, %s): alpaca=%v shopspring=%v", aStr, bStr, got, want)
		}
	})
}

// FuzzDivRound tests DivRound with explicit precision.
func FuzzDivRound(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "3", "7", "10",
		"123.456", "-123.456",
		"9223372", "-9223372",
		"0.000000001",
	}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b, int8(4))
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string, prec int8) {
		if prec < 0 || prec > 16 {
			return
		}
		if fracDigitsOf(aStr) > maxPrec || fracDigitsOf(bStr) > maxPrec {
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
		if !inRange(a) || !inRange(b) || b.IsZero() {
			return
		}

		aResult := a.DivRound(b, p)
		sResult := shopA.DivRound(shopB, p)
		compare(t, fmt.Sprintf("DivRound(%s, %s, %d)", aStr, bStr, p), aResult, sResult)
	})
}

// FuzzQuoRem tests QuoRem (division with remainder).
//
// Validated properties:
//  1. Cross-library: quotient and remainder match shopspring.
//  2. Invariant: q * d2 + r == d (the fundamental quotient-remainder identity).
//  3. Remainder bound: |r| < |d2| * 10^(-prec) (remainder is strictly smaller
//     than one unit in the last quotient digit).
//  4. Sign: remainder has the same sign as the dividend (truncated division).
func FuzzQuoRem(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "0.5", "-0.5",
		"3", "-3", "7", "-7", "10", "-10",
		"123.456", "-123.456",
		"0.000000001", "-0.000000001",
		"0.000000000001",      // 12 fractional digits: optimized boundary
		"9223372", "-9223372", // max optimized integer
		"9223372.000000000001", // max optimized value
		"9223373", "-9223373",  // forces fallback
		"1000000000", "-1000000000", // large fallback
		"0.1", "0.01", "0.001",
		"999999.999999",
	}
	precs := []int8{0, 1, 2, 4, 8, 12, 16}
	for _, a := range seeds {
		for _, b := range seeds {
			for _, p := range precs {
				f.Add(a, b, p)
			}
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string, prec int8) {
		if prec < 0 || prec > 16 {
			return
		}
		if fracDigitsOf(aStr) > maxPrec || fracDigitsOf(bStr) > maxPrec {
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
		if !inRange(a) || !inRange(b) || b.IsZero() {
			return
		}

		// Zero dividend: quotient and remainder must both be zero regardless of
		// precision. Verify this explicitly before the general path.
		if a.IsZero() {
			aQ, aR := a.QuoRem(b, p)
			if !aQ.IsZero() {
				t.Errorf("QuoRem_q(0, %s, %d): expected 0, got %s", bStr, p, aQ.String())
			}
			if !aR.IsZero() {
				t.Errorf("QuoRem_r(0, %s, %d): expected 0, got %s", bStr, p, aR.String())
			}
			return
		}

		// The remainder has commonPrec + prec implicit decimal places.
		// Skip when this exceeds 19 to avoid divergence at the precision boundary.
		maxFrac := fracDigitsOf(bStr)
		if af := fracDigitsOf(aStr); af > maxFrac {
			maxFrac = af
		}
		if maxFrac+int(p) > 19 {
			return
		}

		aQ, aR := a.QuoRem(b, p)
		sQ, sR := shopA.QuoRem(shopB, p)

		// Property 1: cross-library agreement.
		compare(t, fmt.Sprintf("QuoRem_q(%s, %s, %d)", aStr, bStr, p), aQ, sQ)
		compare(t, fmt.Sprintf("QuoRem_r(%s, %s, %d)", aStr, bStr, p), aR, sR)

		// Property 2: quotient-remainder identity q * d2 + r == d.
		// Use truncated comparison because q * d2 multiplication may introduce
		// rounding noise beyond 19 fractional digits.
		reconstructed := aQ.Mul(b).Add(aR)
		if !reconstructed.Equal(a) {
			// Allow for truncation artifacts: compare truncated to maxPrec.
			rTrunc := trimTrailing(reconstructed.Truncate(maxPrec).String())
			aTrunc := trimTrailing(a.Truncate(maxPrec).String())
			if rTrunc != aTrunc {
				t.Errorf("QuoRem invariant q*d2+r==d failed for (%s, %s, %d): q=%s r=%s reconstructed=%s original=%s",
					aStr, bStr, p, aQ.String(), aR.String(), reconstructed.String(), a.String())
			}
		}

		// Property 3: |r| < |d2| * 10^(-prec), i.e. remainder is smaller than
		// one unit in the last place of the quotient.
		// Expressed equivalently: |r| * 10^prec < |d2|.
		if !aR.IsZero() {
			rScaled := aR.Abs().Shift(p)
			if rScaled.GreaterThanOrEqual(b.Abs()) {
				t.Errorf("QuoRem remainder bound violated for (%s, %s, %d): |r|=%s |d2|=%s |r|*10^%d=%s",
					aStr, bStr, p, aR.Abs().String(), b.Abs().String(), p, rScaled.String())
			}
		}

		// Property 4: remainder sign matches dividend sign (truncated division).
		if !aR.IsZero() {
			if a.IsPositive() && aR.IsNegative() {
				t.Errorf("QuoRem sign mismatch for (%s, %s, %d): positive dividend but negative remainder %s",
					aStr, bStr, p, aR.String())
			}
			if a.IsNegative() && aR.IsPositive() {
				t.Errorf("QuoRem sign mismatch for (%s, %s, %d): negative dividend but positive remainder %s",
					aStr, bStr, p, aR.String())
			}
		}
	})
}

// FuzzPow tests Pow with integer exponents.
// Non-integer exponents are not supported by shopspring and are skipped.
func FuzzPow(f *testing.F) {
	bases := []string{"0", "1", "-1", "2", "-2", "1.5", "10", "0.5", "3"}
	exps := []string{"0", "1", "2", "3", "4", "5", "-1", "-2"}
	for _, base := range bases {
		for _, exp := range exps {
			f.Add(base, exp)
		}
	}

	f.Fuzz(func(t *testing.T, baseStr, expStr string) {
		if fracDigitsOf(baseStr) > maxPrec || fracDigitsOf(expStr) > maxPrec {
			return
		}

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
		if !inRange(base) || !inRange(exp) {
			return
		}
		if !exp.IsInteger() {
			return
		}

		// IntPart panics when the integer exceeds int64 bounds (e.g., a string
		// with many leading zeros that parses into a fallback representation).
		var expInt int64
		if !safeCall(func() { expInt = exp.IntPart() }) {
			return
		}
		if expInt < -5 || expInt > 10 {
			return
		}
		// 0^negative is undefined; 0^0 behavior differs between implementations.
		if base.IsZero() && expInt <= 0 {
			return
		}

		var aResult alpaca.Decimal
		if !safeCall(func() { aResult = base.Pow(exp) }) {
			return
		}
		sResult := shopBase.Pow(shopExp)

		if expInt >= 0 {
			// Positive exponents: compare truncated to 12 digits because repeated
			// multiplication accumulates rounding differences.
			compareTruncated(t, fmt.Sprintf("Pow(%s, %s)", baseStr, expStr),
				aResult, sResult, 12)
		}
		// Negative exponents involve division where last-digit rounding may
		// differ between the two libraries (shopspring uses DivisionPrecision=16
		// with round-half-up, alpacadecimal uses DivRound with round-half-away-
		// from-zero). Division correctness is validated by FuzzDivRound; here we
		// only verify no panic for negative exponents.
		_ = aResult
	})
}

// FuzzShift tests Shift (multiplication by 10^shift).
func FuzzShift(f *testing.F) {
	seeds := []string{"0", "1", "-1", "123.456", "-123.456", "0.001", "9223372"}
	for _, s := range seeds {
		for shift := int32(-5); shift <= 5; shift++ {
			f.Add(s, shift)
		}
	}

	f.Fuzz(func(t *testing.T, s string, shift int32) {
		if shift < -8 || shift > 8 {
			return
		}
		if fracDigitsOf(s) > maxPrec {
			return
		}

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}
		if !inRange(a) {
			return
		}

		// A negative shift increases fractional digits. Skip if the result
		// would exceed udecimal's 19-digit precision limit.
		if shift < 0 && fracDigitsOf(s)+int(-shift) > 19 {
			return
		}

		var aResult alpaca.Decimal
		if !safeCall(func() { aResult = a.Shift(shift) }) {
			return
		}
		compare(t, fmt.Sprintf("Shift(%s, %d)", s, shift), aResult, shopD.Shift(shift))
	})
}

// ---------------------------------------------------------------------------
// Rounding tests
// ---------------------------------------------------------------------------

// FuzzRounding tests all rounding methods: Round (half away from zero),
// RoundBank (half to even), RoundCeil, RoundFloor, RoundUp, RoundDown,
// and Truncate.
func FuzzRounding(f *testing.F) {
	seeds := []string{
		"0", "1.5", "-1.5", "2.5", "-2.5",
		"1.45", "1.55", "123.456", "-123.456",
		"0.999", "-0.999", "1.005", "99.995",
		"9223372", "-9223372", // optimized boundary
		"9223372.5", "-9223372.5", // optimized with rounding
		"9223373", "-9223373", // fallback
		"1000000000", "-1000000000", // large fallback
	}
	for _, s := range seeds {
		for places := int8(-5); places <= 15; places++ {
			f.Add(s, places)
		}
	}

	f.Fuzz(func(t *testing.T, s string, places int8) {
		if places < -5 || places > 18 {
			return
		}
		if fracDigitsOf(s) > maxPrec {
			return
		}
		p := int32(places)

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}
		if !inRange(a) {
			return
		}

		compare(t, fmt.Sprintf("Round(%s, %d)", s, p), a.Round(p), shopD.Round(p))
		compare(t, fmt.Sprintf("RoundBank(%s, %d)", s, p), a.RoundBank(p), shopD.RoundBank(p))
		compare(t, fmt.Sprintf("RoundCeil(%s, %d)", s, p), a.RoundCeil(p), shopD.RoundCeil(p))
		compare(t, fmt.Sprintf("RoundFloor(%s, %d)", s, p), a.RoundFloor(p), shopD.RoundFloor(p))
		compare(t, fmt.Sprintf("RoundUp(%s, %d)", s, p), a.RoundUp(p), shopD.RoundUp(p))
		compare(t, fmt.Sprintf("RoundDown(%s, %d)", s, p), a.RoundDown(p), shopD.RoundDown(p))

		// Truncate: only compared for non-negative places. For negative places,
		// shopspring and alpacadecimal have different semantics: shopspring treats
		// Truncate(-n) as a no-op for values smaller than 10^n, while alpacadecimal
		// truncates the integer part toward zero to the nearest 10^n. Both are
		// valid interpretations; we test only the common ground.
		if p >= 0 {
			compare(t, fmt.Sprintf("Truncate(%s, %d)", s, p), a.Truncate(p), shopD.Truncate(p))
		}
	})
}

// FuzzRoundDown tests RoundDown (truncation towards zero) in isolation.
//
// Validated properties:
//  1. Cross-library: result matches shopspring.
//  2. Idempotence: RoundDown(RoundDown(d, p), p) == RoundDown(d, p).
//  3. Towards zero: |RoundDown(d, p)| <= |d|.
func FuzzRoundDown(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "0.5", "-0.5",
		"1.1001", "-1.1001", "1.999", "-1.999",
		"123.456", "-123.456",
		"454545.454545", "-454545.454545",
		"0.001", "-0.001",
		"99", "-99", "545", "-545",
		"9223372.999", "-9223372.999", // optimized boundary
		"9223373.999", "-9223373.999", // fallback
	}
	for _, s := range seeds {
		for places := int8(-5); places <= 10; places++ {
			f.Add(s, places)
		}
	}

	f.Fuzz(func(t *testing.T, s string, places int8) {
		if places < -5 || places > 18 {
			return
		}
		if fracDigitsOf(s) > maxPrec {
			return
		}
		p := int32(places)

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}
		if !inRange(a) {
			return
		}

		rd := a.RoundDown(p)

		// Property 1: cross-library agreement.
		compare(t, fmt.Sprintf("RoundDown(%s, %d)", s, p), rd, shopD.RoundDown(p))

		// Property 2: idempotence.
		rd2 := rd.RoundDown(p)
		if !rd2.Equal(rd) {
			t.Errorf("RoundDown idempotence(%s, %d): first=%s second=%s", s, p, rd.String(), rd2.String())
		}

		// Property 3: towards zero — |result| <= |original|.
		if rd.Abs().GreaterThan(a.Abs()) {
			t.Errorf("RoundDown towards zero(%s, %d): |%s| > |%s|", s, p, rd.String(), a.String())
		}
	})
}

// FuzzCeilFloor tests Ceil and Floor.
func FuzzCeilFloor(f *testing.F) {
	for _, s := range []string{
		"0", "1", "-1", "1.5", "-1.5", "2.5", "-2.5",
		"1.001", "-1.001", "0.999", "-0.999",
		"123.456", "-123.456",
		"9223372", "-9223372", "9223372.5",
		"9223373", "-9223373",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		if fracDigitsOf(s) > maxPrec {
			return
		}

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}
		if !inRange(a) {
			return
		}

		compare(t, fmt.Sprintf("Ceil(%s)", s), a.Ceil(), shopD.Ceil())
		compare(t, fmt.Sprintf("Floor(%s)", s), a.Floor(), shopD.Floor())
	})
}

// FuzzRoundCash tests RoundCash and StringFixedCash for all valid intervals.
func FuzzRoundCash(f *testing.F) {
	seeds := []string{"0", "1.23", "-1.23", "3.43", "3.45", "3.41", "3.75", "3.50"}
	intervals := []uint8{5, 10, 25, 50, 100}
	for _, s := range seeds {
		for _, iv := range intervals {
			f.Add(s, iv)
		}
	}

	f.Fuzz(func(t *testing.T, s string, interval uint8) {
		switch interval {
		case 5, 10, 25, 50, 100:
		default:
			return
		}
		if fracDigitsOf(s) > maxPrec {
			return
		}

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}
		if !inRange(a) {
			return
		}

		compare(t, fmt.Sprintf("RoundCash(%s, %d)", s, interval),
			a.RoundCash(interval), shopD.RoundCash(interval))

		if got, want := a.StringFixedCash(interval), shopD.StringFixedCash(interval); got != want {
			t.Errorf("StringFixedCash(%s, %d): alpaca=%q shopspring=%q", s, interval, got, want)
		}
	})
}

// ---------------------------------------------------------------------------
// Aggregate tests
// ---------------------------------------------------------------------------

// FuzzAggregates tests Sum, Avg, Max, and Min.
func FuzzAggregates(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "123.456", "-999.999", "0.001",
		"9223372", "-9223372", "9223373",
	}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b)
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string) {
		if fracDigitsOf(aStr) > maxPrec || fracDigitsOf(bStr) > maxPrec {
			return
		}

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
		if !inRange(a) || !inRange(b) {
			return
		}

		compare(t, fmt.Sprintf("Sum(%s, %s)", aStr, bStr),
			alpaca.Sum(a, b), shopspring.Sum(shopA, shopB))

		// Avg involves addition followed by division. Both libraries default to
		// DivisionPrecision=16, so we compare truncated to that precision.
		compareDivision(t, fmt.Sprintf("Avg(%s, %s)", aStr, bStr),
			alpaca.Avg(a, b), shopspring.Avg(shopA, shopB))

		compare(t, fmt.Sprintf("Max(%s, %s)", aStr, bStr),
			alpaca.Max(a, b), shopspring.Max(shopA, shopB))
		compare(t, fmt.Sprintf("Min(%s, %s)", aStr, bStr),
			alpaca.Min(a, b), shopspring.Min(shopA, shopB))
	})
}

// ---------------------------------------------------------------------------
// Introspection tests
// ---------------------------------------------------------------------------

// FuzzIntrospection tests single-value introspection methods: IntPart,
// IsInteger, InexactFloat64, Float64, NumDigits, and Copy.
func FuzzIntrospection(f *testing.F) {
	for _, s := range []string{
		"0", "1", "-1", "1.5", "-1.5",
		"100", "0.001",
		"9223372", "-9223372", // max optimized integer
		"9223372.000000000001", // max optimized value
		"9223373", "-9223373",  // forces fallback
		"9999999999", "-9999999999", // large fallback
		"0.1234567890123456789", // 19 fractional digits
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		if fracDigitsOf(s) > maxPrec {
			return
		}

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}
		if !inRange(a) {
			return
		}

		// IntPart: skip values whose integer part overflows int64.
		if len(s) <= 18 {
			if got, want := a.IntPart(), shopD.IntPart(); got != want {
				t.Errorf("IntPart(%s): alpaca=%d shopspring=%d", s, got, want)
			}
		}

		if got, want := a.IsInteger(), shopD.IsInteger(); got != want {
			t.Errorf("IsInteger(%s): alpaca=%v shopspring=%v", s, got, want)
		}

		// InexactFloat64: compare with tolerance for float representation error.
		if af, sf := a.InexactFloat64(), shopD.InexactFloat64(); math.Abs(af-sf) > 1e-10 {
			t.Errorf("InexactFloat64(%s): alpaca=%v shopspring=%v", s, af, sf)
		}

		// Float64: compare values (exactness may differ between implementations).
		aF64, _ := a.Float64()
		sF64, _ := shopD.Float64()
		if math.Abs(aF64-sF64) > 1e-10 {
			t.Errorf("Float64(%s): alpaca=%v shopspring=%v", s, aF64, sF64)
		}

		// NumDigits: both libraries count digits of the coefficient, but the
		// internal representations differ (alpacadecimal uses fixed-point scaled
		// coefficients). We verify self-consistency instead of cross-library equality:
		// NumDigits must equal len(abs(Coefficient).String()), or 1 for zero.
		nd := a.NumDigits()
		coef := a.Coefficient()
		expectedDigits := 1
		if coef.Sign() != 0 {
			expectedDigits = len(new(big.Int).Abs(coef).String())
		}
		if nd != expectedDigits {
			t.Errorf("NumDigits(%s): got %d, expected %d (coef=%s)", s, nd, expectedDigits, coef.String())
		}

		// Copy must produce an equal but independent value.
		cp := a.Copy()
		if !cp.Equal(a) {
			t.Errorf("Copy(%s): copy=%s original=%s", s, cp.String(), a.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Conversion tests
// ---------------------------------------------------------------------------

// FuzzConversions tests type conversion and internal-state inspection methods:
// BigInt, BigFloat, Rat, Coefficient/CoefficientInt64, Exponent, IsOptimized,
// GetFixed, GetFallback, Equals (deprecated), and NewFromUDecimal.
func FuzzConversions(f *testing.F) {
	for _, s := range []string{
		"0", "1", "-1", "1.5", "-1.5",
		"100", "0.001",
		"123456", "-999999.99",
		"123.456789", "-999999.123456789012",
		"9223372", "-9223372",
		"9223373", "-9223373",
		"9999999999999999999",
		"0.000000000001",
		"0.1234567890123456789",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		if fracDigitsOf(s) > maxPrec {
			return
		}

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}
		if !inRange(a) {
			return
		}

		// --- BigInt ---
		if got, want := a.BigInt(), shopD.BigInt(); got.Cmp(want) != 0 {
			t.Errorf("BigInt(%s): alpaca=%s shopspring=%s", s, got.String(), want.String())
		}

		// --- BigFloat (compare with tolerance) ---
		aBF := a.BigFloat()
		sBF := shopD.BigFloat()
		diff := new(big.Float).Abs(new(big.Float).Sub(aBF, sBF))
		if diff.Cmp(new(big.Float).SetFloat64(1e-10)) > 0 {
			t.Errorf("BigFloat(%s): alpaca=%s shopspring=%s", s, aBF.String(), sBF.String())
		}

		// --- Rat ---
		if got, want := a.Rat(), shopD.Rat(); got.Cmp(want) != 0 {
			t.Errorf("Rat(%s): alpaca=%s shopspring=%s", s, got.String(), want.String())
		}

		// --- Coefficient/Exponent self-consistency ---
		// The contract: Coefficient * 10^Exponent == original value.
		aExp := a.Exponent()
		aCoef := a.Coefficient()
		reconstructed := alpaca.NewFromBigInt(aCoef, aExp)
		if !reconstructed.Equal(a) {
			t.Errorf("Coefficient*10^Exponent(%s): reconstructed=%s original=%s (coef=%s, exp=%d)",
				s, reconstructed.String(), a.String(), aCoef.String(), aExp)
		}

		// CoefficientInt64 must agree with Coefficient when the latter fits in int64.
		aCoefI64 := a.CoefficientInt64()
		if aCoef.IsInt64() && aCoef.Int64() != aCoefI64 {
			t.Errorf("CoefficientInt64(%s): got %d expected %d", s, aCoefI64, aCoef.Int64())
		}

		// --- IsOptimized / GetFixed / GetFallback ---
		if a.IsOptimized() {
			if a.GetFallback() != nil {
				t.Errorf("IsOptimized(%s): GetFallback should be nil for optimized values", s)
			}
			// GetFixed should not panic.
			_ = a.GetFixed()
		} else {
			if a.GetFallback() == nil {
				t.Errorf("!IsOptimized(%s): GetFallback should be non-nil for fallback values", s)
			}
		}

		// --- Equals (deprecated) must agree with Equal ---
		b, _ := alpaca.NewFromString(s)
		if a.Equals(b) != a.Equal(b) {
			t.Errorf("Equals vs Equal(%s): mismatch", s)
		}

		// --- NewFromUDecimal / NewFromDecimal ---
		// Parse s as udecimal and verify round-trip through NewFromUDecimal.
		ud, err := udecimal.Parse(s)
		if err == nil {
			fromUD := alpaca.NewFromUDecimal(ud)
			if !fromUD.Equal(a) {
				t.Errorf("NewFromUDecimal(%s): got %s expected %s", s, fromUD.String(), a.String())
			}
			// NewFromDecimal is an alias; verify it behaves identically.
			fromD := alpaca.NewFromDecimal(ud)
			if !fromD.Equal(a) {
				t.Errorf("NewFromDecimal(%s): got %s expected %s", s, fromD.String(), a.String())
			}
		}
	})
}

// ---------------------------------------------------------------------------
// String formatting tests
// ---------------------------------------------------------------------------

// FuzzStringFormats tests StringFixed, StringFixedBank, and StringScaled.
func FuzzStringFormats(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "1.23", "-1.23", "0.001", "999.999",
		"9223372.5", "9223373.5",
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
		if fracDigitsOf(s) > maxPrec {
			return
		}
		p := int32(places)

		a, errA := alpaca.NewFromString(s)
		shopD, errS := shopspring.NewFromString(s)
		if errA != nil || errS != nil {
			return
		}
		if !inRange(a) {
			return
		}

		// StringFixed
		if got, want := a.StringFixed(p), shopD.StringFixed(p); got != want {
			t.Errorf("StringFixed(%s, %d): alpaca=%q shopspring=%q", s, p, got, want)
		}

		// StringFixedBank
		if got, want := a.StringFixedBank(p), shopD.StringFixedBank(p); got != want {
			t.Errorf("StringFixedBank(%s, %d): alpaca=%q shopspring=%q", s, p, got, want)
		}

		// StringScaled is deprecated but must remain consistent:
		// StringScaled(-p) == StringFixed(p) by contract.
		if got, want := a.StringScaled(-p), a.StringFixed(p); got != want {
			t.Errorf("StringScaled(-%d) != StringFixed(%d) for %s: %q vs %q", p, p, s, got, want)
		}
	})
}

// ---------------------------------------------------------------------------
// Serialization tests
// ---------------------------------------------------------------------------

// FuzzSerialization tests round-trip consistency for all serialization formats:
// JSON, Text, Binary, Gob, and database Value/Scan.
func FuzzSerialization(f *testing.F) {
	for _, s := range []string{
		"0", "1", "-1", "1.23", "-1.23",
		"0.000000000001", "9999999.999999",
		"123456789.123456789",
		"9223372.000000000001", // max optimized
		"9223373",              // fallback
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		a, err := alpaca.NewFromString(s)
		if err != nil {
			return
		}
		if !inRange(a) {
			return
		}

		// JSON round-trip
		jsonBytes, err := json.Marshal(a)
		if err != nil {
			t.Errorf("MarshalJSON(%s): %v", s, err)
			return
		}
		var aJSON alpaca.Decimal
		if err := json.Unmarshal(jsonBytes, &aJSON); err != nil {
			t.Errorf("UnmarshalJSON(%s): %v (json=%s)", s, err, string(jsonBytes))
			return
		}
		if !aJSON.Equal(a) {
			t.Errorf("JSON round-trip(%s): before=%s after=%s", s, a.String(), aJSON.String())
		}

		// Text round-trip
		textBytes, err := a.MarshalText()
		if err != nil {
			t.Errorf("MarshalText(%s): %v", s, err)
			return
		}
		var aText alpaca.Decimal
		if err := aText.UnmarshalText(textBytes); err != nil {
			t.Errorf("UnmarshalText(%s): %v", s, err)
			return
		}
		if !aText.Equal(a) {
			t.Errorf("Text round-trip(%s): before=%s after=%s", s, a.String(), aText.String())
		}

		// Binary round-trip
		binBytes, err := a.MarshalBinary()
		if err != nil {
			t.Errorf("MarshalBinary(%s): %v", s, err)
			return
		}
		var aBin alpaca.Decimal
		if err := aBin.UnmarshalBinary(binBytes); err != nil {
			t.Errorf("UnmarshalBinary(%s): %v", s, err)
			return
		}
		if !aBin.Equal(a) {
			t.Errorf("Binary round-trip(%s): before=%s after=%s", s, a.String(), aBin.String())
		}

		// Gob round-trip
		gobBytes, err := a.GobEncode()
		if err != nil {
			t.Errorf("GobEncode(%s): %v", s, err)
			return
		}
		var aGob alpaca.Decimal
		if err := aGob.GobDecode(gobBytes); err != nil {
			t.Errorf("GobDecode(%s): %v", s, err)
			return
		}
		if !aGob.Equal(a) {
			t.Errorf("Gob round-trip(%s): before=%s after=%s", s, a.String(), aGob.String())
		}

		// Value/Scan round-trip (database driver interface)
		val, err := a.Value()
		if err != nil {
			t.Errorf("Value(%s): %v", s, err)
			return
		}
		var aScan alpaca.Decimal
		if err := aScan.Scan(val); err != nil {
			t.Errorf("Scan(%s): %v (val=%v)", s, err, val)
			return
		}
		if !aScan.Equal(a) {
			t.Errorf("Value/Scan round-trip(%s): before=%s after=%s", s, a.String(), aScan.String())
		}
	})
}

// FuzzNullDecimal tests NullDecimal JSON, Text, and Value/Scan round-trips
// for both valid and null states.
func FuzzNullDecimal(f *testing.F) {
	f.Add("1.23", true)
	f.Add("-456.789", true)
	f.Add("0", true)
	f.Add("0", false)
	f.Add("9223372.5", true) // optimized boundary
	f.Add("9223373", true)   // fallback

	f.Fuzz(func(t *testing.T, s string, valid bool) {
		if !valid {
			// Null case: verify all serialization formats produce null semantics.
			nd := alpaca.NullDecimal{Valid: false}

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

			val, err := nd.Value()
			if err != nil {
				t.Errorf("NullDecimal.Value(null): %v", err)
			}
			if val != nil {
				t.Errorf("NullDecimal.Value(null): expected nil, got %v", val)
			}

			var nd3 alpaca.NullDecimal
			if err := nd3.Scan(nil); err != nil {
				t.Errorf("NullDecimal.Scan(nil): %v", err)
			}
			if nd3.Valid {
				t.Errorf("NullDecimal.Scan(nil): expected Valid=false")
			}

			// UnmarshalText with empty string should produce invalid.
			var nd4 alpaca.NullDecimal
			if err := nd4.UnmarshalText([]byte("")); err != nil {
				t.Errorf("NullDecimal.UnmarshalText(empty): %v", err)
			}
			if nd4.Valid {
				t.Errorf("NullDecimal.UnmarshalText(empty): expected Valid=false")
			}
			return
		}

		d, err := alpaca.NewFromString(s)
		if err != nil {
			return
		}
		if !inRange(d) {
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
			t.Errorf("NullDecimal JSON round-trip(%s): before=%s after=%s valid=%v",
				s, d.String(), nd2.Decimal.String(), nd2.Valid)
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
			t.Errorf("NullDecimal Text round-trip(%s): before=%s after=%s valid=%v",
				s, d.String(), nd3.Decimal.String(), nd3.Valid)
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
			t.Errorf("NullDecimal Value/Scan round-trip(%s): before=%s after=%s valid=%v",
				s, d.String(), nd4.Decimal.String(), nd4.Valid)
		}
	})
}

// FuzzScanTypes tests Decimal.Scan with various Go types: string, []byte,
// int64, and float64.
func FuzzScanTypes(f *testing.F) {
	f.Add("123.456")
	f.Add("-999.99")
	f.Add("0")
	f.Add("1")
	f.Add("9223372") // optimized boundary
	f.Add("9223373") // fallback

	f.Fuzz(func(t *testing.T, s string) {
		// Scan passes the raw string to the parser without trimming trailing zeros,
		// so inputs exceeding 19 fractional digits may be rejected even when
		// NewFromString (which trims first) would accept them.
		if fracDigitsOf(s) > maxPrec {
			return
		}

		expected, err := alpaca.NewFromString(s)
		if err != nil {
			return
		}
		if !inRange(expected) {
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

		// Scan from int64 (only for integer values whose IntPart fits in int64).
		// IntPart panics for values parsed into fallback representation with
		// leading zeros (e.g., "000000000000000000000000000000000000000010").
		if expected.IsInteger() {
			var ip int64
			if safeCall(func() { ip = expected.IntPart() }) {
				var d3 alpaca.Decimal
				if err := d3.Scan(ip); err != nil {
					t.Errorf("Scan(int64 %d): %v", ip, err)
					return
				}
				if d3.IntPart() != ip {
					t.Errorf("Scan(int64 %d): got IntPart=%d", ip, d3.IntPart())
				}
			}
		}

		// Scan from float64: float representation may lose precision, so we only
		// verify no error (not exact equality).
		f64 := expected.InexactFloat64()
		if !math.IsInf(f64, 0) && !math.IsNaN(f64) {
			var d4 alpaca.Decimal
			if err := d4.Scan(f64); err != nil {
				t.Errorf("Scan(float64 %v): %v", f64, err)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Randomized deterministic test
// ---------------------------------------------------------------------------

// TestRandomOperations is a deterministic randomized test that exercises the
// most common operations across 10,000 random input pairs. Unlike fuzz tests
// (which only run seed corpus in normal `go test`), this always runs the full
// 10,000 iterations and provides broad regression coverage.
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

		compare(t, fmt.Sprintf("[%d] Add(%s, %s)", i, aStr, bStr),
			a.Add(b), shopA.Add(shopB))
		compare(t, fmt.Sprintf("[%d] Sub(%s, %s)", i, aStr, bStr),
			a.Sub(b), shopA.Sub(shopB))
		compareTruncated(t, fmt.Sprintf("[%d] Mul(%s, %s)", i, aStr, bStr),
			a.Mul(b), shopA.Mul(shopB), 19)

		if !b.IsZero() {
			compare(t, fmt.Sprintf("[%d] Mod(%s, %s)", i, aStr, bStr),
				a.Mod(b), shopA.Mod(shopB))
		}

		places := int32(rng.Intn(15))

		compare(t, fmt.Sprintf("[%d] Round(%s, %d)", i, aStr, places),
			a.Round(places), shopA.Round(places))
		compare(t, fmt.Sprintf("[%d] RoundBank(%s, %d)", i, aStr, places),
			a.RoundBank(places), shopA.RoundBank(places))
		compare(t, fmt.Sprintf("[%d] Truncate(%s, %d)", i, aStr, places),
			a.Truncate(places), shopA.Truncate(places))
		compare(t, fmt.Sprintf("[%d] RoundCeil(%s, %d)", i, aStr, places),
			a.RoundCeil(places), shopA.RoundCeil(places))
		compare(t, fmt.Sprintf("[%d] RoundFloor(%s, %d)", i, aStr, places),
			a.RoundFloor(places), shopA.RoundFloor(places))
		compare(t, fmt.Sprintf("[%d] RoundUp(%s, %d)", i, aStr, places),
			a.RoundUp(places), shopA.RoundUp(places))
		compare(t, fmt.Sprintf("[%d] RoundDown(%s, %d)", i, aStr, places),
			a.RoundDown(places), shopA.RoundDown(places))

		if got, want := a.Sign(), shopA.Sign(); got != want {
			t.Errorf("[%d] Sign(%s): alpaca=%d shopspring=%d", i, aStr, got, want)
		}
		if len(aStr) <= 18 {
			if got, want := a.IntPart(), shopA.IntPart(); got != want {
				t.Errorf("[%d] IntPart(%s): alpaca=%d shopspring=%d", i, aStr, got, want)
			}
		}
	}
}
