package catalog

// Reset-to-Vanilla diff tests. Uses the fixture saves (gitignored -> CI
// skips both).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/savefile"
)

const betterPSNSave = "../../save_files/BetterPSN.dat"

// TestVanillaDiffsEmptyOnPristineFixture is the capstone self-consistency
// check: vanilla_fresh_character.dat is the EXACT file
// internal/assets/data/vanilla_shop_lineup.json was generated from, so diffing it against
// itself must report zero drift. Catches any generator/embedding mistake
// for free, with no hand-picked assertions.
func TestVanillaDiffsEmptyOnPristineFixture(t *testing.T) {
	c := loadedCatalog(t)
	diffs, err := c.VanillaDiffs()
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 0 {
		var sample int64
		for id := range diffs {
			sample = id
			break
		}
		t.Fatalf("VanillaDiffs() on the pristine vanilla fixture found %d drifted row(s), e.g. row %d: %+v",
			len(diffs), sample, diffs[sample])
	}
}

// TestDiffFromVanillaDetectsFieldChange: a plain field bump (no item
// change) is reported as exactly that field.
func TestDiffFromVanillaDetectsFieldChange(t *testing.T) {
	c := loadedCatalog(t)
	rows, err := c.ShopRows()
	if err != nil {
		t.Fatal(err)
	}
	row := findRow(t, rows, func(r *Row) bool { return !r.MaterialLocked })

	clone := *row
	clone.Fields = map[string]int64{}
	for k, v := range row.Fields {
		clone.Fields[k] = v
	}
	clone.Fields["value"] = clone.Fields["value"] + 12345

	d, ok := c.DiffFromVanilla(&clone)
	if !ok {
		t.Fatal("DiffFromVanilla reported no diff for a row with a bumped value")
	}
	if d.ItemChanged {
		t.Error("ItemChanged = true for a value-only change")
	}
	if d.Fields["value"] != row.Fields["value"] {
		t.Errorf("Fields[value] = %d, want the original vanilla value %d", d.Fields["value"], row.Fields["value"])
	}
}

// TestDiffFromVanillaDetectsItemChange: bumping equipId is reported via
// ItemChanged, with the vanilla item's own resolved identity.
func TestDiffFromVanillaDetectsItemChange(t *testing.T) {
	c := loadedCatalog(t)
	rows, err := c.ShopRows()
	if err != nil {
		t.Fatal(err)
	}
	row := findRow(t, rows, func(r *Row) bool { return !r.MaterialLocked && r.ItemID != nil })

	clone := *row
	clone.Fields = map[string]int64{}
	for k, v := range row.Fields {
		clone.Fields[k] = v
	}
	clone.Fields["equipId"] = clone.Fields["equipId"] + 1 // almost certainly a different (or invalid) item

	d, ok := c.DiffFromVanilla(&clone)
	if !ok {
		t.Fatal("DiffFromVanilla reported no diff for a row with a bumped equipId")
	}
	if !d.ItemChanged {
		t.Error("ItemChanged = false for an equipId change")
	}
	if d.VanillaEquipID != row.Fields["equipId"] || d.VanillaEquipType != row.Fields["equipType"] {
		t.Errorf("Vanilla{EquipID,EquipType} = (%d,%d), want the row's own original (%d,%d)",
			d.VanillaEquipID, d.VanillaEquipType, row.Fields["equipId"], row.Fields["equipType"])
	}
	if d.VanillaItemName != row.ItemName {
		t.Errorf("VanillaItemName = %q, want %q", d.VanillaItemName, row.ItemName)
	}
	if _, staged := d.Fields["equipId"]; staged {
		t.Error("equipId leaked into Fields -- item-identity changes must only ever surface via ItemChanged/Vanilla*, never raw Fields")
	}
}

// TestDiffFromVanillaNoDiffWhenIdentical: an untouched row (straight from
// the vanilla fixture) reports ok=false.
func TestDiffFromVanillaNoDiffWhenIdentical(t *testing.T) {
	c := loadedCatalog(t)
	rows, err := c.ShopRows()
	if err != nil {
		t.Fatal(err)
	}
	row := findRow(t, rows, func(r *Row) bool { return true })
	if _, ok := c.DiffFromVanilla(row); ok {
		t.Error("DiffFromVanilla reported a diff for a row untouched from vanilla")
	}
}

// TestDiffFromVanillaSkipsUnknownRow: a row id absent from the embedded
// vanilla dataset reports ok=false rather than panicking.
func TestDiffFromVanillaSkipsUnknownRow(t *testing.T) {
	c := loadedCatalog(t)
	row := &Row{RowID: -999999, Fields: map[string]int64{"value": 1}}
	if _, ok := c.DiffFromVanilla(row); ok {
		t.Error("DiffFromVanilla reported a diff for a row id with no vanilla baseline")
	}
}

func findRow(t *testing.T, rows []*Row, pred func(*Row) bool) *Row {
	t.Helper()
	for _, r := range rows {
		if pred(r) {
			return r
		}
	}
	t.Fatal("no row in fixture matched the predicate")
	return nil
}

// TestResetToVanillaRoundTrip is the capstone integration test: compute
// VanillaDiffs() against the real third-party-edited BetterPSN.dat
// fixture, flatten to savefile.Edit the same way the GUI's
// rowEditFromVanillaDiff/BuildEdits would, apply through the
// already-trusted ApplyEdits into a throwaway working_copies/ file, reload,
// and confirm every previously-diverging row's raw Fields now exactly
// match the vanilla baseline. Proves end-to-end correctness through the
// real write path with no new write-path code.
func TestResetToVanillaRoundTrip(t *testing.T) {
	if _, err := os.Stat(betterPSNSave); err != nil {
		t.Skipf("fixture save absent, skipping: %v", err)
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.LoadSave(betterPSNSave); err != nil {
		t.Fatal(err)
	}

	diffs, err := c.VanillaDiffs()
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) == 0 {
		t.Skip("BetterPSN.dat has no drift from vanilla in this fixture snapshot -- nothing to round-trip")
	}

	edits := make([]savefile.Edit, 0, len(diffs))
	for id, d := range diffs {
		fields := make(map[string]json.Number, len(d.Fields)+2)
		for name, v := range d.Fields {
			fields[name] = json.Number(strconv.FormatInt(v, 10))
		}
		if d.ItemChanged {
			fields["equipId"] = json.Number(strconv.FormatInt(d.VanillaEquipID, 10))
			fields["equipType"] = json.Number(strconv.FormatInt(d.VanillaEquipType, 10))
		}
		edits = append(edits, savefile.Edit{RowID: id, Fields: fields})
	}

	out := filepath.Join(t.TempDir(), "reset.dat")
	if _, err := c.ApplyEdits(edits, out); err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}

	reloaded, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.LoadSave(out); err != nil {
		t.Fatal(err)
	}
	rDiffs, err := reloaded.VanillaDiffs()
	if err != nil {
		t.Fatal(err)
	}
	// Rows this batch didn't touch (material-locked, or genuinely already
	// vanilla) may still legitimately differ -- only assert the rows we
	// actually reset are now clean.
	for id := range diffs {
		if _, stillDrifted := rDiffs[id]; stillDrifted {
			t.Errorf("row %d still differs from vanilla after Reset-to-Vanilla round-trip: %+v", id, rDiffs[id])
		}
	}
}
