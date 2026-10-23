package application

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/character"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/savefile"
)

// A save whose last stage fails, after an earlier stage has written, leaves
// the input loaded and no stage file behind, so the next save starts from the
// input again.
func TestFailedSaveKeepsTheInputLoaded(t *testing.T) {
	in := filepath.Join("..", "..", "save_files", "vanilla_fresh_character.dat")
	if _, err := os.Stat(in); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.LoadSave(in); err != nil {
		t.Fatal(err)
	}
	session := NewSession(cat)
	session.PendingBellBearingEdits[0] = map[uint32]bool{character.BellBearings[0].FlagID: true}
	// No row has this ID, so the item stage, the last, refuses it.
	session.PendingEdits[-1] = &RowEdit{FieldChanges: map[string]FieldChange{"value": {From: 0, To: 1}}}
	plan, err := session.PrepareSave()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var editErr *catalog.EditError
	if err := plan.Apply(filepath.Join(dir, "out.dat")); !errors.As(err, &editErr) {
		t.Fatalf("Apply = %v, want the item stage's refusal", err)
	}
	if cat.SavePath() != in {
		t.Errorf("the catalog was left on %s", cat.SavePath())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("the failed save left %d file(s) behind", len(entries))
	}
}

func TestBuildEditsIsDeterministicAndComplete(t *testing.T) {
	session := NewSession(&catalog.Catalog{})
	session.PendingEdits[20] = &RowEdit{
		FieldChanges: map[string]FieldChange{"value": {From: 100, To: 250}},
	}
	session.PendingEdits[10] = &RowEdit{
		FieldChanges: map[string]FieldChange{"sellQuantity": {From: 1, To: 3}},
		ItemChange: &ItemChange{
			EquipID:        1234,
			EquipType:      2,
			ClearOverrides: []string{"eventFlag_forRelease", "mtrlId"},
		},
	}

	want := []savefile.Edit{
		{RowID: 10, Fields: map[string]json.Number{
			"sellQuantity":         "3",
			"equipId":              "1234",
			"equipType":            "2",
			"eventFlag_forRelease": "-1",
			"mtrlId":               "-1",
		}},
		{RowID: 20, Fields: map[string]json.Number{"value": "250"}},
	}
	if got := session.BuildEdits(); !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildEdits() = %#v, want %#v", got, want)
	}
}

func TestClearPendingReplacesEveryStagingMap(t *testing.T) {
	session := NewSession(&catalog.Catalog{})
	oldRows := session.PendingEdits
	oldFlags := session.PendingFlagEdits
	oldBells := session.PendingBellBearingEdits
	oldRows[1] = &RowEdit{}
	oldFlags[0] = map[int64]bool{1: true}
	oldBells[0] = map[uint32]bool{2: true}

	session.ClearPending()

	if len(session.PendingEdits) != 0 || len(session.PendingFlagEdits) != 0 || len(session.PendingBellBearingEdits) != 0 {
		t.Fatal("ClearPending left staged changes behind")
	}
	oldRows[3] = &RowEdit{}
	oldFlags[1] = map[int64]bool{4: true}
	oldBells[1] = map[uint32]bool{5: true}
	if len(session.PendingEdits) != 0 || len(session.PendingFlagEdits) != 0 || len(session.PendingBellBearingEdits) != 0 {
		t.Fatal("ClearPending reused a staging map that callers could still mutate")
	}
}

func TestItemChangeDisplayName(t *testing.T) {
	if got := (&ItemChange{ToName: "Claymore"}).DisplayName(); got != "Claymore" {
		t.Fatalf("base display name = %q", got)
	}
	if got := (&ItemChange{ToName: "Claymore", Level: 10}).DisplayName(); got != "Claymore +10" {
		t.Fatalf("reinforced display name = %q", got)
	}
}
