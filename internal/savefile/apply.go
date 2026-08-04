// Package shopwrite patches merchant shop rows in an Elden Ring PS4 save file.
//
// It edits existing ShopLineupParam rows in place (value edits only, never
// add/remove rows), then fully re-encodes the decompressed BND4 blob as a
// fresh zstd stream. See docs/WRITEBACK.md.
//
// The GUI (internal/ui/gio) calls Apply in-process; cmd/shopwrite wraps it as
// the historical CLI.
package savefile

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"

	"github.com/klauspost/compress/zstd"
)

// Edit is one entry of the edits array (edits.json element in the CLI).
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

// Apply validates every edit, patches the decompressed BND4 blob, re-encodes
// it as a fresh zstd stream, re-encrypts, and writes a new save at outPath.
// Any validation/capacity error returns before anything is written. The
// input file is never modified. Hardcoded to the ShopLineupParam schema --
// this is the corruption-tested, trusted path, kept byte-identical; see
// ApplyWithSchema for any other table.
func Apply(savePath, outPath, entryName string, edits []Edit) (*Summary, error) {
	schema, err := LoadShopSchema()
	if err != nil {
		return nil, err
	}
	return applyWithSchema(savePath, outPath, entryName, edits, schema)
}

// ApplyWithSchema is Apply generalized to any embedded schema/entry pair --
// e.g. an EquipParam* table's sellValue field (see LoadEquipParamSchema),
// not just ShopLineupParam (see docs/MERCHANT_DATA.md's 2026-07-30 "cost=0"
// entry for why a second table is needed at all). Apply itself is kept as a
// byte-identical thin wrapper around the exact same body, so this addition
// can't change Apply's own already-trusted behavior.
func ApplyWithSchema(savePath, outPath, entryName string, edits []Edit, schema *ShopSchema) (*Summary, error) {
	return applyWithSchema(savePath, outPath, entryName, edits, schema)
}

// applyWithSchema runs the 4 steps below in this exact order -- patch,
// verify, buildRegBlob, encryptAndSplice -- with no disk write until the
// very last one. Split into named steps 2026-08-01 for readability; the
// split is pure extraction (no reordering, no logic change) and is
// covered by the same tests (golden, round-trip self-check, in-game-
// verified fixtures) the single-function version was.
func applyWithSchema(savePath, outPath, entryName string, edits []Edit, schema *ShopSchema) (*Summary, error) {
	if savePath == outPath {
		return nil, fmt.Errorf("-out must differ from -save (never write the input path)")
	}
	if len(edits) == 0 {
		return nil, fmt.Errorf("no edits to apply")
	}

	reg, err := LoadRegulation(savePath)
	if err != nil {
		return nil, err
	}

	entry, _, _, rows, err := LoadParamRows(reg.BND4, entryName)
	if err != nil {
		return nil, err
	}

	touchedRowIDs, err := patchRows(reg.BND4, entry, rows, entryName, edits, schema)
	if err != nil {
		return nil, err
	}

	newStream, err := verifyRecompressed(reg.BND4)
	if err != nil {
		return nil, err
	}

	newRegBlob, capacity, err := buildRegBlob(reg, newStream)
	if err != nil {
		return nil, err
	}

	if err := encryptAndSplice(reg, newRegBlob, capacity, outPath); err != nil {
		return nil, err
	}

	return &Summary{
		EntryName:     entryName,
		RowIDs:        touchedRowIDs,
		OrigStreamLen: reg.CompSize,
		NewStreamLen:  len(newStream),
		NewRegBlobLen: len(newRegBlob),
		Capacity:      capacity,
		OutPath:       outPath,
	}, nil
}

// patchRows validates every edit against schema and writes each field's
// encoded bytes directly into bnd4 (mutating it in place) at the target
// row's offset within entry -- step 1, "patch." Nothing is written to
// disk here; on any validation error bnd4 may already be partially
// mutated, but the caller never reaches encryptAndSplice's WriteFile in
// that case, so the input file itself is never touched. Returns the
// touched row IDs in edits order.
func patchRows(bnd4 []byte, entry BND4Entry, rows []ParamRow, entryName string, edits []Edit, schema *ShopSchema) ([]int64, error) {
	rowByID := make(map[int32]ParamRow, len(rows))
	for _, r := range rows {
		rowByID[r.ID] = r
	}

	touchedRowIDs := make([]int64, 0, len(edits))
	for _, ed := range edits {
		if ed.RowID < math.MinInt32 || ed.RowID > math.MaxInt32 {
			return nil, fmt.Errorf("row_id %d out of range for a param row id (int32)", ed.RowID)
		}
		row, ok := rowByID[int32(ed.RowID)]
		if !ok {
			return nil, fmt.Errorf("row_id %d not found in %s", ed.RowID, entryName)
		}
		if len(ed.Fields) == 0 {
			return nil, fmt.Errorf("row_id %d has no fields to edit", ed.RowID)
		}
		rowBase := entry.Offset + row.DataOffset
		for name, val := range ed.Fields {
			field, ok := schema.byName[name]
			if !ok {
				return nil, fmt.Errorf("row_id %d: unknown field %q (not in schema)", ed.RowID, name)
			}
			if field.Type == "dummy8" {
				return nil, fmt.Errorf("row_id %d: field %q is padding (dummy8), not editable", ed.RowID, name)
			}
			if field.ArrayLength != 1 {
				return nil, fmt.Errorf("row_id %d: field %q is an array (length %d); array edits unsupported", ed.RowID, name, field.ArrayLength)
			}
			b, err := encodeFieldValue(field, val)
			if err != nil {
				return nil, fmt.Errorf("row_id %d: %w", ed.RowID, err)
			}
			abs := rowBase + field.Offset
			if abs+len(b) > len(bnd4) {
				return nil, fmt.Errorf("row_id %d field %q write out of bounds", ed.RowID, name)
			}
			copy(bnd4[abs:], b)
		}
		touchedRowIDs = append(touchedRowIDs, ed.RowID)
	}
	return touchedRowIDs, nil
}

// verifyRecompressed re-encodes the patched bnd4 blob as a fresh zstd
// stream (see recompress.go / docs/WRITEBACK.md's "Recompression" section
// for why this replaced the old per-block Raw patch) and round-trip
// verifies it decompresses back to exactly bnd4 -- step 2, "verify."
// Cheap insurance against an encoder/decoder bug -- nothing is written to
// disk without passing this check, same discipline the old block-patch
// code established 2026-07-27 after a real corruption.
func verifyRecompressed(bnd4 []byte) ([]byte, error) {
	newStream, err := buildRecompressedStream(bnd4)
	if err != nil {
		return nil, err
	}
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	roundTrip, derr := dec.DecodeAll(newStream, make([]byte, 0, len(bnd4)))
	if derr != nil {
		return nil, fmt.Errorf("self-check: recompressed stream failed to decompress: %w", derr)
	}
	if bad := firstMismatch(roundTrip, bnd4); bad >= 0 {
		return nil, fmt.Errorf("self-check: recompressed stream decodes to wrong content at decompressed offset %d; refusing to write", bad)
	}
	return newStream, nil
}

// buildRegBlob assembles the new DCX blob (76-byte header with
// compressed_size patched, + the verified stream) and checks it against
// reg's actual encrypted-region capacity, refusing to grow past it --
// step 3, "buildRegBlob."
func buildRegBlob(reg *Regulation, newStream []byte) ([]byte, int, error) {
	newRegBlob := make([]byte, 76+len(newStream))
	copy(newRegBlob, reg.Plaintext[:76])
	binary.BigEndian.PutUint32(newRegBlob[32:36], uint32(len(newStream)))
	copy(newRegBlob[76:], newStream)

	capacity := reg.CiphertextLen // == len(ud11) - 0x10 - 16
	if len(newRegBlob) > capacity {
		over := len(newRegBlob) - capacity
		return nil, 0, fmt.Errorf("patched regulation blob is %d bytes, exceeds capacity %d by %d bytes; refusing to write",
			len(newRegBlob), capacity, over)
	}
	return newRegBlob, capacity, nil
}

// encryptAndSplice re-encrypts newRegBlob (same IV/key, zero-padded to
// capacity), splices it into a full copy of the original file's bytes
// (only IV+ciphertext within UserData11 change -- the unk header at
// [UD11Off:UD11Off+0x10] is left untouched), and writes the result to
// outPath -- step 4, "encryptAndSplice," the only disk write in the whole
// apply path.
func encryptAndSplice(reg *Regulation, newRegBlob []byte, capacity int, outPath string) error {
	if capacity%aes.BlockSize != 0 {
		return fmt.Errorf("ciphertext capacity %d is not AES-block-aligned; unexpected save format", capacity)
	}
	ciphertext, err := encryptRegulation(newRegBlob, reg.IV, capacity)
	if err != nil {
		return err
	}

	result := make([]byte, len(reg.FileBytes))
	copy(result, reg.FileBytes)
	copy(result[reg.UD11Off+ud11UnkHdrSize:], reg.IV)
	copy(result[reg.UD11Off+ud11UnkHdrSize+aesIVSize:], ciphertext)
	return os.WriteFile(outPath, result, 0o644)
}

// firstMismatch returns the first differing byte offset between a and b (a
// length difference counts, at the shorter length), or -1 if identical.
func firstMismatch(a, b []byte) int {
	if bytes.Equal(a, b) {
		return -1
	}
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func encryptRegulation(regBlob, iv []byte, ciphertextLen int) ([]byte, error) {
	plaintext := make([]byte, ciphertextLen)
	copy(plaintext, regBlob) // remainder stays zero (matches original reserved tail)
	block, err := aes.NewCipher(regulationKey)
	if err != nil {
		return nil, err
	}
	out := make([]byte, ciphertextLen)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, plaintext)
	return out, nil
}

// LoadEditsFile reads an edits.json (array of {row_id, fields:{name:value}}).
func LoadEditsFile(path string) ([]Edit, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var edits []Edit
	if err := dec.Decode(&edits); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return edits, nil
}

// Print writes the human-readable summary (identical to the historical CLI
// stderr output).
func (s *Summary) Print(w io.Writer) {
	sortedRows := append([]int64(nil), s.RowIDs...)
	sort.Slice(sortedRows, func(i, j int) bool { return sortedRows[i] < sortedRows[j] })

	origRegBlobLen := 76 + s.OrigStreamLen
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
