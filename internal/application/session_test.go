package application

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/savefile"
)

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
