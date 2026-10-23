package savefile

// A row's fields, read and written through an embedded schema.

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// DecodeRowFields decodes the schema's integer fields at dataOffset into a
// name->value map, the exact inverse of encodeFieldValue: little-endian per
// typeWidth, signed types sign-extended, unsigned zero-extended.
//
// dummy8 padding fields are skipped, and so are f32/f64 fields: the only one
// in these schemas is ShopLineupParam's value_Magnification, which an int64
// map cannot hold; the one caller that needs it reads it separately.
// Non-dummy8 array fields are rejected (none exist in these schemas; mirrors
// Apply's array guard).
func DecodeRowFields(blob []byte, dataOffset int, schema *ShopSchema) (map[string]int64, error) {
	out := make(map[string]int64, len(schema.Fields))
	for _, f := range schema.Fields {
		if f.Type == "dummy8" {
			continue
		}
		if f.ArrayLength != 1 {
			return nil, fmt.Errorf("field %q is an array (length %d); array decode unsupported", f.Name, f.ArrayLength)
		}
		w, ok := typeWidth[f.Type]
		if !ok {
			return nil, fmt.Errorf("field %q has unsupported type %q", f.Name, f.Type)
		}
		if f.Type == "f32" || f.Type == "f64" {
			continue // float field (value_Magnification); not representable as int64
		}
		off := dataOffset + f.Offset
		if off < 0 || off+w > len(blob) {
			return nil, fmt.Errorf("field %q read out of bounds", f.Name)
		}
		var v int64
		switch f.Type {
		case "s8":
			v = int64(int8(blob[off]))
		case "u8":
			v = int64(blob[off])
		case "s16":
			v = int64(int16(binary.LittleEndian.Uint16(blob[off:])))
		case "u16":
			v = int64(binary.LittleEndian.Uint16(blob[off:]))
		case "s32", "b32":
			v = int64(int32(binary.LittleEndian.Uint32(blob[off:])))
		case "u32":
			v = int64(binary.LittleEndian.Uint32(blob[off:]))
		default:
			return nil, fmt.Errorf("field %q has unsupported integer type %q", f.Name, f.Type)
		}
		out[f.Name] = v
	}
	return out, nil
}

// encodeFieldValue converts a JSON number to the correct little-endian bytes for
// the field's declared type, rejecting values that do not fit the type width.
func encodeFieldValue(f SchemaField, num json.Number) ([]byte, error) {
	w, ok := typeWidth[f.Type]
	if !ok {
		return nil, fmt.Errorf("field %q has unsupported type %q", f.Name, f.Type)
	}
	buf := make([]byte, w)

	switch f.Type {
	case "f32":
		fv, err := num.Float64()
		if err != nil {
			return nil, fmt.Errorf("field %q: %q is not a valid float: %w", f.Name, num, err)
		}
		binary.LittleEndian.PutUint32(buf, math.Float32bits(float32(fv)))
	case "f64":
		fv, err := num.Float64()
		if err != nil {
			return nil, fmt.Errorf("field %q: %q is not a valid float: %w", f.Name, num, err)
		}
		binary.LittleEndian.PutUint64(buf, math.Float64bits(fv))
	default: // integer types
		iv, err := num.Int64()
		if err != nil {
			return nil, fmt.Errorf("field %q: %q is not a valid integer for type %s: %w", f.Name, num, f.Type, err)
		}
		lo, hi, err := intRange(f.Type)
		if err != nil {
			return nil, err
		}
		if iv < lo || iv > hi {
			return nil, fmt.Errorf("field %q value %d out of range for %s (%d..%d)", f.Name, iv, f.Type, lo, hi)
		}
		switch w {
		case 1:
			buf[0] = byte(iv)
		case 2:
			binary.LittleEndian.PutUint16(buf, uint16(iv))
		case 4:
			binary.LittleEndian.PutUint32(buf, uint32(iv))
		}
	}
	return buf, nil
}

func intRange(t string) (int64, int64, error) {
	switch t {
	case "s8":
		return -128, 127, nil
	case "u8":
		return 0, 255, nil
	case "s16":
		return -32768, 32767, nil
	case "u16":
		return 0, 65535, nil
	case "s32", "b32":
		return math.MinInt32, math.MaxInt32, nil
	case "u32":
		return 0, math.MaxUint32, nil
	}
	return 0, 0, fmt.Errorf("type %q is not an integer type", t)
}
