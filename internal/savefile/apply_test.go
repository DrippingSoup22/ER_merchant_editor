package savefile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyBaseline re-applies the canned baseline edits to the vanilla
// fixture save. Historically (through the Raw-block-patch era) this asserted
// sha256-identity against a captured pre-refactor baseline; that guarantee
// no longer applies now that the write side fully recompresses the stream
// (2026-08-02, see docs/WRITEBACK.md's "Recompression" section) -- the exact
// output bytes depend on the zstd encoder, not just the edits. What still
// must hold, and what this now checks: every edited field lands correctly,
// every other row is untouched, and the row count doesn't change. Skips when
// the fixture save is absent (save_files/ is gitignored, so CI skips this).
func TestApplyBaseline(t *testing.T) {
	fixture := filepath.Join("..", "..", "save_files", "vanilla_fresh_character.dat")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture save not present, skipping: %v", err)
	}

	edits, err := LoadEditsFile(filepath.Join("testdata", "edits_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	wantByRow := make(map[int64]map[string]json.Number, len(edits))
	for _, e := range edits {
		wantByRow[e.RowID] = e.Fields
	}

	out := filepath.Join(t.TempDir(), "baseline_output.dat")
	summary, err := Apply(fixture, out, "ShopLineupParam.param", edits)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.RowIDs) != len(edits) {
		t.Errorf("summary reports %d rows touched, want %d", len(summary.RowIDs), len(edits))
	}

	before := decodeAllShopRows(t, fixture)
	after := decodeAllShopRows(t, out)
	if len(before) != len(after) {
		t.Fatalf("row count changed: %d -> %d", len(before), len(after))
	}
	for id, b := range before {
		a := after[id]
		want := wantByRow[id]
		for f, bv := range b {
			av := a[f]
			if wv, edited := want[f]; edited {
				n, _ := wv.Int64()
				if av != n {
					t.Errorf("row %d field %s = %d, want %d (edited)", id, f, av, n)
				}
				continue
			}
			if av != bv {
				t.Errorf("row %d field %s corrupted: %d -> %d", id, f, bv, av)
			}
		}
	}
}

// TestApplyRefusesSamePath guards the never-write-the-input rule at the
// library boundary (the CLI has its own copy of the check).
func TestApplyRefusesSamePath(t *testing.T) {
	if _, err := Apply("same.dat", "same.dat", "ShopLineupParam.param", []Edit{{RowID: 1}}); err == nil {
		t.Fatal("Apply accepted -out == -save")
	}
}

// TestApplyWithSchemaRejectsInvalidEdits drives every per-edit validation
// branch in applyWithSchema that a crafted edit + schema can reach, asserting
// each returns its specific error, writes no output file, and leaves the
// fixture save untouched. A custom schema (via ParseSchema) supplies the
// field-shape guards' inputs independent of the real ShopLineupParam field
// names: a normal field, a dummy8 padding field, an array field, and a field
// whose offset lands far past the end of the decompressed blob.
//
// Two later validation branches — the recompressed blob exceeding the file's
// ciphertext capacity ("exceeds capacity ... refusing to write") and the
// capacity not being AES-block-aligned ("not AES-block-aligned") — run AFTER
// full recompression and depend on the save's own container geometry, not on
// any edit or schema. A value-only edit cannot inflate the blob past capacity,
// and the fixture is aligned by construction, so neither is reachable without
// fabricating a malformed save; they are intentionally not exercised here.
func TestApplyWithSchemaRejectsInvalidEdits(t *testing.T) {
	fixture := filepath.Join("..", "..", "save_files", "vanilla_fresh_character.dat")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture save not present, skipping: %v", err)
	}
	fixtureBefore, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}

	schema, err := ParseSchema([]byte(`{
		"param_type":"TEST","row_size":128,
		"fields":[
			{"name":"okval","type":"u32","array_length":1,"offset":0,"size":4},
			{"name":"pad","type":"dummy8","array_length":1,"offset":8,"size":1},
			{"name":"arr","type":"u8","array_length":4,"offset":12,"size":4},
			{"name":"farfield","type":"u32","array_length":1,"offset":2000000000,"size":4}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}

	const realRow = 100501 // a row that exists in ShopLineupParam
	const entry = "ShopLineupParam.param"

	cases := []struct {
		name    string
		row     int64
		fields  map[string]json.Number
		wantErr string
	}{
		{"row not found", 999999999, map[string]json.Number{"okval": "1"}, "not found"},
		{"row_id exceeds int32", 1 << 40, map[string]json.Number{"okval": "1"}, "out of range for a param row id"},
		{"row_id below int32 min", -(1 << 40), map[string]json.Number{"okval": "1"}, "out of range for a param row id"},
		{"no fields", realRow, map[string]json.Number{}, "no fields to edit"},
		{"unknown field", realRow, map[string]json.Number{"nonesuch": "1"}, "unknown field"},
		{"dummy8 field", realRow, map[string]json.Number{"pad": "1"}, "padding (dummy8)"},
		{"array field", realRow, map[string]json.Number{"arr": "1"}, "is an array"},
		{"value out of type range", realRow, map[string]json.Number{"okval": "99999999999"}, "out of range"},
		{"write out of bounds", realRow, map[string]json.Number{"farfield": "1"}, "out of bounds"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "should_not_exist.dat")
			_, err := ApplyWithSchema(fixture, out, entry, []Edit{{RowID: tc.row, Fields: tc.fields}}, schema)
			if err == nil {
				t.Fatalf("ApplyWithSchema accepted an invalid edit (%s)", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
			if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
				t.Errorf("output file was created despite validation failure (stat err: %v)", statErr)
			}
		})
	}

	fixtureAfter, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fixtureBefore, fixtureAfter) {
		t.Error("fixture save was modified by rejected edits")
	}
}

// decodeAllShopRows decodes every ShopLineupParam row of a save into
// row_id -> field name -> value, for before/after comparison in tests.
func decodeAllShopRows(t *testing.T, path string) map[int64]map[string]int64 {
	t.Helper()
	schema, err := LoadShopSchema()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := LoadRegulation(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := FindBND4Entry(reg.BND4, "ShopLineupParam.param")
	if err != nil {
		t.Fatal(err)
	}
	blob := reg.BND4[entry.Offset : entry.Offset+entry.Size]
	header, err := ParseParamHeader(blob)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := IterParamRows(blob, header)
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[int64]map[string]int64, len(rows))
	for _, r := range rows {
		fields, err := DecodeRowFields(blob, r.DataOffset, schema)
		if err != nil {
			t.Fatalf("row %d: %v", r.ID, err)
		}
		m[int64(r.ID)] = fields
	}
	return m
}

// TestApplyManyDistinctRowsInOneWrite is the regression test for the
// 2026-08-02 recompression switch (see docs/WRITEBACK.md): the old
// Raw-block-patch write path paid a fixed ~55-65KB growth tax per distinct
// 64KB block touched, capping any single write at ~5-6 touched blocks (the
// fixed ~323KB capacity slack) -- far short of "edit all of a merchant's
// stock." Edits every Twin Maiden Husks row (93 rows, spanning many distinct
// items) to price 0 in one Apply call and confirms it succeeds and every
// edited row landed correctly -- this exact scenario overflowed capacity
// under the old write path.
func TestApplyManyDistinctRowsInOneWrite(t *testing.T) {
	fixture := filepath.Join("..", "..", "save_files", "vanilla_fresh_character.dat")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture save not present, skipping: %v", err)
	}

	before := decodeAllShopRows(t, fixture)
	var edits []Edit
	for id := range before {
		if id < 101800 || id > 101899 { // Twin Maiden Husks' contiguous row range
			continue
		}
		edits = append(edits, Edit{RowID: id, Fields: map[string]json.Number{"value": "0"}})
	}
	if len(edits) < 50 {
		t.Fatalf("expected TMH's ~93 rows, found only %d -- fixture or row range changed?", len(edits))
	}

	out := filepath.Join(t.TempDir(), "many_rows.dat")
	summary, err := Apply(fixture, out, "ShopLineupParam.param", edits)
	if err != nil {
		t.Fatalf("Apply with %d distinct rows: %v", len(edits), err)
	}
	if summary.NewRegBlobLen > summary.Capacity {
		t.Errorf("new regulation blob (%d) exceeds capacity (%d)", summary.NewRegBlobLen, summary.Capacity)
	}

	after := decodeAllShopRows(t, out)
	if len(before) != len(after) {
		t.Fatalf("row count changed: %d -> %d", len(before), len(after))
	}
	edited := make(map[int64]bool, len(edits))
	for _, e := range edits {
		edited[e.RowID] = true
	}
	for id, b := range before {
		a := after[id]
		for f, bv := range b {
			av := a[f]
			if edited[id] && f == "value" {
				if av != 0 {
					t.Errorf("row %d value = %d, want 0", id, av)
				}
				continue
			}
			if av != bv {
				t.Errorf("row %d field %s corrupted: %d -> %d", id, f, bv, av)
			}
		}
	}
}

// TestApplySingleBlockEditRoundTrips reproduces the 2026-07-27 GUI corruption
// (a single-row edit under the old Raw-block-patch path could silently
// corrupt a neighboring block's rows via a stale LZ back-reference). Full
// recompression (2026-08-02) has no block concept and can't reproduce that
// specific bug class, but this stays as a basic single-edit correctness
// check: the written save decodes to exactly the input rows plus the
// intended edit, across ALL rows.
func TestApplySingleBlockEditRoundTrips(t *testing.T) {
	fixture := filepath.Join("..", "..", "save_files", "vanilla_fresh_character.dat")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture save not present, skipping: %v", err)
	}

	edits := []Edit{{RowID: 100501, Fields: map[string]json.Number{"value": "999"}}}
	out := filepath.Join(t.TempDir(), "single_block.dat")
	if _, err := Apply(fixture, out, "ShopLineupParam.param", edits); err != nil {
		t.Fatal(err)
	}

	before := decodeAllShopRows(t, fixture)
	after := decodeAllShopRows(t, out)
	if len(before) != len(after) {
		t.Fatalf("row count changed: %d -> %d", len(before), len(after))
	}
	for id, b := range before {
		a := after[id]
		for f, bv := range b {
			av := a[f]
			if id == 100501 && f == "value" {
				if av != 999 {
					t.Errorf("row 100501 value = %d, want 999", av)
				}
				continue
			}
			if av != bv {
				t.Errorf("row %d field %s corrupted: %d -> %d", id, f, bv, av)
			}
		}
	}
}

// TestApplyWithSchemaIdenticalToApply guards the Apply/applyWithSchema
// split (2026-07-30, needed to write a second param table for the cost=0
// fix, see docs/MERCHANT_DATA.md): Apply must stay a byte-identical thin
// wrapper -- ApplyWithSchema given the exact same ShopLineupParam schema
// must produce the exact same output as Apply for the same edits.
func TestApplyWithSchemaIdenticalToApply(t *testing.T) {
	fixture := filepath.Join("..", "..", "save_files", "vanilla_fresh_character.dat")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture save not present, skipping: %v", err)
	}

	edits := []Edit{{RowID: 100501, Fields: map[string]json.Number{"value": "777"}}}

	outA := filepath.Join(t.TempDir(), "via_apply.dat")
	if _, err := Apply(fixture, outA, "ShopLineupParam.param", edits); err != nil {
		t.Fatal(err)
	}
	schema, err := LoadShopSchema()
	if err != nil {
		t.Fatal(err)
	}
	outB := filepath.Join(t.TempDir(), "via_apply_with_schema.dat")
	if _, err := ApplyWithSchema(fixture, outB, "ShopLineupParam.param", edits, schema); err != nil {
		t.Fatal(err)
	}

	gotA, err := os.ReadFile(outA)
	if err != nil {
		t.Fatal(err)
	}
	gotB, err := os.ReadFile(outB)
	if err != nil {
		t.Fatal(err)
	}
	sumA, sumB := sha256.Sum256(gotA), sha256.Sum256(gotB)
	if sumA != sumB {
		t.Fatalf("Apply and ApplyWithSchema(shopSchema) produced different output: %s vs %s",
			hex.EncodeToString(sumA[:]), hex.EncodeToString(sumB[:]))
	}
}

// TestApplyWithSchemaWritesEquipParamSellValue covers the actual new
// capability: writing a field on an EquipParam* table, a completely
// different table/row-id-space from ShopLineupParam. Edits Rune Arc's own
// EquipParamGoods row (equipId 190, see docs/MERCHANT_DATA.md) and decodes
// it back to confirm the byte landed at the right offset.
func TestApplyWithSchemaWritesEquipParamSellValue(t *testing.T) {
	fixture := filepath.Join("..", "..", "save_files", "vanilla_fresh_character.dat")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture save not present, skipping: %v", err)
	}

	schema, err := LoadEquipParamSchema(3) // Goods
	if err != nil {
		t.Fatal(err)
	}
	entryName, ok := EquipParamEntryName(3)
	if !ok {
		t.Fatal("no entry name for equipType 3")
	}

	out := filepath.Join(t.TempDir(), "sellvalue.dat")
	edits := []Edit{{RowID: 190, Fields: map[string]json.Number{"sellValue": "-1"}}}
	if _, err := ApplyWithSchema(fixture, out, entryName, edits, schema); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadRegulation(out)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := FindBND4Entry(reg.BND4, entryName)
	if err != nil {
		t.Fatal(err)
	}
	blob := reg.BND4[entry.Offset : entry.Offset+entry.Size]
	header, err := ParseParamHeader(blob)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := IterParamRows(blob, header)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range rows {
		if r.ID != 190 {
			continue
		}
		found = true
		fields, err := DecodeRowFields(blob, r.DataOffset, schema)
		if err != nil {
			t.Fatal(err)
		}
		if fields["sellValue"] != -1 {
			t.Errorf("EquipParamGoods row 190 sellValue = %d, want -1", fields["sellValue"])
		}
	}
	if !found {
		t.Fatal("EquipParamGoods row 190 not found in output")
	}
}
