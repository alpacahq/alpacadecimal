package alpacadecimal

import (
	"database/sql/driver"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// currently support 12 precision, this is tunnable,
// more precision => smaller maxInt
// less precision => bigger maxInt
const scale = 1000 * 1000 * 1000 * 1000
const maxInt int64 = math.MaxInt64 / scale
const minInt int64 = math.MinInt64 / scale
const maxDecimalDigits int64 = maxInt * scale
const minDecimalDigits int64 = minInt * scale

type OptimizationLevel int

const OptimizationLevelCached OptimizationLevel = 0
const OptimizationLevelRegular OptimizationLevel = 1
const OptimizationLevelFallback OptimizationLevel = 2

type Decimal struct {
	// represent decimal with 12 precision, 1.23 will have `i = 1_230_000_000_000`
	// max support decimal is 9_223_372
	// min support decimal is -9_223_372
	i int64

	// fallback to original decimal.Decimal if necessary
	fallback *decimal.Decimal
}

var valueCache map[int64]driver.Value
var scanCache map[string]int64

func init() {
	// init cache
	valueCache = make(map[int64]driver.Value)
	scanCache = make(map[string]int64)

	for i := 0.0; i < 1000.0; i += 0.01 {
		digits := int64(i*100) * 10 * 1000 * 1000 * 1000
		if i == float64(int64(i)) {
			// whole number
			valueCache[digits] = fmt.Sprintf("%d", int64(i))
			valueCache[-digits] = fmt.Sprintf("-%d", int64(i))

			scanCache[fmt.Sprintf("%d", int64(i))] = digits
			scanCache[fmt.Sprintf("-%d", int64(i))] = -digits
			// TODO: add quoted form for scanCache
		} else {
			valueCache[digits] = fmt.Sprintf("%f", i)
			valueCache[-digits] = fmt.Sprintf("-%f", i)

			scanCache[fmt.Sprintf("%f", i)] = digits
			scanCache[fmt.Sprintf("-%f", i)] = -digits
			// TODO: add quoted form for scanCache
		}
	}
}

func NewFromInt(x int64) Decimal {
	// if it's smaller than max int supported, then it's, otherwise, it's go to fallback
	if x > minInt && x < maxInt {
		return Decimal{i: x * scale}
	}
	fallback := decimal.NewFromInt(x)
	return Decimal{fallback: &fallback}
}

func NewFromFloat(f float64) Decimal {
	panic("TODO")
}

func NewFromDecimal(d decimal.Decimal) Decimal {
	// TODO: if it's within optimization range
	// might cast to optimized version instead of fallback
	return Decimal{fallback: &d}
}

func (d Decimal) String() string {
	return "TODO"
}

func (d Decimal) GetOptimizationLevel() OptimizationLevel {
	if d.fallback == nil {
		return OptimizationLevelFallback
	}
	if _, ok := valueCache[d.i]; ok {
		return OptimizationLevelCached
	}
	return OptimizationLevelRegular
}

// sql support

// sql.Valuer interface
func (d Decimal) Value() (driver.Value, error) {
	if d.fallback == nil {
		// this cached value has type `driver.Value`
		// which avoids runtime.convTstring
		if v, ok := valueCache[d.i]; ok {
			return v, nil
		}
		mod := d.i % scale
		if mod == 0 {
			// whole number
			return strconv.FormatInt(d.i/scale, 10), nil
		} else {
			// TODO: handle float
			panic("TODO: handle float")
		}
	}

	return d.fallback.String(), nil
}

// sql.Scanner interface
func (d *Decimal) Scan(value interface{}) error {

	// TODO: optimize this case
	switch v := value.(type) {
	case string:
		// fmt.Printf("it's string %s\n", v)
		// for len(v)

		// only check if the last digit is '0'
		// this deals with case like `0.000` or `0.10`
		// to improve cache usage.
		// if len(v) > 2 &&  {
		if strings.Contains(v, ".") { // TODO: optimize this!
			value = strings.TrimRight(v, "0")

			// would need to care about quote
			// trim right bytes
			// last := v[len(v)-1]
			// for len(v) > 0 && (v[len(v)-1] == '0' || v[len(v)-1] == '.') {
			// 	v = v[:len(v)-1]
			// }
			// fast by check how many pos to cut, then only cut by 1 times.
		}
		// }

		// cache hit
		// cache need to support both "123" and 123
		if i, ok := scanCache[v]; ok {
			d.i = i
			return nil
		}
	}

	var dec decimal.Decimal

	if err := dec.Scan(value); err != nil {
		return err
	}

	switch dec.Exponent() {
	case 0:
		i := dec.BigInt().Int64()
		if i < 1000 {
			d.i = i * 100
			return nil
		}
	case -1:
		i := dec.BigInt().Int64()
		if i < 10000 {
			d.i = i * 10
			return nil
		}
	case -2:
		i := dec.BigInt().Int64()
		if i < 100000 {
			d.i = i
			return nil
		}
	default:
		// special case, because a lot are `0.000` => exp = -3
		if dec.IsZero() {
			d.i = 0
			return nil
		}
		// if dec
		// fmt.Printf("scan not optimized val=%s, big=%s, exp=%d, raw=%s\n", dec.String(), dec.BigInt().String(), dec.Exponent(), value)
		d.fallback = &dec
	}

	return nil
}

// arithmatic operations

func (d Decimal) Add(d2 Decimal) Decimal {
	// if result of add is not overflow,
	// we can keep result as optimized format as well.
	// otherwise, we would need to fallback to decimal.Decimal
	if d.fallback == nil && d2.fallback == nil {
		// check overflow
		// based on https://stackoverflow.com/a/33643773
		if d2.i > 0 {
			if d.i <= maxDecimalDigits-d2.i {
				return Decimal{i: d.i + d2.i}
			}
		} else {
			if d.i >= minDecimalDigits-d2.i {
				return Decimal{i: d.i + d2.i}
			}
		}
	}

	result := d.asFallback().Add(d2.asFallback())

	return Decimal{fallback: &result}
}

func (d Decimal) asFallback() decimal.Decimal {
	if d.fallback == nil {
		x := big.NewInt(d.i)
		return decimal.NewFromBigInt(x, -12)
	}
	return *d.fallback
}
