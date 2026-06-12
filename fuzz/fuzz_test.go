// Package fuzz provides fuzz tests that validate alpacadecimal against
// shopspring/decimal as a reference implementation.
//
// alpacadecimal uses an optimized fixed-point representation for values where
// |integer part| <= 9,223,372 with up to 12 fractional digits. Larger values
// fall back to zerodecimal, which is bounded: a 128-bit coefficient with 0..19
// fractional digits and NO big.Int escape. Two boundaries shape the suite:
//
//   - 128-bit domain: a value is representable iff trunc(|v|) < 2^128
//     (~3.4e38). Parse/decode paths (NewFromString, UnmarshalBinary,
//     UnmarshalJSON, UnmarshalText, Scan) return an error containing
//     "exceeds the 128-bit decimal domain" for out-of-domain values, while
//     infallible paths (arithmetic, constructors) panic with domainPanicMsg.
//     The checkOp/domainGuard helpers assert an iff discipline: the
//     panic/error is accepted exactly when the shopspring reference value is
//     out of domain, and is a test failure otherwise.
//
//   - 19 fractional digits: values needing more are truncated by
//     alpacadecimal while shopspring keeps arbitrary precision, so tests skip
//     or truncate-compare in that regime. Additionally, when the coefficient
//     at the current precision exceeds 2^128, fractional digits are truncated
//     toward zero one at a time until it fits, so in-domain results can carry
//     FEWER than 19 fractional digits; domainAdjust models this exactly.
//
// maxAbsValue additionally caps most test magnitudes at 10^19. That cap is
// pragmatic — magnitudes up to 2^128 are representable — it simply bounds the
// region where both libraries are exercised together at full precision with
// reasonable runtime.
package fuzz

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"testing"

	zerodecimal "github.com/AlexandrosKyriakakis/zerodecimal"
	shopspring "github.com/shopspring/decimal"

	alpaca "github.com/alpacahq/alpacadecimal"
)

// ---------------------------------------------------------------------------
// Precision constraints
// ---------------------------------------------------------------------------

// maxPrec is the maximum number of fractional digits alpacadecimal represents.
// Values that need more are truncated (in the default parse/arithmetic mode)
// while shopspring preserves arbitrary precision.
const maxPrec = 19

// maxAbsValue is a pragmatic 10^19 cap on test inputs. It is not the
// representability limit (that is 2^128; see the package comment); it bounds
// the region where both libraries are compared at full precision while
// keeping fuzz throughput high.
var maxAbsValue = alpaca.RequireFromString("10000000000000000000")

// shopMaxAbs is maxAbsValue as a shopspring decimal.
var shopMaxAbs = shopspring.RequireFromString("10000000000000000000")

// boundarySeeds straddle the optimized/fallback boundary
// (|integer part| = 9,223,372 with up to 12 fractional digits).
var boundarySeeds = []string{
	"9223372", "-9223372",
	"9223372.000000000001", "-9223372.000000000001",
	"9223371.999999999999", "-9223371.999999999999",
	"9223373", "-9223373",
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// inRange returns true if d is within [-10^19, 10^19].
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

// comparableValue reports whether a shopspring value lies in the range where
// the two libraries are comparable at full precision: at most 19 canonical
// fractional digits (alpacadecimal truncates beyond that) and |v| <= 10^19
// (the pragmatic magnitude cap; see the package comment).
func comparableValue(sd shopspring.Decimal) bool {
	return fracDigitsOf(sd.String()) <= maxPrec && sd.Abs().Cmp(shopMaxAbs) <= 0
}

// sciExpOutOfBounds reports whether s uses scientific notation with an
// exponent magnitude beyond what the harness can afford to materialize.
// shopspring stores huge exponents lazily, while alpacadecimal expands the
// coefficient eagerly at parse time (and rejects |exp| > 10000 by design as a
// DoS guard, where shopspring would accept). The harness bound of |exp| < 100
// keeps fuzzing fast and stays below that intentional divergence.
//
// The exponent digits are scanned manually (instead of strconv.ParseInt on
// the whole tail) so that surrounding junk the callers strip before parsing
// — JSON whitespace, quotes — cannot defeat the bound (e.g. "9e8000400 ").
func sciExpOutOfBounds(s string) bool {
	idx := strings.IndexAny(s, "eE")
	if idx < 0 {
		return false
	}
	tail := s[idx+1:]
	i := 0
	if i < len(tail) && (tail[i] == '+' || tail[i] == '-') {
		i++
	}
	start := i
	for i < len(tail) && tail[i] >= '0' && tail[i] <= '9' {
		i++
	}
	digits := strings.TrimLeft(tail[start:i], "0")
	// More than two significant digits means |exp| >= 100: too expensive.
	return len(digits) > 2
}

// knownParseDivergence reports inputs hit by the single remaining intentional
// alpacadecimal-vs-shopspring parser divergence.
//
// shopspring joins the substrings around the dot before parsing with
// big.Int.SetString, so a sign directly after a leading dot is accepted with
// a surprising value (".-5" == -0.05, ".+0" == 0). alpacadecimal rejects
// these on purpose: replicating the quirk would silently turn malformed
// input into sign-shifted values.
func knownParseDivergence(s string) bool {
	if len(s) > 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1] // Scan and UnmarshalJSON unquote before parsing
	}
	// sign after a leading dot (also inside a scientific mantissa)
	if idx := strings.IndexAny(s, "eE"); idx >= 0 {
		s = s[:idx]
	}
	return len(s) >= 2 && s[0] == '.' && (s[1] == '+' || s[1] == '-')
}

// compare asserts that alpacadecimal and shopspring produce exactly the same
// String() output. Both libraries trim trailing zeros in String(), so equal
// values must render identically — no normalization is applied.
func compare(t *testing.T, op string, alpacaResult alpaca.Decimal, shopResult shopspring.Decimal) {
	t.Helper()
	if a, s := alpacaResult.String(), shopResult.String(); a != s {
		t.Errorf("%s: alpaca=%q shopspring=%q", op, a, s)
	}
}

// ---------------------------------------------------------------------------
// 128-bit domain modeling
// ---------------------------------------------------------------------------

// twoPow128 is the first integer magnitude alpacadecimal cannot represent:
// zerodecimal coefficients are 128-bit, so trunc(|v|) must stay below 2^128.
var twoPow128 = shopspring.NewFromBigInt(new(big.Int).Lsh(big.NewInt(1), 128), 0)

// domainPanicMsg is the exact panic value used by infallible alpacadecimal
// paths for out-of-domain values. NewFromFloat wraps it with a prefix, so its
// panic merely contains the marker substring; see domainGuard.
const domainPanicMsg = "alpacadecimal: value exceeds the 128-bit decimal domain (|value| >= 2^128)"

// isDomainError reports whether err is the parse/decode-path counterpart of
// the domain panic.
func isDomainError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "exceeds the 128-bit decimal domain")
}

// outOfDomain is the exact predicate for "alpacadecimal must reject (parse
// paths) or panic (infallible paths)": trunc(|sd|) >= 2^128. Only the integer
// part matters — excess fractional digits are truncated, never rejected.
//
// The digit-count fast paths are pure optimization/hardening: 2^128 has 39
// decimal digits, so any 40+-digit integer part is out and any <=38-digit one
// is in. They keep decoded garbage with astronomical exponents (which
// shopspring stores lazily but Cmp/Truncate would materialize eagerly) from
// allocating gigantic big.Ints.
func outOfDomain(sd shopspring.Decimal) bool {
	if sd.IsZero() {
		return false
	}
	// 10^(digits+exp-1) <= |sd| < 10^(digits+exp)
	intDigits := int64(sd.NumDigits()) + int64(sd.Exponent())
	if intDigits >= 40 {
		return true // |sd| >= 10^39 > 2^128
	}
	if intDigits <= 38 {
		return false // |sd| < 10^38 < 2^128
	}
	return sd.Abs().Truncate(0).Cmp(twoPow128) >= 0
}

// domainAdjust models alpacadecimal's bounded representation of sd: ok is
// false when the value is out of the 128-bit domain entirely (alpacadecimal
// rejects or panics); otherwise fractional digits are truncated toward zero
// one at a time until the coefficient fits 128 bits, mirroring
// decimalFromBigParts/zdFromBigOK in the main package. Callers must apply the
// 19-fractional-digit truncation convention BEFORE calling domainAdjust, so
// that the returned value's digits are exactly the digits alpacadecimal
// stores.
//
// shopspring representations can carry positive exponents (small coefficient,
// large exponent) and Truncate(0) does NOT materialize the digits in that
// case (verified: New(5, 40).Truncate(0) keeps coef=5/exp=40), so such values
// are rebuilt via NewFromBigInt instead. They are integers, so once they pass
// the outOfDomain gate the materialized coefficient trunc(|v|) already fits
// 128 bits and no truncation loop is needed.
func domainAdjust(sd shopspring.Decimal) (shopspring.Decimal, bool) {
	if outOfDomain(sd) {
		return sd, false
	}
	if sd.Exponent() > 0 {
		// Integer-valued: materialize the digits (value-preserving; BigInt()
		// is exact for integers).
		return shopspring.NewFromBigInt(sd.BigInt(), 0), true
	}
	for new(big.Int).Abs(sd.Coefficient()).BitLen() > 128 {
		frac := -sd.Exponent()
		if frac <= 0 {
			break // unreachable for in-domain values; defensive
		}
		sd = sd.Truncate(frac - 1)
	}
	return sd, true
}

// domainGuard runs fn and recovers ONLY the documented domain panic: the
// exact domainPanicMsg string, or a string containing the marker substring
// (NewFromFloat wraps the message). Any other panic is re-raised, preserving
// the suite-wide discipline that unexpected panics crash the fuzz target.
func domainGuard(fn func()) (panicked bool) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if msg, ok := r.(string); ok &&
			(msg == domainPanicMsg || strings.Contains(msg, "exceeds the 128-bit decimal domain")) {
			panicked = true
			return
		}
		panic(r)
	}()
	fn()
	return false
}

// compareAdjusted compares an already-computed alpacadecimal result against
// the domain-adjusted shopspring expectation. The caller must already have
// applied the 19-fractional-digit truncation convention to shopResult where
// the operation requires it. It fails when the expectation is out of domain —
// alpacadecimal should have panicked instead of producing a result.
func compareAdjusted(t *testing.T, op string, alpacaResult alpaca.Decimal, shopResult shopspring.Decimal) {
	t.Helper()
	adjusted, ok := domainAdjust(shopResult)
	if !ok {
		t.Errorf("%s: alpaca=%q but expected a domain panic (shopspring=%q is out of domain)",
			op, alpacaResult.String(), shopResult.String())
		return
	}
	compare(t, op, alpacaResult, adjusted)
}

// checkOpAt implements checkOp and checkOpTruncated: it runs alpacaFn under
// domainGuard and asserts the iff discipline. places < 0 means "compare
// exactly"; places >= 0 truncates both sides first (the old compareTruncated
// convention, for operations whose exact result may exceed 19 fractional
// digits).
func checkOpAt(t *testing.T, op string, alpacaFn func() alpaca.Decimal, shopResult shopspring.Decimal, places int32) {
	t.Helper()
	var aResult alpaca.Decimal
	if domainGuard(func() { aResult = alpacaFn() }) {
		// A domain panic is correct iff the exact expected value is out of
		// domain. Fractional truncation never changes the integer part, so
		// the untruncated shopResult is the right predicate input for
		// truncated operations too.
		if !outOfDomain(shopResult) {
			t.Errorf("%s: spurious domain panic (shopspring=%q is in domain)", op, shopResult.String())
		}
		return
	}
	expected := shopResult
	if places >= 0 {
		aResult = aResult.Truncate(places)
		expected = expected.Truncate(places)
	}
	compareAdjusted(t, op, aResult, expected)
}

// checkOp wraps an alpacadecimal operation that returns a Decimal and is
// compared exactly (the compare convention): a domain panic is accepted iff
// the shopspring result is out of the 128-bit domain, and otherwise the
// result must match the domain-adjusted shopspring expectation exactly.
func checkOp(t *testing.T, op string, alpacaFn func() alpaca.Decimal, shopResult shopspring.Decimal) {
	t.Helper()
	checkOpAt(t, op, alpacaFn, shopResult, -1)
}

// checkOpTruncated is checkOp for operations whose exact result may exceed 19
// fractional digits (e.g. multiplication): both sides are truncated to places
// before the domain adjustment and comparison.
func checkOpTruncated(t *testing.T, op string, alpacaFn func() alpaca.Decimal, shopResult shopspring.Decimal, places int32) {
	t.Helper()
	checkOpAt(t, op, alpacaFn, shopResult, places)
}

// checkOp2 is the domainGuard front half of checkOp for operations returning
// two Decimals (QuoRem). It reports whether the call completed without a
// domain panic; the caller compares the returned values (via compareAdjusted)
// and runs its property checks only in that case.
func checkOp2(t *testing.T, op string, alpacaFn func() (alpaca.Decimal, alpaca.Decimal), shopFirst, shopSecond shopspring.Decimal) (alpaca.Decimal, alpaca.Decimal, bool) {
	t.Helper()
	var r1, r2 alpaca.Decimal
	if domainGuard(func() { r1, r2 = alpacaFn() }) {
		if !outOfDomain(shopFirst) && !outOfDomain(shopSecond) {
			t.Errorf("%s: spurious domain panic (shopspring=(%q, %q) is in domain)",
				op, shopFirst.String(), shopSecond.String())
		}
		return alpaca.Decimal{}, alpaca.Decimal{}, false
	}
	return r1, r2, true
}

// checkOpString is checkOp for operations returning a string (the StringFixed
// family). shopOperand is the value being formatted: formatting at
// non-negative places never pushes the integer part past the operand's
// magnitude by more than one, so a domain panic is correct iff the operand
// itself is out of domain — which parseBoth-gated call sites can never
// produce, making the guard documentation of the iff rule rather than an
// expected occurrence.
func checkOpString(t *testing.T, op string, alpacaFn func() string, want string, shopOperand shopspring.Decimal) {
	t.Helper()
	var got string
	if domainGuard(func() { got = alpacaFn() }) {
		if !outOfDomain(shopOperand) {
			t.Errorf("%s: spurious domain panic (shopspring operand %q is in domain)", op, shopOperand.String())
		}
		return
	}
	if got != want {
		t.Errorf("%s: alpaca=%q shopspring=%q", op, got, want)
	}
}

// assertPanics fails the test when fn does not panic. It is used only for
// operations where shopspring/decimal itself panics (e.g. division by zero),
// so alpacadecimal must panic too. Everywhere else panics are NOT recovered:
// if alpacadecimal panics where shopspring does not, the fuzz target crashes
// and reports the input.
func assertPanics(t *testing.T, op string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: expected panic, got none", op)
		}
	}()
	fn()
}

// parseBoth parses s with both libraries for operation-level fuzz targets.
// It returns ok=false for inputs either library rejects (error parity itself
// is asserted by FuzzNewFromString) or that fall outside the comparable
// range. When both parses succeed the parsed values are asserted equal so
// downstream operation mismatches are not confused with parse mismatches.
func parseBoth(t *testing.T, s string) (alpaca.Decimal, shopspring.Decimal, bool) {
	t.Helper()
	// knownParseDivergence inputs can parse to DIFFERENT values (e.g. the
	// case-3 exponent loss), which would surface as bogus operation failures.
	if len(s) > 100 || sciExpOutOfBounds(s) || knownParseDivergence(s) {
		return alpaca.Decimal{}, shopspring.Decimal{}, false
	}
	a, errA := alpaca.NewFromString(s)
	sd, errS := shopspring.NewFromString(s)
	if errA != nil || errS != nil {
		return alpaca.Decimal{}, shopspring.Decimal{}, false
	}
	if !comparableValue(sd) || !inRange(a) {
		return alpaca.Decimal{}, shopspring.Decimal{}, false
	}
	compare(t, fmt.Sprintf("parse(%q)", s), a, sd)
	return a, sd, true
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
// shopspring never panics here, but value*10^exp can leave the 128-bit
// domain (e.g. 9e18 * 10^30), where alpacadecimal panics by contract — so
// both calls go through checkOp, which accepts the domain panic iff the
// value is out of domain.
func FuzzNew(f *testing.F) {
	for _, v := range []int64{0, 1, -1, 100, -100, 123, -456, 999999999, 9223372, -9223372, 9223373} {
		for _, e := range []int32{-19, -12, -5, -2, 0, 2, 5, 20} {
			f.Add(v, e)
		}
	}

	f.Fuzz(func(t *testing.T, value int64, exp int32) {
		// exp < -19 enters the >19-fractional-digit truncation regime where
		// alpacadecimal legitimately diverges; exp > 30 only inflates runtime.
		if exp < -19 || exp > 30 {
			return
		}

		sNew := shopspring.New(value, exp)
		checkOp(t, fmt.Sprintf("New(%d, %d)", value, exp),
			func() alpaca.Decimal { return alpaca.New(value, exp) }, sNew)

		bi := big.NewInt(value)
		sBig := shopspring.NewFromBigInt(bi, exp)
		checkOp(t, fmt.Sprintf("NewFromBigInt(%d, %d)", value, exp),
			func() alpaca.Decimal { return alpaca.NewFromBigInt(bi, exp) }, sBig)
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
	f.Add(int64(9223371))  // just below the boundary
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
// shopspring panics only on NaN/Inf, which are excluded, so the alpacadecimal
// call is unguarded: a panic fails the target.
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
	f.Add(9223372.5) // optimized boundary with fraction
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

		a := alpaca.NewFromFloat(v)
		s := shopspring.NewFromFloat(v)

		// Skip values whose minimal float representation exceeds 19 fractional
		// digits, since alpacadecimal truncates there while shopspring keeps
		// every digit.
		str := strconv.FormatFloat(v, 'f', -1, 64)
		if dotIdx := strings.IndexByte(str, '.'); dotIdx >= 0 && len(str)-dotIdx-1 > maxPrec {
			return
		}
		compare(t, fmt.Sprintf("NewFromFloat(%v)", v), a, s)
	})
}

// FuzzNewFromFloat32 tests NewFromFloat32 with float32 inputs.
// As with NewFromFloat, NaN/Inf are the only shopspring panics, so the
// alpacadecimal call is unguarded.
func FuzzNewFromFloat32(f *testing.F) {
	f.Add(float32(0.0))
	f.Add(float32(1.0))
	f.Add(float32(-1.0))
	f.Add(float32(0.1))
	f.Add(float32(-0.1))
	f.Add(float32(123.456))
	f.Add(float32(-123.456))
	f.Add(float32(9223372.0))
	f.Add(float32(9223373.0))
	f.Add(float32(-0.00024414063)) // known shopspring/strconv divergence

	f.Fuzz(func(t *testing.T, v float32) {
		if v != v || math.IsInf(float64(v), 0) { // NaN or Inf
			return
		}
		if v > 1e10 || v < -1e10 {
			return
		}

		// Known divergence: shopspring's float32 shortest-digit algorithm
		// occasionally differs from strconv's shortest round-trip
		// representation by one ulp in the last digit (e.g. -0.00024414063
		// becomes -0.00024414062 — the exact binary value truncated rather
		// than rounded). alpacadecimal follows strconv. Skip inputs where
		// shopspring itself disagrees with strconv.
		// TODO: decide whether alpacadecimal should mirror shopspring here.
		shortest := strconv.FormatFloat(float64(v), 'f', -1, 32)
		sD := shopspring.NewFromFloat32(v)
		if sD.String() != shortest {
			t.Skip("known divergence: shopspring NewFromFloat32 deviates from strconv shortest representation")
		}

		a := alpaca.NewFromFloat32(v)
		// shopspring may produce more fractional digits than the 19-digit
		// limit; truncate shopspring's result to match.
		compare(t, fmt.Sprintf("NewFromFloat32(%v)", v), a, sD.Truncate(maxPrec))
	})
}

// FuzzNewFromFloatWithExponent verifies that NewFromFloatWithExponent matches
// shopspring exactly (binary expansion of the float, rounded at 10^exp) for
// exponents in [-15, 10].
func FuzzNewFromFloatWithExponent(f *testing.F) {
	f.Add(123.456, int32(-2))
	f.Add(-123.456, int32(-2))
	f.Add(0.0, int32(0))
	f.Add(1.0, int32(-5))
	f.Add(99.995, int32(-2))
	f.Add(0.1, int32(-15))
	f.Add(9223372.5, int32(-1)) // optimized boundary
	f.Add(9223371.5, int32(-1)) // just below the boundary
	f.Add(9223373.5, int32(-1)) // fallback
	f.Add(12345.0, int32(3))    // positive exponent
	f.Add(12345.0, int32(10))   // max positive exponent

	f.Fuzz(func(t *testing.T, v float64, exp int32) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return
		}
		if v > 1e12 || v < -1e12 {
			return
		}
		// below -15, results gain >19 fractional digits where the package
		// truncation convention legitimately diverges from shopspring
		if exp < -15 || exp > 10 {
			return
		}

		a := alpaca.NewFromFloatWithExponent(v, exp)
		s := shopspring.NewFromFloatWithExponent(v, exp)
		compare(t, fmt.Sprintf("NewFromFloatWithExponent(%v, %d)", v, exp), a, s)
	})
}

// FuzzNewFromString tests NewFromString and RequireFromString for error
// parity, panic agreement, round-trip stability, and value equality.
//
// ERROR PARITY: alpacadecimal must return an error exactly when shopspring
// does, except for two documented limits:
//   - 128-bit domain: values whose integer part reaches 2^128 MUST be
//     rejected with the domain error even though shopspring accepts them
//     (and accepting them, or raising the domain error for an in-domain
//     value, is a failure);
//   - representational truncation: values needing more than 19 fractional
//     digits or |v| > 10^19 may truncate (default parse mode) yet still
//     succeed.
func FuzzNewFromString(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "0.5", "-0.5",
		"123.456", "-123.456",
		"0.000000001", "0.000000000001",
		"999999999.999999999",
		"0.1234567890123456789",
		// scientific notation
		"1e5", "1.23e-4", "-1.5E+3", "12e2", "1e0", "5.e3", "1e-25",
		// quoted strings (rejected by both libraries)
		`"1.5"`, `"abc"`,
		// dot edge cases ("5." and ".5" parse; "." and ".." do not)
		".", "..", "5.", ".5", "-.5", "+.5", "-.", "1.2.3",
		// known divergences (see knownParseDivergence): alpacadecimal
		// wrongly accepts these...
		"0..", "1.2.", ".5.", "5..",
		"0.0000000000000000000A", "1.2345678901234567890XYZ",
		"1.0000000000000000000e5", // parsed as 1: exponent silently dropped
		"-+1000000000000000000000000000000000000000",
		"--0000000000000000000000000000000000000000",
		// ...and shopspring quirkily accepts these
		".-5", ".+0", ".-5e2",
		// malformed
		"", "-", "+", "1e", "e5", "abc", " 1", "1 ",
		// the 128-bit domain boundary: 2^128-1 parses, 2^128 is rejected
		"340282366920938463463374607431768211455",
		"-340282366920938463463374607431768211455",
		"340282366920938463463374607431768211456",
		"-340282366920938463463374607431768211456",
		"340282366920938463463374607431768211455.99",
		"340282366920938463463374607431768211456.01",
		// long inputs
		strings.Repeat("9", 250),
		"1." + strings.Repeat("3", 249),
		"-" + strings.Repeat("7", 120) + "." + strings.Repeat("1", 130),
	}
	seeds = append(seeds, boundarySeeds...)
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// Bound runtime: gigantic exponents would make alpacadecimal eagerly
		// materialize the coefficient; absurdly long inputs slow the run.
		if len(s) > 500 || sciExpOutOfBounds(s) {
			return
		}
		if knownParseDivergence(s) {
			t.Skip("known parser divergence (see knownParseDivergence)")
		}

		shopD, shopErr := shopspring.NewFromString(s)
		d, err := alpaca.NewFromString(s)

		// 128-bit domain rules (iff): out-of-domain values must be rejected
		// with the domain error; in-domain values must never raise it.
		if shopErr == nil {
			if outOfDomain(shopD) {
				if err == nil {
					t.Errorf("NewFromString(%q) accepted out-of-domain value: alpaca=%s shopspring=%s",
						s, d.String(), shopD.String())
					return
				}
				if !isDomainError(err) {
					t.Errorf("NewFromString(%q): expected domain error for out-of-domain value %s, got: %v",
						s, shopD.String(), err)
				}
			} else if isDomainError(err) {
				t.Errorf("NewFromString(%q): spurious domain error for in-domain value %s: %v",
					s, shopD.String(), err)
				return
			}
		}

		// Documented representational exemption (see the fuzz doc comment).
		exempt := shopErr == nil && !comparableValue(shopD)

		if (err != nil) != (shopErr != nil) && !exempt {
			t.Errorf("NewFromString(%q) error parity: alpaca=%v shopspring=%v", s, err, shopErr)
			return
		}

		if err != nil {
			// RequireFromString must panic for inputs that NewFromString rejects.
			assertPanics(t, fmt.Sprintf("RequireFromString(%q)", s), func() {
				alpaca.RequireFromString(s)
			})
			return
		}

		// RequireFromString must return the same value without panicking.
		r := alpaca.RequireFromString(s)
		if !r.Equal(d) {
			t.Errorf("RequireFromString(%q) = %s, NewFromString = %s", s, r.String(), d.String())
		}

		// Round-trip: String() -> NewFromString must preserve the value.
		rt, rtErr := alpaca.NewFromString(d.String())
		if rtErr != nil {
			t.Errorf("String round-trip(%q): parse error: %v", s, rtErr)
		} else if !rt.Equal(d) {
			t.Errorf("String round-trip(%q): %q -> %q", s, d.String(), rt.String())
		}

		// Cross-library value comparison within the comparable range.
		if shopErr != nil || exempt {
			return
		}
		compare(t, fmt.Sprintf("NewFromString(%q)", s), d, shopD)
	})
}

// FuzzNewFromFormattedString tests parsing strings with embedded separators.
func FuzzNewFromFormattedString(f *testing.F) {
	f.Add("1,234.56", ",")
	f.Add("1 234.56", " ")
	f.Add("-1,000,000.99", ",")
	f.Add("1_000.50", "_")
	f.Add("9,223,373.5", ",")

	f.Fuzz(func(t *testing.T, s, sep string) {
		if len(s) > 100 || len(sep) != 1 {
			return
		}
		c := sep[0]
		if (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '+' {
			return
		}

		cleaned := strings.NewReplacer(sep, "").Replace(s)
		if sciExpOutOfBounds(cleaned) {
			return
		}
		expected, err := alpaca.NewFromString(cleaned)
		if err != nil {
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
	seeds := append([]string{
		"0", "1", "-1", "0.5", "-0.5",
		"123.456", "-123.456",
		"0.000000001", "-0.000000001",
		"0.000000000001",            // 12 fractional digits: optimized boundary
		"9999999.999999999",         // large fractional
		"1000000000", "-1000000000", // large integer, fallback
	}, boundarySeeds...)
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b)
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string) {
		a, shopA, ok := parseBoth(t, aStr)
		if !ok {
			return
		}
		b, shopB, ok := parseBoth(t, bStr)
		if !ok {
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

		checkOp(t, fmt.Sprintf("Add(%s, %s)", aStr, bStr),
			func() alpaca.Decimal { return a.Add(b) }, shopA.Add(shopB))
		checkOp(t, fmt.Sprintf("Sub(%s, %s)", aStr, bStr),
			func() alpaca.Decimal { return a.Sub(b) }, shopA.Sub(shopB))

		// Mul: the exact product may need up to 38 fractional digits;
		// alpacadecimal truncates at 19 while shopspring keeps all of them
		// (and may truncate further so the coefficient fits 128 bits —
		// checkOpTruncated's domain adjustment models that).
		checkOpTruncated(t, fmt.Sprintf("Mul(%s, %s)", aStr, bStr),
			func() alpaca.Decimal { return a.Mul(b) }, shopA.Mul(shopB), maxPrec)

		if b.IsZero() {
			// Panic parity: shopspring panics on division by zero, so
			// alpacadecimal must too.
			assertPanics(t, fmt.Sprintf("Div(%s, 0)", aStr), func() { a.Div(b) })
			assertPanics(t, fmt.Sprintf("Mod(%s, 0)", aStr), func() { a.Mod(b) })
		} else {
			// Both libraries round the quotient at DivisionPrecision=16, so
			// the results must match exactly (after domain adjustment when
			// the 16-fractional-digit coefficient exceeds 128 bits).
			checkOp(t, fmt.Sprintf("Div(%s, %s)", aStr, bStr),
				func() alpaca.Decimal { return a.Div(b) }, shopA.Div(shopB))
			checkOp(t, fmt.Sprintf("Mod(%s, %s)", aStr, bStr),
				func() alpaca.Decimal { return a.Mod(b) }, shopA.Mod(shopB))
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

// FuzzDivRound tests DivRound with explicit precision in [-5, 19].
// The result always has at most |precision| <= 19 fractional digits, so it is
// representable and must match shopspring exactly for every precision.
func FuzzDivRound(f *testing.F) {
	seeds := append([]string{
		"0", "1", "-1", "3", "7", "10",
		"123.456", "-123.456",
		"0.000000001",
	}, boundarySeeds...)
	precs := []int8{-5, -2, 0, 1, 4, 16, 19}
	for _, a := range seeds {
		for _, b := range seeds {
			for _, p := range precs {
				f.Add(a, b, p)
			}
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string, prec int8) {
		if prec < -5 || prec > 19 {
			return
		}
		p := int32(prec)

		a, shopA, ok := parseBoth(t, aStr)
		if !ok {
			return
		}
		b, shopB, ok := parseBoth(t, bStr)
		if !ok {
			return
		}
		if b.IsZero() {
			// Panic parity with shopspring.
			assertPanics(t, fmt.Sprintf("DivRound(%s, 0, %d)", aStr, p), func() { a.DivRound(b, p) })
			return
		}

		sResult := shopA.DivRound(shopB, p)
		checkOp(t, fmt.Sprintf("DivRound(%s, %s, %d)", aStr, bStr, p),
			func() alpaca.Decimal { return a.DivRound(b, p) }, sResult)
	})
}

// FuzzQuoRem tests QuoRem (division with remainder) with precision in [-5, 19].
//
// Validated properties:
//  1. Cross-library: quotient and remainder match shopspring exactly whenever
//     the remainder is representable within 19 fractional digits. The quotient
//     has at most precision <= 19 fractional digits, so it must match even
//     when the remainder is out of range.
//  2. Invariant: d2*q + r == d, verified EXACTLY (via shopspring big.Int
//     arithmetic on alpacadecimal's outputs).
//  3. Remainder bound: |r| * 10^prec < |d2|.
//  4. Sign: a nonzero remainder has the sign of the dividend.
func FuzzQuoRem(f *testing.F) {
	seeds := append([]string{
		"0", "1", "-1", "0.5", "-0.5",
		"3", "-3", "7", "-7", "10", "-10",
		"123.456", "-123.456",
		"0.000000001", "-0.000000001",
		"0.000000000001",            // 12 fractional digits: optimized boundary
		"1000000000", "-1000000000", // large fallback
		"0.1", "0.01", "0.001",
		"999999.999999",
		"0.1234567890123456789", // 19 fractional digits
	}, boundarySeeds...)
	precs := []int8{-5, -2, 0, 1, 2, 4, 8, 12, 16, 19}
	for _, a := range seeds {
		for _, b := range seeds {
			for _, p := range precs {
				f.Add(a, b, p)
			}
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string, prec int8) {
		if prec < -5 || prec > 19 {
			return
		}
		p := int32(prec)

		a, shopA, ok := parseBoth(t, aStr)
		if !ok {
			return
		}
		b, shopB, ok := parseBoth(t, bStr)
		if !ok {
			return
		}
		if b.IsZero() {
			// Panic parity with shopspring.
			assertPanics(t, fmt.Sprintf("QuoRem(%s, 0, %d)", aStr, p), func() { a.QuoRem(b, p) })
			return
		}

		sQ, sR := shopA.QuoRem(shopB, p)
		aQ, aR, ok := checkOp2(t, fmt.Sprintf("QuoRem(%s, %s, %d)", aStr, bStr, p),
			func() (alpaca.Decimal, alpaca.Decimal) { return a.QuoRem(b, p) }, sQ, sR)
		if !ok {
			return
		}

		// Canonical fractional digits of the operands (trailing zeros trimmed).
		maxFrac := fracDigitsOf(shopA.String())
		if f2 := fracDigitsOf(shopB.String()); f2 > maxFrac {
			maxFrac = f2
		}

		if maxFrac+int(p) > maxPrec {
			// The exact remainder may need more than 19 fractional digits,
			// where alpacadecimal truncates. The quotient has at most p
			// fractional digits and must match after domain adjustment (its
			// coefficient can exceed 128 bits, e.g. a 10^38-magnitude
			// quotient at precision 19).
			compareAdjusted(t, fmt.Sprintf("QuoRem_q(%s, %s, %d) [r beyond 19 digits]", aStr, bStr, p), aQ, sQ)
			return
		}

		// Property 1: cross-library agreement, exact. In this regime
		// (operand magnitudes <= 10^19, maxFrac+p <= 19) both q and r have
		// coefficients below 2^128, so domain adjustment is an identity and
		// the comparison stays exact.
		compareAdjusted(t, fmt.Sprintf("QuoRem_q(%s, %s, %d)", aStr, bStr, p), aQ, sQ)
		compareAdjusted(t, fmt.Sprintf("QuoRem_r(%s, %s, %d)", aStr, bStr, p), aR, sR)

		// Re-parse alpacadecimal's outputs into shopspring for exact (big.Int)
		// arithmetic in the property checks below.
		qRef, errQ := shopspring.NewFromString(aQ.String())
		rRef, errR := shopspring.NewFromString(aR.String())
		if errQ != nil || errR != nil {
			t.Errorf("QuoRem(%s, %s, %d): outputs unparseable: q=%q (%v) r=%q (%v)",
				aStr, bStr, p, aQ.String(), errQ, aR.String(), errR)
			return
		}

		// Property 2: exact quotient-remainder identity d2*q + r == d.
		if recon := qRef.Mul(shopB).Add(rRef); !recon.Equal(shopA) {
			t.Errorf("QuoRem invariant d2*q+r==d failed for (%s, %s, %d): q=%s r=%s reconstructed=%s original=%s",
				aStr, bStr, p, aQ.String(), aR.String(), recon.String(), shopA.String())
		}

		// Property 3: |r| < |d2| * 10^(-prec), i.e. |r| * 10^prec < |d2|.
		if rRef.Abs().Shift(p).Cmp(shopB.Abs()) >= 0 {
			t.Errorf("QuoRem remainder bound violated for (%s, %s, %d): |r|=%s |d2|=%s",
				aStr, bStr, p, rRef.Abs().String(), shopB.Abs().String())
		}

		// Property 4: remainder sign matches dividend sign (truncated division).
		if !rRef.IsZero() && rRef.Sign() != shopA.Sign() {
			t.Errorf("QuoRem sign mismatch for (%s, %s, %d): dividend sign %d, remainder %s",
				aStr, bStr, p, shopA.Sign(), aR.String())
		}
	})
}

// FuzzPow tests Pow against shopspring for integer exponents (positive: exact
// exponentiation; negative: rounded at PowPrecisionNegativeExponent — both
// must match shopspring exactly when the result fits 19 fractional digits)
// and fractional exponents (alpacadecimal delegates to shopspring, so results
// match exactly within 19 fractional digits and truncate-compare beyond).
// Exponent magnitude is clamped to [-10, 10] to bound runtime.
func FuzzPow(f *testing.F) {
	bases := []string{
		"0", "1", "-1", "2", "-2", "1.5", "-1.5", "10", "0.5", "3", "0.001",
		"9223372", "9223372.000000000001", "9223371.999999999999", "9223373",
	}
	exps := []string{"0", "1", "2", "3", "4", "5", "10", "-1", "-2", "-5", "-10", "0.5", "-0.5", "1.5", "2.25"}
	for _, base := range bases {
		for _, exp := range exps {
			f.Add(base, exp)
		}
	}

	f.Fuzz(func(t *testing.T, baseStr, expStr string) {
		base, shopBase, ok := parseBoth(t, baseStr)
		if !ok {
			return
		}
		exp, shopExp, ok := parseBoth(t, expStr)
		if !ok {
			return
		}
		// Clamp exponent magnitude to keep runtime bounded.
		if exp.Abs().GreaterThan(alpaca.NewFromInt(10)) {
			return
		}

		// shopspring's fractional Pow derives its working precision from the
		// operand representation ("2.50" vs "2.5" can differ in low-order
		// digits) and alpacadecimal always computes on the canonical trimmed
		// form, so canonicalize the reference operands the same way.
		shopBase = shopspring.RequireFromString(shopBase.String())
		shopExp = shopspring.RequireFromString(shopExp.String())

		// shopspring.Pow never panics for these inputs (0^0, 0^negative and
		// negative^fractional all return 0), but the result can leave the
		// 128-bit domain (e.g. (10^19)^10), where alpacadecimal panics by
		// contract. checkOpTruncated accepts the domain panic iff the exact
		// result is out of domain; in-domain results are compared after the
		// 19-fractional-digit truncation plus domain adjustment (results
		// with <=19 fractional digits are untouched by the truncation, so
		// the comparison stays exact there).
		sResult := shopBase.Pow(shopExp)
		checkOpTruncated(t, fmt.Sprintf("Pow(%s, %s)", baseStr, expStr),
			func() alpaca.Decimal { return base.Pow(exp) }, sResult, maxPrec)
	})
}

// FuzzShift tests Shift (multiplication by 10^shift).
func FuzzShift(f *testing.F) {
	seeds := append([]string{"0", "1", "-1", "123.456", "-123.456", "0.001"}, boundarySeeds...)
	for _, s := range seeds {
		for shift := int32(-5); shift <= 5; shift++ {
			f.Add(s, shift)
		}
	}

	f.Fuzz(func(t *testing.T, s string, shift int32) {
		if shift < -8 || shift > 8 {
			return
		}

		a, shopD, ok := parseBoth(t, s)
		if !ok {
			return
		}

		// A negative shift increases fractional digits. Skip if the result
		// would exceed the 19-digit precision limit (canonical digit count).
		if shift < 0 && fracDigitsOf(shopD.String())+int(-shift) > maxPrec {
			return
		}

		checkOp(t, fmt.Sprintf("Shift(%s, %d)", s, shift),
			func() alpaca.Decimal { return a.Shift(shift) }, shopD.Shift(shift))
	})
}

// ---------------------------------------------------------------------------
// Rounding tests
// ---------------------------------------------------------------------------

// FuzzRounding tests all rounding methods: Round (half away from zero),
// RoundBank (half to even), RoundCeil, RoundFloor, RoundUp, RoundDown,
// and Truncate, with places in [-30, 30].
//
// Rounding never increases the number of fractional digits, so no input needs
// to be skipped: for places > 19 both libraries return the value unchanged
// (alpacadecimal literally, shopspring after trailing-zero trimming in
// String), and Truncate with negative places is a no-op in both.
func FuzzRounding(f *testing.F) {
	seeds := append([]string{
		"0", "1.5", "-1.5", "2.5", "-2.5",
		"1.45", "1.55", "123.456", "-123.456",
		"0.999", "-0.999", "1.005", "99.995",
		"9223372.5", "-9223372.5", // optimized with rounding
		"1000000000", "-1000000000", // large fallback
		"9999999999999999999", "-9999999999999999999",
		"0.1234567890123456789", "-0.1234567890123456789",
	}, boundarySeeds...)
	for _, s := range seeds {
		for places := int8(-30); places <= 30; places += 3 {
			f.Add(s, places)
		}
	}

	f.Fuzz(func(t *testing.T, s string, places int8) {
		if places < -30 || places > 30 {
			return
		}
		p := int32(places)

		a, shopD, ok := parseBoth(t, s)
		if !ok {
			return
		}

		// Rounding at negative places is an infallible path that panics for
		// out-of-domain results, so the calls go through checkOp (inputs are
		// capped at 10^19, so rounding at places >= -30 stays in domain and
		// the guard documents the iff rule rather than an expected panic).
		checkOp(t, fmt.Sprintf("Round(%s, %d)", s, p),
			func() alpaca.Decimal { return a.Round(p) }, shopD.Round(p))
		checkOp(t, fmt.Sprintf("RoundBank(%s, %d)", s, p),
			func() alpaca.Decimal { return a.RoundBank(p) }, shopD.RoundBank(p))
		checkOp(t, fmt.Sprintf("RoundCeil(%s, %d)", s, p),
			func() alpaca.Decimal { return a.RoundCeil(p) }, shopD.RoundCeil(p))
		checkOp(t, fmt.Sprintf("RoundFloor(%s, %d)", s, p),
			func() alpaca.Decimal { return a.RoundFloor(p) }, shopD.RoundFloor(p))
		checkOp(t, fmt.Sprintf("RoundUp(%s, %d)", s, p),
			func() alpaca.Decimal { return a.RoundUp(p) }, shopD.RoundUp(p))
		checkOp(t, fmt.Sprintf("RoundDown(%s, %d)", s, p),
			func() alpaca.Decimal { return a.RoundDown(p) }, shopD.RoundDown(p))
		checkOp(t, fmt.Sprintf("Truncate(%s, %d)", s, p),
			func() alpaca.Decimal { return a.Truncate(p) }, shopD.Truncate(p))
	})
}

// FuzzRoundDown tests RoundDown (truncation towards zero) in isolation with
// places in [-30, 30].
//
// Validated properties:
//  1. Cross-library: result matches shopspring.
//  2. Idempotence: RoundDown(RoundDown(d, p), p) == RoundDown(d, p).
//  3. Towards zero: |RoundDown(d, p)| <= |d|.
func FuzzRoundDown(f *testing.F) {
	seeds := append([]string{
		"0", "1", "-1", "0.5", "-0.5",
		"1.1001", "-1.1001", "1.999", "-1.999",
		"123.456", "-123.456",
		"454545.454545", "-454545.454545",
		"0.001", "-0.001",
		"99", "-99", "545", "-545",
		"9223372.999", "-9223372.999", // around the optimized boundary
		"9223373.999", "-9223373.999", // fallback
	}, boundarySeeds...)
	for _, s := range seeds {
		for places := int8(-30); places <= 30; places += 5 {
			f.Add(s, places)
		}
	}

	f.Fuzz(func(t *testing.T, s string, places int8) {
		if places < -30 || places > 30 {
			return
		}
		p := int32(places)

		a, shopD, ok := parseBoth(t, s)
		if !ok {
			return
		}

		// Inputs are capped at 10^19, so RoundDown at places >= -30 cannot
		// leave the domain; the guard documents the iff rule.
		var rd alpaca.Decimal
		if domainGuard(func() { rd = a.RoundDown(p) }) {
			if !outOfDomain(shopD.RoundDown(p)) {
				t.Errorf("RoundDown(%s, %d): spurious domain panic", s, p)
			}
			return
		}

		// Property 1: cross-library agreement.
		compareAdjusted(t, fmt.Sprintf("RoundDown(%s, %d)", s, p), rd, shopD.RoundDown(p))

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
	seeds := append([]string{
		"0", "1", "-1", "1.5", "-1.5", "2.5", "-2.5",
		"1.001", "-1.001", "0.999", "-0.999",
		"123.456", "-123.456", "9223372.5",
	}, boundarySeeds...)
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		a, shopD, ok := parseBoth(t, s)
		if !ok {
			return
		}

		checkOp(t, fmt.Sprintf("Ceil(%s)", s),
			func() alpaca.Decimal { return a.Ceil() }, shopD.Ceil())
		checkOp(t, fmt.Sprintf("Floor(%s)", s),
			func() alpaca.Decimal { return a.Floor() }, shopD.Floor())
	})
}

// FuzzRoundCash tests RoundCash and StringFixedCash for all valid intervals.
func FuzzRoundCash(f *testing.F) {
	seeds := []string{"0", "1.23", "-1.23", "3.43", "3.45", "3.41", "3.75", "3.50", "9223372.5", "9223373.5"}
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

		a, shopD, ok := parseBoth(t, s)
		if !ok {
			return
		}

		checkOp(t, fmt.Sprintf("RoundCash(%s, %d)", s, interval),
			func() alpaca.Decimal { return a.RoundCash(interval) }, shopD.RoundCash(interval))

		checkOpString(t, fmt.Sprintf("StringFixedCash(%s, %d)", s, interval),
			func() string { return a.StringFixedCash(interval) },
			shopD.StringFixedCash(interval), shopD)
	})
}

// ---------------------------------------------------------------------------
// Aggregate tests
// ---------------------------------------------------------------------------

// FuzzAggregates tests Sum, Avg, Max, and Min.
func FuzzAggregates(f *testing.F) {
	seeds := append([]string{
		"0", "1", "-1", "123.456", "-999.999", "0.001",
	}, boundarySeeds...)
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b)
		}
	}

	f.Fuzz(func(t *testing.T, aStr, bStr string) {
		a, shopA, ok := parseBoth(t, aStr)
		if !ok {
			return
		}
		b, shopB, ok := parseBoth(t, bStr)
		if !ok {
			return
		}

		checkOp(t, fmt.Sprintf("Sum(%s, %s)", aStr, bStr),
			func() alpaca.Decimal { return alpaca.Sum(a, b) }, shopspring.Sum(shopA, shopB))

		// Avg = Sum/n; both libraries round at DivisionPrecision=16, so the
		// results must match exactly.
		checkOp(t, fmt.Sprintf("Avg(%s, %s)", aStr, bStr),
			func() alpaca.Decimal { return alpaca.Avg(a, b) }, shopspring.Avg(shopA, shopB))

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
	seeds := append([]string{
		"0", "1", "-1", "1.5", "-1.5",
		"100", "0.001",
		"9999999999", "-9999999999", // large fallback
		"0.1234567890123456789", // 19 fractional digits
	}, boundarySeeds...)
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		a, shopD, ok := parseBoth(t, s)
		if !ok {
			return
		}

		// IntPart: skip values whose integer part overflows int64 (behavior
		// is documented as undefined in shopspring there).
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
// GetFixed, GetFallback, Equals (deprecated), and NewFromDecimal.
func FuzzConversions(f *testing.F) {
	seeds := append([]string{
		"0", "1", "-1", "1.5", "-1.5",
		"100", "0.001",
		"123456", "-999999.99",
		"123.456789", "-999999.123456789012",
		"9999999999999999999",
		"0.000000000001",
		"0.1234567890123456789",
		// big.Int-backed fallback with trailing fractional zero: documents
		// the Coefficient/Exponent inconsistency carve-out below
		"-000000000000000000000000000000000.0000010",
	}, boundarySeeds...)
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		a, shopD, ok := parseBoth(t, s)
		if !ok {
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
		// (zerodecimal's ToHiLo is total, so the udecimal-era big.Int
		// coefficient carve-out is gone.)
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

		// --- NewFromDecimal ---
		// Parse s as zerodecimal and verify round-trip through NewFromDecimal.
		zd, err := zerodecimal.NewFromStringTrunc(s)
		if err == nil {
			fromZD := alpaca.NewFromDecimal(zd)
			if !fromZD.Equal(a) {
				t.Errorf("NewFromDecimal(%s): got %s expected %s", s, fromZD.String(), a.String())
			}
		}
	})
}

// ---------------------------------------------------------------------------
// String formatting tests
// ---------------------------------------------------------------------------

// FuzzStringFormats tests StringFixed, StringFixedBank, and StringScaled.
func FuzzStringFormats(f *testing.F) {
	seeds := append([]string{
		"0", "1", "-1", "1.23", "-1.23", "0.001", "999.999",
		"9223372.5", "9223373.5",
	}, boundarySeeds...)
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

		a, shopD, ok := parseBoth(t, s)
		if !ok {
			return
		}

		// StringFixed
		checkOpString(t, fmt.Sprintf("StringFixed(%s, %d)", s, p),
			func() string { return a.StringFixed(p) }, shopD.StringFixed(p), shopD)

		// StringFixedBank
		checkOpString(t, fmt.Sprintf("StringFixedBank(%s, %d)", s, p),
			func() string { return a.StringFixedBank(p) }, shopD.StringFixedBank(p), shopD)

		// StringScaled is deprecated; it truncates (not rounds) and trims,
		// exactly like shopspring's rescale + String.
		checkOpString(t, fmt.Sprintf("StringScaled(%s, -%d)", s, p),
			func() string { return a.StringScaled(-p) }, shopD.StringScaled(-p), shopD)
	})
}

// ---------------------------------------------------------------------------
// Serialization tests
// ---------------------------------------------------------------------------

// FuzzSerialization tests round-trip consistency for all serialization
// formats (JSON, Text, Binary, Gob, database Value/Scan), raw JSON parity
// with shopspring (including unquoted scientific-notation JSON numbers), and
// CROSS-LIBRARY binary compatibility: the wire format is shopspring's, so
// bytes must decode to the same value in either library.
func FuzzSerialization(f *testing.F) {
	seeds := append([]string{
		"0", "1", "-1", "1.23", "-1.23",
		"0.000000000001", "9999999.999999",
		"123456789.123456789",
		"0.1234567890123456789",
		// raw JSON numbers in scientific notation
		"1.5e3", "1e5", "1.23e-4", "-1.5E+3",
		// quoted JSON strings
		`"1.23"`, `"-4.5e2"`,
	}, boundarySeeds...)
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 500 || sciExpOutOfBounds(s) {
			return
		}

		// --- Raw JSON parity (covers unquoted scientific-notation numbers
		// like 1.5e3 and quoted strings like "1.23") ---
		if knownParseDivergence(s) {
			t.Skip("known parser divergence (see knownParseDivergence)")
		}
		var aRaw alpaca.Decimal
		aRawErr := json.Unmarshal([]byte(s), &aRaw)
		var sRaw shopspring.Decimal
		sRawErr := json.Unmarshal([]byte(s), &sRaw)
		// 128-bit domain rules (iff): out-of-domain values must be rejected
		// with the domain error; in-domain values must never raise it.
		if sRawErr == nil {
			if outOfDomain(sRaw) {
				if aRawErr == nil {
					t.Errorf("json.Unmarshal(%q) accepted out-of-domain value: alpaca=%s shopspring=%s",
						s, aRaw.String(), sRaw.String())
				} else if !isDomainError(aRawErr) {
					t.Errorf("json.Unmarshal(%q): expected domain error for out-of-domain value %s, got: %v",
						s, sRaw.String(), aRawErr)
				}
			} else if isDomainError(aRawErr) {
				t.Errorf("json.Unmarshal(%q): spurious domain error for in-domain value %s: %v",
					s, sRaw.String(), aRawErr)
			}
		}
		if (aRawErr != nil) != (sRawErr != nil) {
			if !(sRawErr == nil && !comparableValue(sRaw)) {
				t.Errorf("json.Unmarshal(%q) error parity: alpaca=%v shopspring=%v", s, aRawErr, sRawErr)
			}
		} else if aRawErr == nil && comparableValue(sRaw) {
			compare(t, fmt.Sprintf("json.Unmarshal(%q)", s), aRaw, sRaw)
		}

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

		// Cross-library binary: alpaca encodes -> shopspring decodes.
		// sRef is the exact shopspring twin of a (a.String() always has at
		// most 19 fractional digits, so it parses exactly).
		sRef, err := shopspring.NewFromString(a.String())
		if err != nil {
			t.Errorf("shopspring failed to parse alpaca String %q: %v", a.String(), err)
			return
		}
		var sCross shopspring.Decimal
		if err := sCross.UnmarshalBinary(binBytes); err != nil {
			t.Errorf("shopspring.UnmarshalBinary(alpaca bytes)(%s): %v", s, err)
		} else if sCross.Cmp(sRef) != 0 {
			t.Errorf("cross binary alpaca->shopspring(%s): got %s want %s", s, sCross.String(), sRef.String())
		}

		// Cross-library binary: shopspring encodes -> alpaca decodes.
		shopBin, err := sRef.MarshalBinary()
		if err != nil {
			t.Errorf("shopspring.MarshalBinary(%s): %v", sRef.String(), err)
			return
		}
		var aCross alpaca.Decimal
		if err := aCross.UnmarshalBinary(shopBin); err != nil {
			t.Errorf("alpaca.UnmarshalBinary(shopspring bytes)(%s): %v", s, err)
		} else if !aCross.Equal(a) {
			t.Errorf("cross binary shopspring->alpaca(%s): got %s want %s", s, aCross.String(), a.String())
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

// FuzzUnmarshalBinary feeds raw bytes to UnmarshalBinary on both libraries.
// The wire format is shopspring's (4 big-endian exponent bytes followed by a
// gob-encoded big.Int), so error presence must agree, with two accepted
// divergences:
//
//   - 128-bit domain: shopspring decodes coefficients of any magnitude;
//     alpacadecimal must reject out-of-domain values with the domain error —
//     and must neither raise it for in-domain values nor accept
//     out-of-domain ones (the iff rule);
//   - exponent DoS guard: alpacadecimal rejects wire exponents beyond ±10000
//     because it materializes the coefficient eagerly, where shopspring
//     stores the exponent lazily and accepts.
//
// Garbage input must never panic: the alpacadecimal call is unguarded, so
// any panic crashes the target.
func FuzzUnmarshalBinary(f *testing.F) {
	for _, s := range []string{
		"0", "1", "-1", "123.456", "-0.0000000000000000001",
		"9223372.000000000001", // optimized boundary
		"10000000000000000000",
		"0.12345678901234567890123456789",         // >19 fractional digits
		"340282366920938463463374607431768211455", // 2^128-1: in domain
		"340282366920938463463374607431768211456", // 2^128: out of domain
		"-340282366920938463463374607431768211456",
		"340282366920938463463374607431768211456.5",
		"1e100", "-1e100", "1e-100",
	} {
		b, err := shopspring.RequireFromString(s).MarshalBinary()
		if err != nil {
			f.Fatalf("seed %q: MarshalBinary: %v", s, err)
		}
		f.Add(b)
	}
	// garbage seeds
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00})
	f.Add([]byte("garbage input"))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0x02, 0x01})
	f.Add([]byte{0x00, 0x00, 0x00, 0x01, 0x04, 0xff}) // unsupported gob version

	f.Fuzz(func(t *testing.T, data []byte) {
		// Bound the coefficient size: gigantic big.Ints only slow the run.
		if len(data) > 200 {
			return
		}

		var aD alpaca.Decimal
		aErr := aD.UnmarshalBinary(data) // must never panic, even on garbage

		var sD shopspring.Decimal
		sErr := sD.UnmarshalBinary(data)

		// Intentional divergence: wire exponents beyond the ±10000 DoS guard
		// are rejected by alpacadecimal and lazily accepted by shopspring.
		// Cross-checking would force shopspring to materialize 10^|exp|, so
		// these inputs are skipped entirely (after the no-panic check above).
		if len(data) >= 4 {
			if exp := int32(binary.BigEndian.Uint32(data[:4])); exp > 10000 || exp < -10000 {
				return
			}
		}

		if sErr != nil {
			// The wire format is shared, so bytes shopspring rejects must be
			// rejected by alpacadecimal too.
			if aErr == nil {
				t.Errorf("UnmarshalBinary(%x): alpaca accepted bytes shopspring rejects (%v): got %s",
					data, sErr, aD.String())
			}
			return
		}

		// 128-bit domain rules (iff).
		if outOfDomain(sD) {
			if aErr == nil {
				t.Errorf("UnmarshalBinary(%x) accepted out-of-domain value: alpaca=%s shopspring=%s",
					data, aD.String(), sD.String())
			} else if !isDomainError(aErr) {
				t.Errorf("UnmarshalBinary(%x): expected domain error for out-of-domain value %s, got: %v",
					data, sD.String(), aErr)
			}
			return
		}
		if aErr != nil {
			if isDomainError(aErr) {
				t.Errorf("UnmarshalBinary(%x): spurious domain error for in-domain value %s: %v",
					data, sD.String(), aErr)
			} else {
				t.Errorf("UnmarshalBinary(%x) error parity: alpaca=%v shopspring=nil (value %s)",
					data, aErr, sD.String())
			}
			return
		}

		// Both decoded an in-domain value: alpaca truncates fractional digits
		// beyond 19 and then to a 128-bit coefficient; compare accordingly.
		compareAdjusted(t, fmt.Sprintf("UnmarshalBinary(%x)", data), aD, sD.Truncate(maxPrec))
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

		if len(s) > 100 || sciExpOutOfBounds(s) {
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

// FuzzScanTypes tests Decimal.Scan round-trips with various Go types derived
// from a single parsed value: string, []byte, int64, and float64.
func FuzzScanTypes(f *testing.F) {
	seeds := append([]string{"123.456", "-999.99", "0", "1"}, boundarySeeds...)
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 100 || sciExpOutOfBounds(s) {
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

		// Scan from int64 (for integer values; IntPart is exact within range).
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

// FuzzScan cross-checks Decimal.Scan against shopspring's Scan for every
// supported source type: string, []byte, float64, float32, int64, and uint64.
// The selector picks the type; the string is converted to that type when
// possible. Both the error presence and the resulting value must agree.
func FuzzScan(f *testing.F) {
	seeds := append([]string{
		"0", "1.23", "-1.23", "123.456", "abc", "",
		"1e5", "1.23e-4", `"1.5"`,
		"42", "-42", "18446744073709551615",
		"0.1234567890123456789",
	}, boundarySeeds...)
	for _, s := range seeds {
		for sel := int8(0); sel < 6; sel++ {
			f.Add(s, sel)
		}
	}

	f.Fuzz(func(t *testing.T, s string, sel int8) {
		if sel < 0 || sel > 5 {
			return
		}
		if len(s) > 100 || sciExpOutOfBounds(s) {
			return
		}
		if knownParseDivergence(s) {
			t.Skip("known parser divergence (see knownParseDivergence)")
		}

		var src interface{}
		switch sel {
		case 0:
			src = s
		case 1:
			src = []byte(s)
		case 2:
			v, err := strconv.ParseFloat(s, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return
			}
			// Out-of-domain floats (|v| >= 2^128): Scan must return the
			// domain error (decode-path contract) while shopspring succeeds.
			if math.Abs(v) >= math.Ldexp(1, 128) {
				var aD alpaca.Decimal
				err := aD.Scan(v)
				if err == nil || !isDomainError(err) {
					t.Errorf("Scan(float64 %v): expected domain error, got %v", v, err)
				}
				return
			}
			// Both libraries Scan floats via NewFromFloat (minimal
			// representation); alpaca truncates past 19 fractional digits.
			if fracDigitsOf(strconv.FormatFloat(v, 'f', -1, 64)) > maxPrec {
				return
			}
			src = v
		case 3:
			v, err := strconv.ParseFloat(s, 32)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return
			}
			// see the case 2 comment: out-of-domain float32 Scan must
			// return the domain error
			if math.Abs(float64(float32(v))) >= math.Ldexp(1, 128) {
				var aD alpaca.Decimal
				err := aD.Scan(float32(v))
				if err == nil || !isDomainError(err) {
					t.Errorf("Scan(float32 %v): expected domain error, got %v", float32(v), err)
				}
				return
			}
			if fracDigitsOf(strconv.FormatFloat(float64(float32(v)), 'f', -1, 64)) > maxPrec {
				return
			}
			src = float32(v)
		case 4:
			v, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return
			}
			src = v
		case 5:
			v, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return
			}
			src = v
		}

		var aD alpaca.Decimal
		aErr := aD.Scan(src)
		var sD shopspring.Decimal
		sErr := sD.Scan(src)

		// 128-bit domain rules (iff): out-of-domain values must be rejected
		// with the domain error; in-domain values must never raise it.
		if sErr == nil {
			if outOfDomain(sD) {
				if aErr == nil {
					t.Errorf("Scan(%T from %q) accepted out-of-domain value: alpaca=%s shopspring=%s",
						src, s, aD.String(), sD.String())
				} else if !isDomainError(aErr) {
					t.Errorf("Scan(%T from %q): expected domain error for out-of-domain value %s, got: %v",
						src, s, sD.String(), aErr)
				}
				return
			}
			if isDomainError(aErr) {
				t.Errorf("Scan(%T from %q): spurious domain error for in-domain value %s: %v",
					src, s, sD.String(), aErr)
				return
			}
		}

		if (aErr != nil) != (sErr != nil) {
			if sErr == nil && !comparableValue(sD) {
				return // documented representational limit
			}
			t.Errorf("Scan(%T from %q) error parity: alpaca=%v shopspring=%v", src, s, aErr, sErr)
			return
		}
		if aErr != nil {
			return
		}
		if !comparableValue(sD) {
			return
		}
		compare(t, fmt.Sprintf("Scan(%T from %q)", src, s), aD, sD)
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

		checkOp(t, fmt.Sprintf("[%d] Add(%s, %s)", i, aStr, bStr),
			func() alpaca.Decimal { return a.Add(b) }, shopA.Add(shopB))
		checkOp(t, fmt.Sprintf("[%d] Sub(%s, %s)", i, aStr, bStr),
			func() alpaca.Decimal { return a.Sub(b) }, shopA.Sub(shopB))
		// Products of 15-integer-digit operands routinely exceed a 128-bit
		// coefficient at 19 fractional digits; checkOpTruncated's domain
		// adjustment models the extra fractional truncation.
		checkOpTruncated(t, fmt.Sprintf("[%d] Mul(%s, %s)", i, aStr, bStr),
			func() alpaca.Decimal { return a.Mul(b) }, shopA.Mul(shopB), maxPrec)

		if !b.IsZero() {
			checkOp(t, fmt.Sprintf("[%d] Mod(%s, %s)", i, aStr, bStr),
				func() alpaca.Decimal { return a.Mod(b) }, shopA.Mod(shopB))
		}

		places := int32(rng.Intn(36) - 10) // [-10, 25]

		checkOp(t, fmt.Sprintf("[%d] Round(%s, %d)", i, aStr, places),
			func() alpaca.Decimal { return a.Round(places) }, shopA.Round(places))
		checkOp(t, fmt.Sprintf("[%d] RoundBank(%s, %d)", i, aStr, places),
			func() alpaca.Decimal { return a.RoundBank(places) }, shopA.RoundBank(places))
		checkOp(t, fmt.Sprintf("[%d] Truncate(%s, %d)", i, aStr, places),
			func() alpaca.Decimal { return a.Truncate(places) }, shopA.Truncate(places))
		checkOp(t, fmt.Sprintf("[%d] RoundCeil(%s, %d)", i, aStr, places),
			func() alpaca.Decimal { return a.RoundCeil(places) }, shopA.RoundCeil(places))
		checkOp(t, fmt.Sprintf("[%d] RoundFloor(%s, %d)", i, aStr, places),
			func() alpaca.Decimal { return a.RoundFloor(places) }, shopA.RoundFloor(places))
		checkOp(t, fmt.Sprintf("[%d] RoundUp(%s, %d)", i, aStr, places),
			func() alpaca.Decimal { return a.RoundUp(places) }, shopA.RoundUp(places))
		checkOp(t, fmt.Sprintf("[%d] RoundDown(%s, %d)", i, aStr, places),
			func() alpaca.Decimal { return a.RoundDown(places) }, shopA.RoundDown(places))

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
