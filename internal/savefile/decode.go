package savefile

// Read direction of encodeFieldValue: decode a param row's integer fields from
// a decompressed BND4 blob, given a schema. Ported from savescan.py's
// decode_row_fields (the proven-correct read implementation).

import (
	"encoding/binary"
	"fmt"
)

// DecodeRowFields decodes the schema's integer fields at dataOffset into a
// name->value map, the exact inverse of encodeFieldValue: little-endian per
// typeWidth, signed types sign-extended, unsigned zero-extended.
//
// dummy8 padding fields are skipped (as savescan.py does). f32/f64 fields are
// skipped too: the only one in these schemas is ShopLineupParam's
// value_Magnification, which is not an integer and has no representation in an
// int64 map — savescan decodes it as a float, and the (one) caller that needs
// it reads it separately. Non-dummy8 array fields are rejected (none exist in
// these schemas; mirrors Apply's array guard).
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
