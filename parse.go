package alpacadecimal

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// parseFixed parses a plain decimal string ("123", "-12.34", "0.001") directly
// into the fixed representation. It is strict: it accepts exactly the inputs
// shopspring accepts AND that fit the fixed range, otherwise reports false so
// the caller can take the slow path.
//
// Accepted shapes mirror shopspring: optional sign, digits, optional single
// dot, digits ("5.", ".5" included; "." and "" excluded).
func parseFixed[T string | []byte](v T) (int64, bool) {
	n := len(v)
	if n == 0 || n > 21 {
		return 0, false
	}
	neg := false
	i := 0
	switch v[0] {
	case '-':
		neg = true
		i = 1
	case '+':
		i = 1
	}

	var ip int64 // integer part
	intDigits := 0
	for ; i < n; i++ {
		c := v[i]
		if c == '.' {
			break
		}
		c -= '0'
		if c > 9 {
			return 0, false
		}
		ip = ip*10 + int64(c)
		intDigits++
		if ip > maxInt {
			return 0, false
		}
	}

	if i == n {
		if intDigits == 0 {
			return 0, false // "", "-", "+"
		}
		f := ip * scale
		if neg {
			f = -f
		}
		return f, true
	}

	// v[i] == '.'
	i++
	var frac int64
	fracDigits := 0
	for ; i < n && fracDigits < 12; i++ {
		c := v[i] - '0'
		if c > 9 {
			return 0, false
		}
		frac = frac*10 + int64(c)
		fracDigits++
	}
	// Digits beyond 12 fractional places must be zeros to stay exact.
	for ; i < n; i++ {
		if v[i] != '0' {
			return 0, false
		}
	}
	if intDigits == 0 && fracDigits == 0 {
		return 0, false // ".", "-.", "+."
	}
	frac *= pow10Table[12-fracDigits]
	if ip == maxInt && frac != 0 {
		return 0, false // would exceed ±9223372.000000000000
	}
	f := ip*scale + frac
	if neg {
		f = -f
	}
	return f, true
}

// parseModeError tracks whether SetDefaultParseModeError was selected, so the
// extended parsing paths (scientific notation, >200 char inputs) can honor it.
var parseModeError = false

// parseSlow handles every valid input that parseFixed does not: out-of-range
// magnitudes, more than 12 fractional digits, scientific notation, and
// arbitrarily long plain notation. Acceptance matches shopspring; values whose
// fractional part exceeds 19 digits are truncated (or rejected when
// SetDefaultParseModeError is configured).
func parseSlow(value string) (Decimal, error) {
	if eIdx := strings.IndexAny(value, "eE"); eIdx >= 0 {
		return parseSci(value, eIdx)
	}
	return parseMantissa(value, value, 0)
}

// maxSciExponent bounds scientific-notation exponents so a tiny input like
// "9e80000000" cannot force materialization of a multi-megabyte coefficient
// (shopspring stores such exponents lazily; we materialize at parse time).
const maxSciExponent = 10_000

// parseSci parses scientific notation ("1.23e4"), mirroring shopspring's
// NewFromString behavior and error messages.
func parseSci(value string, eIdx int) (Decimal, error) {
	expPart := value[eIdx+1:]
	exp, err := strconv.ParseInt(expPart, 10, 32)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return Zero, fmt.Errorf("can't convert %s to decimal: fractional part too long", value)
		}
		return Zero, fmt.Errorf("can't convert %s to decimal: exponent is not numeric", value)
	}
	if exp > maxSciExponent || exp < -maxSciExponent {
		return Zero, fmt.Errorf("can't convert %s to decimal: exponent out of range", value)
	}
	return parseMantissa(value, value[:eIdx], exp)
}

// parseMantissa is the strict general parser: an optionally signed digit
// string with at most one dot, times 10^exp. It handles arbitrary lengths via
// big.Int and applies the package truncation convention beyond 19 fractional
// digits. Acceptance matches shopspring's NewFromString.
func parseMantissa(original, v string, exp int64) (Decimal, error) {
	neg := false
	i := 0
	if len(v) > 0 && (v[0] == '-' || v[0] == '+') {
		neg = v[0] == '-'
		i = 1
	}

	var m int64    // mantissa accumulator, valid while sig <= 18
	sig := 0       // significant digits (leading zeros skipped)
	sigStart := -1 // index of the first significant digit
	dot := -1
	digitSeen := false
	for ; i < len(v); i++ {
		c := v[i]
		if c == '.' {
			if dot >= 0 {
				return Zero, fmt.Errorf("can't convert %s to decimal: too many .s", original)
			}
			dot = i
			continue
		}
		c -= '0'
		if c > 9 {
			return Zero, fmt.Errorf("can't convert %s to decimal", original)
		}
		digitSeen = true
		if dot >= 0 {
			exp--
		}
		if sig == 0 && c == 0 {
			continue // leading zero
		}
		if sig == 0 {
			sigStart = i
		}
		sig++
		if sig <= 18 {
			m = m*10 + int64(c)
		}
	}
	if !digitSeen {
		return Zero, fmt.Errorf("can't convert %s to decimal", original)
	}

	if sig <= 18 {
		if parseModeError && exp < -19 && truncatedInt64LosesDigits(m, exp) {
			return Zero, fmt.Errorf("can't convert %s to decimal: %w", original, errPrecisionOutOfRange)
		}
		if neg {
			m = -m
		}
		return newFromInt64Exp(m, exp), nil
	}

	// Big mantissa: rebuild the significant digits without the dot.
	var b strings.Builder
	b.Grow(len(v) - sigStart)
	for j := sigStart; j < len(v); j++ {
		if v[j] != '.' {
			b.WriteByte(v[j])
		}
	}
	digits := b.String()
	if parseModeError && truncationLosesDigits(digits, exp) {
		return Zero, fmt.Errorf("can't convert %s to decimal: %w", original, errPrecisionOutOfRange)
	}
	bi, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return Zero, fmt.Errorf("can't convert %s to decimal", original)
	}
	if neg {
		bi.Neg(bi)
	}
	return decimalFromBigParts(bi, exp), nil
}

// truncatedInt64LosesDigits reports whether representing m * 10^exp (exp <
// -19) with 19 fractional digits drops nonzero digits.
func truncatedInt64LosesDigits(m int64, exp int64) bool {
	shift := -exp - 19
	if shift > 18 {
		return m != 0
	}
	return m%pow10Table[shift] != 0
}

var errPrecisionOutOfRange = fmt.Errorf("precision out of range. Only support maximum 19 digits after the decimal point")

// truncationLosesDigits reports whether representing digits * 10^exp with at
// most 19 fractional digits would drop nonzero digits.
func truncationLosesDigits(digits string, exp int64) bool {
	if exp >= -19 {
		return false
	}
	drop := -exp - 19 // number of trailing digits dropped
	if drop >= int64(len(digits)) {
		drop = int64(len(digits))
	}
	for i := int64(len(digits)) - drop; i < int64(len(digits)); i++ {
		if digits[i] != '0' {
			return true
		}
	}
	return false
}

// unquoteIfQuoted mirrors shopspring's helper used by Scan and UnmarshalJSON.
func unquoteIfQuoted(value interface{}) (string, error) {
	var b []byte
	switch v := value.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		return "", fmt.Errorf("could not convert value '%+v' to byte array of type '%T'", value, value)
	}
	if len(b) > 2 && b[0] == '"' && b[len(b)-1] == '"' {
		b = b[1 : len(b)-1]
	}
	return string(b), nil
}
