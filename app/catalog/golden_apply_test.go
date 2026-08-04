package catalog

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"er_merchant_editor/app/shopwrite"
)

// TestApplyEditsMatchesDirectApply proves the GUI's in-process write path
// (Catalog.ApplyEdits, with its validation layer) produces byte-identical
// output to calling shopwrite.Apply directly (what the CLI does) for the same
// edits. Guards against the two paths ever drifting. Uses GUI-eligible edits —
// material-locked rows are (deliberately) rejected by ApplyEdits, so the M1
// baseline edits (which include material row 111105) can't be reused here.
// Skips without the gitignored fixture.
func TestApplyEditsMatchesDirectApply(t *testing.T) {
	fixture := filepath.Join("..", "..", "save_files", "vanilla_fresh_character.dat")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture save not present, skipping: %v", err)
	}

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.LoadSave(fixture); err != nil {
		t.Fatal(err)
	}

	// Deterministic GUI-eligible edits: the TMH gate clear (row 101802, known
	// non-material) plus a price+quantity edit on the lowest-id non-material,
	// ungated row.
	rows, err := c.ShopRows()
	if err != nil {
		t.Fatal(err)
	}
	var priceRow int64 = -1
	for _, r := range rows { // param-table order; take the first eligible
		if !r.MaterialLocked && r.UnlockFlag == 0 && r.RowID != 101802 {
			priceRow = r.RowID
			break
		}
	}
	if priceRow == -1 {
		t.Fatal("no eligible row found for a price edit")
	}
	edits := []shopwrite.Edit{
		{RowID: priceRow, Fields: map[string]json.Number{"value": "777", "sellQuantity": "3"}},
		{RowID: 101802, Fields: map[string]json.Number{"eventFlag_forRelease": "0"}},
	}

	dir := t.TempDir()
	guiOut := filepath.Join(dir, "gui.dat")
	cliOut := filepath.Join(dir, "cli.dat")

	if _, err := c.ApplyEdits(edits, guiOut); err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	if _, err := shopwrite.Apply(fixture, cliOut, "ShopLineupParam.param", edits); err != nil {
		t.Fatalf("direct Apply: %v", err)
	}

	guiBytes, err := os.ReadFile(guiOut)
	if err != nil {
		t.Fatal(err)
	}
	cliBytes, err := os.ReadFile(cliOut)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(guiBytes) != sha256.Sum256(cliBytes) {
		t.Fatal("GUI path and direct Apply produced different bytes for identical edits")
	}

	// Save-As semantics: the catalog now points at the output and re-reading
	// rows reflects the edits.
	if c.SavePath() != guiOut {
		t.Fatalf("SavePath = %q after apply, want %q", c.SavePath(), guiOut)
	}
	applied, err := c.RowsByID()
	if err != nil {
		t.Fatal(err)
	}
	if got := applied[priceRow].Fields["value"]; got != 777 {
		t.Errorf("row %d value = %d after apply, want 777", priceRow, got)
	}
	if got := applied[101802].Fields["eventFlag_forRelease"]; got != 0 {
		t.Errorf("row 101802 eventFlag_forRelease = %d after apply, want 0", got)
	}
}
