package alpacadecimal

import (
	"database/sql/driver"
	"strings"

	"github.com/quagmt/udecimal"
)

// optimized:
// String returns the string representation of the decimal
// with the fixed point.
//
// Like shopspring, trailing zeros are trimmed: "1.500" prints as "1.5".
func (d Decimal) String() string {
	if d.fallback == nil {
		// cache hit
		if d.fixed <= a1000InFixed && d.fixed >= aNeg1000InFixed && d.fixed%aCentInFixed == 0 {
			return stringCache[d.fixed/aCentInFixed+cacheOffset]
		}
		return formatFixed(d.fixed)
	}
	return formatFallback(*d.fallback)
}

// formatFixed renders a fixed value, trimming trailing fractional zeros.
func formatFixed(fixed int64) string {
	// "-9223372.000000000000" => max length = 21 bytes
	var s [21]byte
	start := 7
	end := 8

	var ufixed uint64
	if fixed >= 0 {
		ufixed = uint64(fixed)
	} else {
		ufixed = uint64(-fixed)
	}

	integerPart := ufixed / scale
	fractionalPart := ufixed % scale

	// integer part
	if integerPart == 0 {
		s[start] = '0'
	} else {
		for integerPart >= 10 {
			s[start] = byte(integerPart%10 + '0')
			start--
			integerPart /= 10
		}
		s[start] = byte(integerPart + '0')
	}

	// fractional part
	if fractionalPart > 0 {
		s[8] = '.'
		for i := 20; i > 8; i-- {
			is := fractionalPart % 10
			fractionalPart /= 10
			if is != 0 {
				s[i] = byte(is + '0')
				end = i + 1
				for j := i - 1; j > 8; j-- {
					s[j] = byte(fractionalPart%10 + '0')
					fractionalPart /= 10
				}
				break
			}
		}
	}

	// sign part
	if fixed < 0 {
		start -= 1
		s[start] = '-'
	}

	return string(s[start:end])
}

// appendFixed appends the same rendering as formatFixed to b.
func appendFixed(b []byte, fixed int64) []byte {
	var s [21]byte
	str := formatFixedInto(&s, fixed)
	return append(b, str...)
}

func formatFixedInto(s *[21]byte, fixed int64) []byte {
	start := 7
	end := 8

	var ufixed uint64
	if fixed >= 0 {
		ufixed = uint64(fixed)
	} else {
		ufixed = uint64(-fixed)
	}

	integerPart := ufixed / scale
	fractionalPart := ufixed % scale

	if integerPart == 0 {
		s[start] = '0'
	} else {
		for integerPart >= 10 {
			s[start] = byte(integerPart%10 + '0')
			start--
			integerPart /= 10
		}
		s[start] = byte(integerPart + '0')
	}

	if fractionalPart > 0 {
		s[8] = '.'
		for i := 20; i > 8; i-- {
			is := fractionalPart % 10
			fractionalPart /= 10
			if is != 0 {
				s[i] = byte(is + '0')
				end = i + 1
				for j := i - 1; j > 8; j-- {
					s[j] = byte(fractionalPart%10 + '0')
					fractionalPart /= 10
				}
				break
			}
		}
	}

	if fixed < 0 {
		start -= 1
		s[start] = '-'
	}

	return s[start:end]
}

// formatFixedPlaces renders a fixed value with exactly places fractional
// digits (places in [1, 12]); the value must already be rounded at places.
func formatFixedPlaces(fixed int64, places int32) string {
	// "-9223372." + up to 12 fractional digits => max 21 bytes
	var s [21]byte
	start := 7

	var ufixed uint64
	if fixed >= 0 {
		ufixed = uint64(fixed)
	} else {
		ufixed = uint64(-fixed)
	}

	integerPart := ufixed / scale
	fractionalPart := ufixed % scale

	if integerPart == 0 {
		s[start] = '0'
	} else {
		for integerPart >= 10 {
			s[start] = byte(integerPart%10 + '0')
			start--
			integerPart /= 10
		}
		s[start] = byte(integerPart + '0')
	}

	s[8] = '.'
	// write exactly `places` digits: drop the trailing (12-places) zeros
	fractionalPart /= pow10u[12-places]
	end := 9 + int(places)
	for i := end - 1; i > 8; i-- {
		s[i] = byte(fractionalPart%10 + '0')
		fractionalPart /= 10
	}

	if fixed < 0 {
		start -= 1
		s[start] = '-'
	}

	return string(s[start:end])
}

// formatFallback renders a fallback value identically to udecimal.String,
// with a faster path for coefficients that fit in 64 bits.
func formatFallback(fb udecimal.Decimal) string {
	neg, hi, lo, prec, ok := fb.ToHiLo()
	if !ok || hi != 0 {
		return fb.String()
	}
	return formatU64(neg, lo, int(prec))
}

// formatU64 renders coef * 10^-prec (sign-applied), trimming trailing zeros.
func formatU64(neg bool, coef uint64, prec int) string {
	if coef == 0 {
		return "0"
	}
	var buf [32]byte
	pos := len(buf)
	if prec > 0 {
		p := pow10u[prec]
		ip := coef / p
		fp := coef % p
		for prec > 0 && fp%10 == 0 {
			fp /= 10
			prec--
		}
		if prec > 0 {
			for i := 0; i < prec; i++ {
				pos--
				buf[pos] = byte(fp%10) + '0'
				fp /= 10
			}
			pos--
			buf[pos] = '.'
		}
		coef = ip
	}
	if coef == 0 {
		pos--
		buf[pos] = '0'
	} else {
		for coef > 0 {
			pos--
			buf[pos] = byte(coef%10) + '0'
			coef /= 10
		}
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// optimized:
// StringFixed returns a rounded fixed-point string with places digits after
// the decimal point.
//
// Example:
//
//	NewFromFloat(0).StringFixed(2) // output: "0.00"
//	NewFromFloat(0).StringFixed(0) // output: "0"
//	NewFromFloat(5.45).StringFixed(0) // output: "5"
//	NewFromFloat(5.45).StringFixed(1) // output: "5.5"
//	NewFromFloat(5.45).StringFixed(2) // output: "5.45"
//	NewFromFloat(5.45).StringFixed(3) // output: "5.450"
//	NewFromFloat(545).StringFixed(-1) // output: "550"
func (d Decimal) StringFixed(places int32) string {
	rounded := d.Round(places)
	if rounded.fallback == nil && places >= 1 && places <= 12 {
		return formatFixedPlaces(rounded.fixed, places)
	}
	return padStringToPlaces(rounded.String(), places)
}

// fallback:
// StringFixedBank returns a banker rounded fixed-point string with places digits
// after the decimal point.
func (d Decimal) StringFixedBank(places int32) string {
	rounded := d.RoundBank(places)
	if rounded.fallback == nil && places >= 1 && places <= 12 {
		return formatFixedPlaces(rounded.fixed, places)
	}
	return padStringToPlaces(rounded.String(), places)
}

// fallback:
// StringFixedCash returns a Swedish/Cash rounded fixed-point string. For
// more details see the documentation at function RoundCash.
func (d Decimal) StringFixedCash(interval uint8) string {
	rounded := d.RoundCash(interval)
	if rounded.fallback == nil {
		return formatFixedPlaces(rounded.fixed, 2)
	}
	return padStringToPlaces(rounded.String(), 2)
}

// fallback:
// StringScaled first scales the decimal (truncating, like shopspring's
// internal rescale), then calls String() on it.
//
// Deprecated: buggy and unintuitive. Use StringFixed instead.
func (d Decimal) StringScaled(exp int32) string {
	return d.RoundDown(-exp).String()
}

// padStringToPlaces pads or formats a decimal string to have exactly `places` digits after the decimal point.
func padStringToPlaces(s string, places int32) string {
	if places <= 0 {
		// for zero or negative places, return the (already rounded) integer
		if idx := strings.IndexByte(s, '.'); idx >= 0 {
			s = s[:idx]
		}
		return s
	}

	dotIdx := strings.IndexByte(s, '.')
	if dotIdx < 0 {
		return s + "." + strings.Repeat("0", int(places))
	}

	fracLen := int32(len(s) - dotIdx - 1)
	if fracLen >= places {
		return s
	}
	return s + strings.Repeat("0", int(places-fracLen))
}

// optimized:
// Value implements the driver.Valuer interface for database serialization.
func (d Decimal) Value() (driver.Value, error) {
	if d.fallback == nil {
		// cache hit
		if d.fixed <= a1000InFixed && d.fixed >= aNeg1000InFixed && d.fixed%aCentInFixed == 0 {
			return valueCache[d.fixed/aCentInFixed+cacheOffset], nil
		}
		return formatFixed(d.fixed), nil
	}
	return formatFallback(*d.fallback), nil
}
