package savefile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	reg "github.com/DrippingSoup22/ER_shared/regulation"
)

// The private saves, one per platform.
const (
	ps4Fixture = "vanilla_fresh_character.dat"
	pcFixture  = "pc_fixture.sl2"
	shopEntry  = "ShopLineupParam.param"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "save_files", name)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	return path
}

// decodeRows is every row of one table: row ID -> field -> value.
func decodeRows(t *testing.T, path, entry string, schema *ShopSchema) map[int64]map[string]int64 {
	t.Helper()
	loaded, err := reg.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	_, blob, _, rows, err := reg.LoadParamRows(loaded.BND4, entry)
	if err != nil {
		t.Fatal(err)
	}
	decoded := make(map[int64]map[string]int64, len(rows))
	for _, row := range rows {
		if decoded[int64(row.ID)], err = DecodeRowFields(blob, row.DataOffset, schema); err != nil {
			t.Fatalf("row %d: %v", row.ID, err)
		}
	}
	return decoded
}

// The canned edits plus every Twin Maiden Husks row priced at 0 - far more
// rows than the old block-patch writer could fit - on both platforms: the
// edited fields land, no other field moves, and nothing before the regulation
// changes.
func TestApplyEditsLandAndNothingElseMoves(t *testing.T) {
	schema, err := LoadShopSchema()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{ps4Fixture, pcFixture} {
		in := fixture(t, name)
		before := decodeRows(t, in, shopEntry, schema)
		edits, err := LoadEditsFile(filepath.Join("testdata", "edits_baseline.json"))
		if err != nil {
			t.Fatal(err)
		}
		for id := range before {
			if id >= 101800 && id <= 101899 {
				edits = append(edits, Edit{RowID: id, Fields: map[string]json.Number{"value": "0"}})
			}
		}
		want := map[int64]map[string]int64{}
		for _, edit := range edits {
			if want[edit.RowID] == nil {
				want[edit.RowID] = map[string]int64{}
			}
			for field, value := range edit.Fields {
				want[edit.RowID][field], _ = value.Int64()
			}
		}

		out := filepath.Join(t.TempDir(), "out")
		summary, err := Apply(in, out, shopEntry, edits)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(summary.RowIDs) != len(edits) {
			t.Errorf("%s: %d rows reported, want %d", name, len(summary.RowIDs), len(edits))
		}
		after := decodeRows(t, out, shopEntry, schema)
		if len(after) != len(before) {
			t.Fatalf("%s: row count %d -> %d", name, len(before), len(after))
		}
		for id, fields := range before {
			for field, value := range fields {
				if edited, ok := want[id][field]; ok {
					value = edited
				}
				if after[id][field] != value {
					t.Errorf("%s: row %d %s = %d, want %d", name, id, field, after[id][field], value)
				}
			}
		}

		original, _ := os.ReadFile(in)
		written, _ := os.ReadFile(out)
		platform, _ := reg.DetectPlatform(original)
		untouched := reg.UserData11Offset(platform)
		if platform == reg.PC {
			untouched -= reg.DigestSize
		}
		if !bytes.Equal(written[:untouched], original[:untouched]) {
			t.Errorf("%s: bytes before the regulation changed", name)
		}
	}
}

func TestApplyWithSchemaWritesAnEquipParam(t *testing.T) {
	in := fixture(t, ps4Fixture)
	schema, err := LoadEquipParamSchema(3)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := EquipParamEntryName(3)
	out := filepath.Join(t.TempDir(), "out")
	edits := []Edit{{RowID: 190, Fields: map[string]json.Number{"sellValue": "-1"}}}
	if _, err := ApplyWithSchema(in, out, entry, edits, schema); err != nil {
		t.Fatal(err)
	}
	if got := decodeRows(t, out, entry, schema)[190]["sellValue"]; got != -1 {
		t.Fatalf("Rune Arc's sellValue is %d, want -1", got)
	}
}

// Every refusal writes nothing, and the input is never the output.
func TestApplyRefusesBadEdits(t *testing.T) {
	in := fixture(t, ps4Fixture)
	original, _ := os.ReadFile(in)
	schema, err := ParseSchema([]byte(`{"param_type":"TEST","row_size":128,"fields":[
		{"name":"okval","type":"u32","array_length":1,"offset":0,"size":4},
		{"name":"pad","type":"dummy8","array_length":1,"offset":8,"size":1},
		{"name":"arr","type":"u8","array_length":4,"offset":12,"size":4},
		{"name":"farfield","type":"u32","array_length":1,"offset":2000000000,"size":4}]}`))
	if err != nil {
		t.Fatal(err)
	}
	const row = 100501
	for _, testCase := range []struct {
		row    int64
		fields map[string]json.Number
		want   string
	}{
		{999999999, map[string]json.Number{"okval": "1"}, "not found"},
		{1 << 40, map[string]json.Number{"okval": "1"}, "out of range for a param row id"},
		{row, map[string]json.Number{}, "no fields to edit"},
		{row, map[string]json.Number{"nonesuch": "1"}, "unknown field"},
		{row, map[string]json.Number{"pad": "1"}, "padding (dummy8)"},
		{row, map[string]json.Number{"arr": "1"}, "is an array"},
		{row, map[string]json.Number{"okval": "99999999999"}, "out of range"},
		{row, map[string]json.Number{"farfield": "1"}, "out of bounds"},
	} {
		out := filepath.Join(t.TempDir(), "out")
		_, err := ApplyWithSchema(in, out, shopEntry, []Edit{{RowID: testCase.row, Fields: testCase.fields}}, schema)
		if err == nil || !strings.Contains(err.Error(), testCase.want) {
			t.Errorf("want %q, got %v", testCase.want, err)
		}
		if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
			t.Errorf("%q: an output was written", testCase.want)
		}
	}
	edits := []Edit{{RowID: row, Fields: map[string]json.Number{"value": "1"}}}
	if _, err := Apply(in, in, shopEntry, edits); err == nil {
		t.Error("the input was accepted as the output")
	}
	if now, _ := os.ReadFile(in); !bytes.Equal(now, original) {
		t.Error("the fixture changed")
	}
}
