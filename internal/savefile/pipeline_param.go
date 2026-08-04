package savefile

// Inner PARAM header + row table (ported from savescan.py), plus
// encodeFieldValue, the write direction of decode.go's DecodeRowFields.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

const (
	fmt1Flag01          = 0b0000_0001
	fmt1IntDataOffset   = 0b0000_0010
	fmt1LongDataOffset  = 0b0000_0100
	fmt1OffsetParamType = 0b1000_0000
)

type ParamHeader struct {
	RowCount       int
	LongDataOffset bool
	RowTableOffset int
	ParamType      string
}

func ParseParamHeader(blob []byte) (*ParamHeader, error) {
	if len(blob) < 0x40 {
		return nil, fmt.Errorf("param file too small (%d bytes)", len(blob))
	}
	format2d := blob[0x2D]
	longDataOffset := format2d&fmt1LongDataOffset != 0
	intDataOffsetExpanded := format2d&fmt1Flag01 != 0 && format2d&fmt1IntDataOffset != 0

	pos := 0
	pos += 4 // strings_offset (u32) - unused here
	pos += 2 // reserved int16
	pos += 2 // unk06 (s16)
	pos += 2 // paramdef_data_version (s16)
	rowCount := int(binary.LittleEndian.Uint16(blob[pos : pos+2]))
	pos += 2

	var paramType string
	if format2d&fmt1OffsetParamType != 0 {
		pos += 4                                                                     // assert int32(0)
		paramTypeOffset := int(int64(binary.LittleEndian.Uint64(blob[pos : pos+8]))) // real source reads Int64
		pos += 8
		pos += 0x14
		if paramTypeOffset != 0 {
			paramType = readASCIICStr(blob, paramTypeOffset)
		}
	} else {
		raw := blob[pos : pos+0x20]
		if i := bytes.IndexByte(raw, 0); i >= 0 {
			raw = raw[:i]
		}
		paramType = string(raw)
		pos += 0x20
	}

	pos += 4 // Format quartet (already read by absolute offset above)

	if intDataOffsetExpanded {
		pos += 16 // data start (int32) + 3x assert-zero int32
	} else if longDataOffset {
		pos += 16 // data start (int64) + assert-zero int64
	}

	return &ParamHeader{
		RowCount:       rowCount,
		LongDataOffset: longDataOffset,
		RowTableOffset: pos,
		ParamType:      paramType,
	}, nil
}

func readASCIICStr(blob []byte, offset int) string {
	end := offset
	for end < len(blob) && blob[end] != 0 {
		end++
	}
	return string(blob[offset:end])
}

// ParamRow is (id, data_offset) where DataOffset is relative to the param file.
type ParamRow struct {
	ID         int32
	DataOffset int
}

func IterParamRows(blob []byte, h *ParamHeader) ([]ParamRow, error) {
	entrySize := 12
	if h.LongDataOffset {
		entrySize = 24
	}
	rows := make([]ParamRow, 0, h.RowCount)
	for i := 0; i < h.RowCount; i++ {
		base := h.RowTableOffset + i*entrySize
		if base+entrySize > len(blob) {
			return nil, fmt.Errorf("row %d entry out of bounds", i)
		}
		id := int32(binary.LittleEndian.Uint32(blob[base : base+4]))
		var dataOffset int
		if h.LongDataOffset {
			dataOffset = int(int64(binary.LittleEndian.Uint64(blob[base+8 : base+16]))) // real source reads Int64
		} else {
			dataOffset = int(binary.LittleEndian.Uint32(blob[base+4 : base+8]))
		}
		rows = append(rows, ParamRow{ID: id, DataOffset: dataOffset})
	}
	return rows, nil
}

// LoadParamRows runs the standard 3-step decode for a named param table within
// a decompressed BND4 blob: FindBND4Entry -> ParseParamHeader -> IterParamRows.
// entry (for entry.Offset within bnd4) and paramData (the entry's byte slice,
// aliasing bnd4's backing array) are returned alongside header/rows so callers
// that need those intermediates -- the write path needs entry.Offset; readers
// slice fields out of paramData -- don't have to re-derive them.
func LoadParamRows(bnd4 []byte, entryName string) (entry BND4Entry, paramData []byte, header *ParamHeader, rows []ParamRow, err error) {
	entry, err = FindBND4Entry(bnd4, entryName)
	if err != nil {
		return
	}
	paramData = bnd4[entry.Offset : entry.Offset+entry.Size]
	header, err = ParseParamHeader(paramData)
	if err != nil {
		return
	}
	rows, err = IterParamRows(paramData, header)
	return
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
