package alpacadecimal

import (
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"math/big"
)

// optimized:
// UnmarshalJSON implements the json.Unmarshaler interface.
func (d *Decimal) UnmarshalJSON(decimalBytes []byte) error {
	if string(decimalBytes) == "null" {
		return nil
	}
	v := decimalBytes
	if len(v) > 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	if fixed, ok := parseFixed(v); ok {
		d.fixed = fixed
		d.fallback = nil
		return nil
	}
	dec, err := parseSlow(string(v))
	*d = dec
	if err != nil {
		return fmt.Errorf("error decoding string '%s': %s", v, err)
	}
	return nil
}

// optimized:
// MarshalJSON implements the json.Marshaler interface.
func (d Decimal) MarshalJSON() ([]byte, error) {
	var str string
	if MarshalJSONWithoutQuotes {
		str = d.String()
	} else {
		str = "\"" + d.String() + "\""
	}
	return []byte(str), nil
}

// optimized:
// UnmarshalText implements the encoding.TextUnmarshaler interface for XML
// deserialization.
func (d *Decimal) UnmarshalText(text []byte) error {
	if fixed, ok := parseFixed(text); ok {
		d.fixed = fixed
		d.fallback = nil
		return nil
	}
	dec, err := parseSlow(string(text))
	*d = dec
	if err != nil {
		return fmt.Errorf("error decoding string '%s': %s", text, err)
	}
	return nil
}

// optimized:
// MarshalText implements the encoding.TextMarshaler interface for XML
// serialization.
func (d Decimal) MarshalText() (text []byte, err error) {
	if d.fallback == nil {
		if d.fixed <= a1000InFixed && d.fixed >= aNeg1000InFixed && d.fixed%aCentInFixed == 0 {
			return []byte(stringCache[d.fixed/aCentInFixed+cacheOffset]), nil
		}
		return appendFixed(make([]byte, 0, 21), d.fixed), nil
	}
	return []byte(formatFallback(*d.fallback)), nil
}

// optimized:
// MarshalBinary implements the encoding.BinaryMarshaler interface.
// The format is identical to shopspring/decimal's: a big-endian int32 exponent
// followed by the gob encoding of the big.Int coefficient, so data is
// interchangeable with shopspring and with previous alpacadecimal versions.
func (d Decimal) MarshalBinary() (data []byte, err error) {
	if d.fallback == nil {
		return appendBinaryParts(make([]byte, 0, 14), -precision, d.fixed < 0, 0, uabs(d.fixed)), nil
	}
	neg, hi, lo, prec, ok := d.fallback.ToHiLo()
	if ok {
		return appendBinaryParts(make([]byte, 0, 22), -int32(prec), neg, hi, lo), nil
	}
	// >128-bit coefficient: delegate to big.Int's encoder.
	coef, exp := d.toBigParts()
	valueData, err := coef.GobEncode()
	if err != nil {
		return nil, err
	}
	expData := make([]byte, 4, len(valueData)+4)
	binary.BigEndian.PutUint32(expData, uint32(exp))
	return append(expData, valueData...), nil
}

// appendBinaryParts writes the shopspring binary format: 4-byte big-endian
// exponent, then big.Int gob: 1 byte (version<<1 | sign) + minimal big-endian
// magnitude bytes.
func appendBinaryParts(b []byte, exp int32, neg bool, hi, lo uint64) []byte {
	var ebuf [4]byte
	binary.BigEndian.PutUint32(ebuf[:], uint32(exp))
	b = append(b, ebuf[:]...)
	gobByte := byte(1 << 1) // big.Int gob version 1
	if neg {
		gobByte |= 1
	}
	b = append(b, gobByte)
	var mag [16]byte
	binary.BigEndian.PutUint64(mag[0:8], hi)
	binary.BigEndian.PutUint64(mag[8:16], lo)
	i := 0
	for i < 15 && mag[i] == 0 {
		i++
	}
	if hi == 0 && lo == 0 {
		// zero encodes with no magnitude bytes
		return b
	}
	return append(b, mag[i:]...)
}

// optimized:
// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface.
// It accepts the shopspring/decimal binary format.
func (d *Decimal) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("error decoding binary %v: expected at least 4 bytes, got %d", data, len(data))
	}
	exp := int32(binary.BigEndian.Uint32(data[:4]))
	payload := data[4:]
	if len(payload) == 0 {
		return fmt.Errorf("error decoding binary %v: Int.GobDecode: no data", data)
	}
	version := payload[0] >> 1
	if version != 1 {
		return fmt.Errorf("error decoding binary %v: Int.GobDecode: encoding version %d not supported", data, version)
	}
	neg := payload[0]&1 != 0
	mag := payload[1:]
	if len(mag) <= 8 {
		var lo uint64
		for _, c := range mag {
			lo = lo<<8 | uint64(c)
		}
		if lo <= 1<<63-1 {
			*d = newFromInt64Exp(signed64(lo, neg), int64(exp))
			return nil
		}
	}
	bi := new(big.Int).SetBytes(mag)
	if neg {
		bi.Neg(bi)
	}
	*d = decimalFromBigParts(bi, int64(exp))
	return nil
}

// optimized:
// GobEncode implements the gob.GobEncoder interface for gob serialization.
func (d Decimal) GobEncode() ([]byte, error) {
	return d.MarshalBinary()
}

// optimized:
// GobDecode implements the gob.GobDecoder interface for gob serialization.
func (d *Decimal) GobDecode(data []byte) error {
	return d.UnmarshalBinary(data)
}

// optimized:
// Scan implements the sql.Scanner interface for database deserialization.
func (d *Decimal) Scan(value interface{}) error {
	switch v := value.(type) {
	case []byte:
		if fixed, ok := parseFixed(v); ok {
			d.fixed = fixed
			d.fallback = nil
			return nil
		}
		return d.scanString(string(v))
	case string:
		if fixed, ok := parseFixed(v); ok {
			d.fixed = fixed
			d.fallback = nil
			return nil
		}
		return d.scanString(v)
	case float32:
		// shopspring widens float32 to float64 when scanning
		*d = NewFromFloat(float64(v))
		return nil
	case float64:
		// numeric in sqlite3 sends us float64
		*d = NewFromFloat(v)
		return nil
	case int64:
		// at least in sqlite3 when the value is 0 in db, the data is sent
		// to us as an int64 instead of a float64
		*d = NewFromInt(v)
		return nil
	case uint64:
		// while clickhouse may send 0 in db as uint64
		*d = NewFromUint64(v)
		return nil
	}
	// default: interpret the value stored as a (possibly quoted) string
	str, err := unquoteIfQuoted(value)
	if err != nil {
		return err
	}
	dec, err := NewFromString(str)
	*d = dec
	return err
}

// scanString handles the non-fixed string/[]byte Scan path; like shopspring,
// surrounding double quotes are stripped.
func (d *Decimal) scanString(str string) error {
	if len(str) > 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
		if fixed, ok := parseFixed(str); ok {
			d.fixed = fixed
			d.fallback = nil
			return nil
		}
	}
	dec, err := parseSlow(str)
	*d = dec
	return err
}

// NullDecimal represents a nullable decimal with compatibility for
// scanning null values from the database.
type NullDecimal struct {
	Decimal Decimal
	Valid   bool
}

func NewNullDecimal(d Decimal) NullDecimal {
	return NullDecimal{
		Decimal: d,
		Valid:   true,
	}
}

// Scan implements the sql.Scanner interface for database deserialization.
func (d *NullDecimal) Scan(value interface{}) error {
	if value == nil {
		d.Valid = false
		return nil
	}
	d.Valid = true
	return d.Decimal.Scan(value)
}

// Value implements the driver.Valuer interface for database serialization.
func (d NullDecimal) Value() (driver.Value, error) {
	if !d.Valid {
		return nil, nil
	}
	return d.Decimal.Value()
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (d *NullDecimal) UnmarshalJSON(decimalBytes []byte) error {
	if string(decimalBytes) == "null" {
		d.Valid = false
		return nil
	}
	d.Valid = true
	return d.Decimal.UnmarshalJSON(decimalBytes)
}

// MarshalJSON implements the json.Marshaler interface.
func (d NullDecimal) MarshalJSON() ([]byte, error) {
	if !d.Valid {
		return []byte("null"), nil
	}
	return d.Decimal.MarshalJSON()
}

// UnmarshalText implements the encoding.TextUnmarshaler interface for XML
// deserialization
func (d *NullDecimal) UnmarshalText(text []byte) error {
	str := string(text)

	// check for empty XML or XML without body e.g., <tag></tag>
	if str == "" {
		d.Valid = false
		return nil
	}

	if err := d.Decimal.UnmarshalText(text); err != nil {
		d.Valid = false
		return err
	}

	d.Valid = true
	return nil
}

// MarshalText implements the encoding.TextMarshaler interface for XML
// serialization.
func (d NullDecimal) MarshalText() (text []byte, err error) {
	if !d.Valid {
		return []byte{}, nil
	}
	return d.Decimal.MarshalText()
}
