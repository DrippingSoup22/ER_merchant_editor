// Package savefile writes merchant edits into a save, PS4 or PC: value edits
// to existing rows of ShopLineupParam or an EquipParam table, never added or
// removed rows. Reading, rebuilding and writing the regulation is ER_shared's;
// this package owns only the edits. See docs/DESIGN.md (Saving).
package savefile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"

	reg "github.com/DrippingSoup22/ER_shared/regulation"
)

// Edit is one row's new field values, as an edits.json element.
type Edit struct {
	RowID  int64                  `json:"row_id"`
	Fields map[string]json.Number `json:"fields"`
}

// Summary reports what a successful Apply did.
type Summary struct {
	EntryName     string
	RowIDs        []int64
	OrigStreamLen int
	NewStreamLen  int
	NewRegBlobLen int
	Capacity      int
	OutPath       string
}

// dcxHeaderSize is the DCX header in front of the zstd stream; the summary
// reports the regulation blob's size with it.
const dcxHeaderSize = 76

// Apply edits ShopLineupParam rows and writes a new save at outPath.
func Apply(savePath, outPath, entryName string, edits []Edit) (*Summary, error) {
	schema, err := LoadShopSchema()
	if err != nil {
		return nil, err
	}
	return ApplyWithSchema(savePath, outPath, entryName, edits, schema)
}

// ApplyWithSchema edits the rows of any table an embedded schema describes
// and writes a new save at outPath. Every edit is validated before a byte is
// written; the output may replace an existing file the user chose, never the
// input, and is reopened and checked before it takes the output's name.
func ApplyWithSchema(savePath, outPath, entryName string, edits []Edit, schema *ShopSchema) (*Summary, error) {
	if len(edits) == 0 {
		return nil, fmt.Errorf("no edits to apply")
	}
	regulation, err := reg.Load(savePath)
	if err != nil {
		return nil, err
	}
	touched, err := patchRows(regulation.BND4, entryName, edits, schema)
	if err != nil {
		return nil, err
	}
	output, written, err := reg.Build(regulation)
	if err != nil {
		return nil, err
	}
	err = reg.ReplaceAtomicValidated(savePath, outPath, output, func(candidate string) error {
		reopened, err := reg.Load(candidate)
		if err != nil {
			return err
		}
		if !bytes.Equal(reopened.BND4, regulation.BND4) {
			return fmt.Errorf("the written save reads back different rows")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Summary{
		EntryName:     entryName,
		RowIDs:        touched,
		OrigStreamLen: written.OriginalStreamSize,
		NewStreamLen:  written.NewStreamSize,
		NewRegBlobLen: dcxHeaderSize + written.NewStreamSize,
		Capacity:      written.Capacity,
		OutPath:       outPath,
	}, nil
}

// patchRows validates every edit against the schema and writes its fields
// into bnd4 in place. It returns the edited row IDs in edit order.
func patchRows(bnd4 []byte, entryName string, edits []Edit, schema *ShopSchema) ([]int64, error) {
	entry, _, _, rows, err := reg.LoadParamRows(bnd4, entryName)
	if err != nil {
		return nil, err
	}
	rowByID := make(map[int32]reg.ParamRow, len(rows))
	for _, row := range rows {
		rowByID[row.ID] = row
	}

	touched := make([]int64, 0, len(edits))
	for _, edit := range edits {
		if edit.RowID < math.MinInt32 || edit.RowID > math.MaxInt32 {
			return nil, fmt.Errorf("row_id %d out of range for a param row id (int32)", edit.RowID)
		}
		row, ok := rowByID[int32(edit.RowID)]
		if !ok {
			return nil, fmt.Errorf("row_id %d not found in %s", edit.RowID, entryName)
		}
		if len(edit.Fields) == 0 {
			return nil, fmt.Errorf("row_id %d has no fields to edit", edit.RowID)
		}
		rowBase := entry.Offset + row.DataOffset
		for name, value := range edit.Fields {
			field, ok := schema.byName[name]
			if !ok {
				return nil, fmt.Errorf("row_id %d: unknown field %q (not in schema)", edit.RowID, name)
			}
			if field.Type == "dummy8" {
				return nil, fmt.Errorf("row_id %d: field %q is padding (dummy8), not editable", edit.RowID, name)
			}
			if field.ArrayLength != 1 {
				return nil, fmt.Errorf("row_id %d: field %q is an array (length %d); array edits unsupported", edit.RowID, name, field.ArrayLength)
			}
			encoded, err := encodeFieldValue(field, value)
			if err != nil {
				return nil, fmt.Errorf("row_id %d: %w", edit.RowID, err)
			}
			at := rowBase + field.Offset
			if at+len(encoded) > entry.Offset+entry.Size {
				return nil, fmt.Errorf("row_id %d field %q write out of bounds", edit.RowID, name)
			}
			copy(bnd4[at:], encoded)
		}
		touched = append(touched, edit.RowID)
	}
	return touched, nil
}

// LoadEditsFile reads an edits.json (array of {row_id, fields:{name:value}}).
func LoadEditsFile(path string) ([]Edit, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var edits []Edit
	if err := decoder.Decode(&edits); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return edits, nil
}

// Print writes the human-readable summary the CLI prints.
func (s *Summary) Print(w io.Writer) {
	sortedRows := append([]int64(nil), s.RowIDs...)
	sort.Slice(sortedRows, func(i, j int) bool { return sortedRows[i] < sortedRows[j] })

	origRegBlobLen := dcxHeaderSize + s.OrigStreamLen
	growth := s.NewRegBlobLen - origRegBlobLen
	slackBefore := s.Capacity - origRegBlobLen
	slackRemaining := s.Capacity - s.NewRegBlobLen

	fmt.Fprintf(w, "entry:            %s\n", s.EntryName)
	fmt.Fprintf(w, "rows touched:     %d %v\n", len(s.RowIDs), sortedRows)
	fmt.Fprintf(w, "orig stream len:  %d bytes\n", s.OrigStreamLen)
	fmt.Fprintf(w, "new stream len:   %d bytes\n", s.NewStreamLen)
	fmt.Fprintf(w, "orig regBlob len: %d bytes\n", origRegBlobLen)
	fmt.Fprintf(w, "new regBlob len:  %d bytes\n", s.NewRegBlobLen)
	fmt.Fprintf(w, "capacity:         %d bytes\n", s.Capacity)
	fmt.Fprintf(w, "growth used:      %d bytes (slack %d -> %d remaining of %d)\n",
		growth, slackBefore, slackRemaining, s.Capacity)
	fmt.Fprintf(w, "wrote:            %s\n", s.OutPath)
}
