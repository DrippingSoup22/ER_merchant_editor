package catalog

// Golden test: every field of every ShopLineupParam row decoded by this Go
// port must match, value-for-value, what tools/savescan.py's `rows` command
// emits. The golden JSONL is generated out-of-band (see the header of
// TestGoldenRowsMatchSavescan); both it and the fixture save are gitignored, so
// CI skips this test.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/savefile"
)

const (
	fixtureSave = "../../save_files/vanilla_fresh_character.dat"
	// Regenerate with, from the repo root:
	//   tools/.venv/bin/python tools/savescan.py rows \
	//     save_files/vanilla_fresh_character.dat > working_copies/rows.golden.jsonl
	goldenRows = "../../working_copies/rows.golden.jsonl"
)

// enrichedKeys are the top-level keys savescan emits that are NOT raw param
// fields; everything else in a golden row is a raw decoded field.
var enrichedKeys = map[string]bool{
	"row_id": true, "merchant": true, "label": true, "item_name": true,
	"price": true, "cost_type": true, "quantity": true, "unlock_flag": true,
	"stock_flag": true, "materials": true, "warnings": true,
}

func TestGoldenRowsMatchSavescan(t *testing.T) {
	if _, err := os.Stat(fixtureSave); err != nil {
		t.Skipf("fixture save absent, skipping: %v", err)
	}
	if _, err := os.Stat(goldenRows); err != nil {
		t.Skipf("golden JSONL absent, skipping: %v", err)
	}

	golden := readGolden(t)

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.LoadSave(fixtureSave); err != nil {
		t.Fatal(err)
	}
	rows, err := c.ShopRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(golden) {
		t.Fatalf("row count: go=%d golden=%d", len(rows), len(golden))
	}

	// value_Magnification (f32) is decoded independently, in param order, since
	// DecodeRowFields (int64-only) skips it.
	mags := decodeMagnifications(t)
	if len(mags) != len(rows) {
		t.Fatalf("magnification count: %d vs rows %d", len(mags), len(rows))
	}

	fieldCmps := 0
	for i, g := range golden {
		r := rows[i]
		rid := jnInt(t, g["row_id"])
		ctx := func(field string) string {
			return "row " + itoa(rid) + " field " + field
		}

		eqInt(t, ctx("row_id"), rid, r.RowID)
		eqStr(t, ctx("merchant"), g["merchant"], r.Merchant)
		eqStr(t, ctx("label"), g["label"], r.Label)
		eqStr(t, ctx("item_name"), g["item_name"], r.ItemName)
		eqNullableInt(t, ctx("price"), g["price"], r.Price)
		eqInt(t, ctx("cost_type"), jnInt(t, g["cost_type"]), r.CostType)
		eqInt(t, ctx("quantity"), jnInt(t, g["quantity"]), r.Quantity)
		eqInt(t, ctx("unlock_flag"), jnInt(t, g["unlock_flag"]), r.UnlockFlag)
		eqInt(t, ctx("stock_flag"), jnInt(t, g["stock_flag"]), r.StockFlag)
		fieldCmps += 9

		fieldCmps += compareMaterials(t, ctx("materials"), g["materials"], r.Materials)
		fieldCmps += compareWarnings(t, ctx("warnings"), g["warnings"], r.Warnings)

		// Every raw decoded field savescan emits.
		rawCount := 0
		for k, v := range g {
			if enrichedKeys[k] {
				continue
			}
			rawCount++
			if k == "value_Magnification" {
				want, _ := v.(json.Number).Float64()
				if float64(mags[i]) != want {
					t.Errorf("%s: go=%v golden=%v", ctx(k), mags[i], want)
				}
				continue
			}
			want := jnInt(t, v)
			got, ok := r.Fields[k]
			if !ok {
				t.Errorf("%s: missing from go Fields (golden=%d)", ctx(k), want)
				continue
			}
			if got != want {
				t.Errorf("%s: go=%d golden=%d", ctx(k), got, want)
			}
		}
		fieldCmps += rawCount
		// No extra Go integer fields beyond the raw ones savescan emits
		// (rawCount includes value_Magnification, which is not in Fields).
		if len(r.Fields) != rawCount-1 {
			t.Errorf("row %d: go Fields has %d keys, golden raw non-float fields = %d", rid, len(r.Fields), rawCount-1)
		}
	}

	t.Logf("compared %d rows, %d field-level assertions, zero mismatches", len(rows), fieldCmps)
}

// --- helpers ---

func readGolden(t *testing.T) []map[string]interface{} {
	t.Helper()
	f, err := os.Open(goldenRows)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]interface{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		var m map[string]interface{}
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("parse golden line: %v", err)
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// decodeMagnifications reads value_Magnification (f32) for every row in param
// order, independently of the catalog.
func decodeMagnifications(t *testing.T) []float32 {
	t.Helper()
	reg, err := savefile.LoadRegulation(fixtureSave)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := savefile.FindBND4Entry(reg.BND4, "ShopLineupParam.param")
	if err != nil {
		t.Fatal(err)
	}
	blob := reg.BND4[entry.Offset : entry.Offset+entry.Size]
	header, err := savefile.ParseParamHeader(blob)
	if err != nil {
		t.Fatal(err)
	}
	prows, err := savefile.IterParamRows(blob, header)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := savefile.LoadShopSchema()
	if err != nil {
		t.Fatal(err)
	}
	mag, ok := schema.Field("value_Magnification")
	if !ok {
		t.Fatal("schema missing value_Magnification")
	}
	out := make([]float32, len(prows))
	for i, pr := range prows {
		off := pr.DataOffset + mag.Offset
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[off : off+4]))
	}
	return out
}

func compareMaterials(t *testing.T, ctx string, gv interface{}, got []Material) int {
	t.Helper()
	gl, _ := gv.([]interface{})
	if len(gl) != len(got) {
		t.Errorf("%s: length go=%d golden=%d", ctx, len(got), len(gl))
		return 0
	}
	cmps := 0
	for i, gm := range gl {
		m, _ := gm.(map[string]interface{})
		if un, isUn := m["unresolved_mtrl_id"]; isUn {
			if got[i].UnresolvedMtrlID != jnInt(t, un) {
				t.Errorf("%s[%d]: unresolved go=%d golden=%v", ctx, i, got[i].UnresolvedMtrlID, un)
			}
			cmps++
			continue
		}
		eqStr(t, ctx+" item_name", m["item_name"], got[i].ItemName)
		eqNullableInt(t, ctx+" item_id", m["item_id"], nullIf(got[i].ItemID))
		eqInt(t, ctx+" qty", jnInt(t, m["qty"]), got[i].Qty)
		cmps += 3
	}
	return cmps
}

func compareWarnings(t *testing.T, ctx string, gv interface{}, got []string) int {
	t.Helper()
	gl, _ := gv.([]interface{})
	if len(gl) != len(got) {
		t.Errorf("%s: length go=%d golden=%d", ctx, len(got), len(gl))
		return 0
	}
	for i, gw := range gl {
		if gw.(string) != got[i] {
			t.Errorf("%s[%d]: go=%q golden=%q", ctx, i, got[i], gw)
		}
	}
	return len(gl)
}

// --- primitive comparisons ---

func jnInt(t *testing.T, v interface{}) int64 {
	t.Helper()
	n, ok := v.(json.Number)
	if !ok {
		t.Fatalf("expected json.Number, got %T (%v)", v, v)
	}
	i, err := n.Int64()
	if err != nil {
		t.Fatalf("not an int64: %v", err)
	}
	return i
}

func eqInt(t *testing.T, ctx string, want, got int64) {
	t.Helper()
	if want != got {
		t.Errorf("%s: go=%d golden=%d", ctx, got, want)
	}
}

// eqStr compares a golden value that is either null (-> "") or a string.
func eqStr(t *testing.T, ctx string, gv interface{}, got string) {
	t.Helper()
	want := ""
	if gv != nil {
		s, ok := gv.(string)
		if !ok {
			t.Fatalf("%s: expected string/null, got %T", ctx, gv)
		}
		want = s
	}
	if want != got {
		t.Errorf("%s: go=%q golden=%q", ctx, got, want)
	}
}

// eqNullableInt compares a golden value that is either null (-> nil) or a
// number, against a *int64.
func eqNullableInt(t *testing.T, ctx string, gv interface{}, got *int64) {
	t.Helper()
	if gv == nil {
		if got != nil {
			t.Errorf("%s: go=%d golden=null", ctx, *got)
		}
		return
	}
	want := jnInt(t, gv)
	if got == nil {
		t.Errorf("%s: go=null golden=%d", ctx, want)
		return
	}
	if *got != want {
		t.Errorf("%s: go=%d golden=%d", ctx, *got, want)
	}
}

// nullIf treats a zero item_id as "resolved to 0" — materials in the fixture
// never carry a null item_id, so a non-pointer int64 is a faithful stand-in.
func nullIf(id int64) *int64 { return &id }

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
