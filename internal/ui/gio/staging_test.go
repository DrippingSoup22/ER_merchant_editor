package gio

// Pure staging-logic tests (no window/frame loop involved).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
)

// noteRow fabricates a "Note: X"-style row: nameMsgId pinned, everything else
// default. Field values mirror row 100500 (Kale's physick note) in shape.
func noteRow() *catalog.Row {
	return &catalog.Row{
		RowID:    100500,
		ItemName: "Note: Flask of Wondrous Physick",
		Fields: map[string]int64{
			"equipId": 8600, "equipType": 3, "value": 500,
			"iconId": -1, "nameMsgId": 8752, "menuTitleMsgId": -1, "menuIconId": -1,
		},
	}
}

func sellableItem(name string, id int64) *catalog.Item {
	et, eid := 3, id
	return &catalog.Item{Name: name, EquipType: &et, EquipID: &eid}
}

// TestSwapClearsDisplayOverrides: staging a swap on an override row stages the
// override reset, BuildEdits emits -1 for it, and undoing the swap drops it.
func TestSwapClearsDisplayOverrides(t *testing.T) {
	s := newTestState(nil)
	row := noteRow()
	s.stageItemSwapCore(s.PendingEdits, row, sellableItem("Rune Arc", 190))

	e := s.PendingEdits[row.RowID]
	if e == nil || e.ItemChange == nil {
		t.Fatal("swap not staged")
	}
	if got := e.ItemChange.ClearOverrides; len(got) != 1 || got[0] != "nameMsgId" {
		t.Fatalf("ClearOverrides = %v, want [nameMsgId]", got)
	}

	edits := s.BuildEdits()
	if len(edits) != 1 {
		t.Fatalf("BuildEdits len = %d, want 1", len(edits))
	}
	f := edits[0].Fields
	if f["nameMsgId"] != "-1" {
		t.Fatalf("nameMsgId = %q, want -1 (full fields: %v)", f["nameMsgId"], f)
	}
	if f["equipId"] != "190" || f["equipType"] != "3" {
		t.Fatalf("swap fields wrong: %v", f)
	}

	s.ClearItemSwap(s.PendingEdits, row.RowID)
	if len(s.BuildEdits()) != 0 {
		t.Fatal("undoing the swap should drop the override reset too")
	}
}

// TestSwapOnCleanRowStagesNoOverrideReset: rows without overrides get none.
func TestSwapOnCleanRowStagesNoOverrideReset(t *testing.T) {
	s := newTestState(nil)
	row := noteRow()
	for _, k := range []string{"iconId", "nameMsgId", "menuTitleMsgId", "menuIconId"} {
		row.Fields[k] = -1
	}
	s.stageItemSwapCore(s.PendingEdits, row, sellableItem("Rune Arc", 190))
	f := s.BuildEdits()[0].Fields
	if _, ok := f["nameMsgId"]; ok {
		t.Fatalf("unexpected override reset staged: %v", f)
	}
	if len(f) != 2 {
		t.Fatalf("want only equipId+equipType, got %v", f)
	}
}

// TestAutoFreeUnlimitedStagesOnSwap: with both settings on, swapping an item
// onto a row stages price 0 and unlimited (-1) quantity alongside the swap,
// without the user touching either field directly.
func TestAutoFreeUnlimitedStagesOnSwap(t *testing.T) {
	s := newTestState(nil)
	s.Settings.AutoFreeItems = true
	s.Settings.AutoUnlimitedItems = true
	row := noteRow()
	row.Fields["sellQuantity"] = 5
	s.stageItemSwapCore(s.PendingEdits, row, sellableItem("Rune Arc", 190))

	f := s.BuildEdits()[0].Fields
	if f["value"] != "0" {
		t.Fatalf("value = %q, want 0 (auto-free)", f["value"])
	}
	if f["sellQuantity"] != "-1" {
		t.Fatalf("sellQuantity = %q, want -1 (auto-unlimited)", f["sellQuantity"])
	}
}

// TestAutoFreeUnlimitedOffByDefault: with the settings off (zero value), a
// swap stages neither field -- existing behavior, unaffected by the feature.
func TestAutoFreeUnlimitedOffByDefault(t *testing.T) {
	s := newTestState(nil)
	row := noteRow()
	s.stageItemSwapCore(s.PendingEdits, row, sellableItem("Rune Arc", 190))

	f := s.BuildEdits()[0].Fields
	if _, ok := f["value"]; ok {
		t.Fatalf("value staged with auto-free off: %v", f)
	}
	if _, ok := f["sellQuantity"]; ok {
		t.Fatalf("sellQuantity staged with auto-unlimited off: %v", f)
	}
}

// TestAutoFreeUnlimitedUserOverrideWins: the user's own field edit after the
// swap takes precedence over the auto-staged default, whichever order they
// happen in staging-call order (mirrors the real UI: pick item, then adjust
// price in the row-edit form).
func TestAutoFreeUnlimitedUserOverrideWins(t *testing.T) {
	s := newTestState(nil)
	s.Settings.AutoFreeItems = true
	row := noteRow()
	s.stageItemSwapCore(s.PendingEdits, row, sellableItem("Rune Arc", 190))
	s.StageField(s.PendingEdits, row, "value", 300)

	f := s.BuildEdits()[0].Fields
	if f["value"] != "300" {
		t.Fatalf("value = %q, want user-staged 300 to win over auto-free 0", f["value"])
	}
}

// TestAutoFreeNoOpWhenAlreadyFree: auto-free staging a value equal to the
// row's own current price must NOT create a no-op field entry (mirrors
// StageField's own to==from unstage rule).
func TestAutoFreeNoOpWhenAlreadyFree(t *testing.T) {
	s := newTestState(nil)
	s.Settings.AutoFreeItems = true
	row := noteRow()
	row.Fields["value"] = 0
	s.stageItemSwapCore(s.PendingEdits, row, sellableItem("Rune Arc", 190))

	f := s.BuildEdits()[0].Fields
	if _, ok := f["value"]; ok {
		t.Fatalf("value staged even though row was already free: %v", f)
	}
}

// TestStageItemSwapCapturesMerchant: setRowEntry stamps the currently active
// merchant (s.lastMerchant) onto the staged RowEdit, since Row.Merchant
// itself is the raw, sometimes-empty Paramdex name (see enrich.go) and can't
// be reused for the normal-mode pending-list display.
func TestStageItemSwapCapturesMerchant(t *testing.T) {
	s := newTestState(nil)
	s.lastMerchant = "Merchant Kale"
	row := noteRow()
	s.stageItemSwapCore(s.PendingEdits, row, sellableItem("Rune Arc", 190))
	if got := s.PendingEdits[row.RowID].Merchant; got != "Merchant Kale" {
		t.Fatalf("Merchant = %q, want %q", got, "Merchant Kale")
	}
}

// TestFilteredItemsHidesRiskyUnlessOptedIn: by default the catalog must
// never offer cut-content/ban-risk items for staging; Settings.
// ShowRiskyItems (the deliberate opt-in that replaced Debug mode
// 2026-08-03) must offer every one of them.
func TestFilteredItemsHidesRiskyUnlessOptedIn(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	riskyCount := 0
	for _, it := range cat.ListItems("", "", "", nil) {
		if it.Risky {
			riskyCount++
		}
	}
	if riskyCount == 0 {
		t.Fatal("fixture data has no risky items -- test can't exercise the filter")
	}

	s := NewState(cat)
	for _, it := range s.filteredItems() {
		if it.Risky {
			t.Fatalf("default settings listed a risky item: %s", it.Name)
		}
	}

	s.Settings.ShowRiskyItems = true
	s.itemsCache = nil // force re-filter under the new setting
	got := 0
	for _, it := range s.filteredItems() {
		if it.Risky {
			got++
		}
	}
	if got != riskyCount {
		t.Fatalf("ShowRiskyItems listed %d risky items, want %d", got, riskyCount)
	}
}

// TestHazardWarningsFiltersHandledCases: the display-override and material-
// exchange warnings are not red-square hazards (both have dedicated
// treatment); everything else passes through.
func TestHazardWarningsFiltersHandledCases(t *testing.T) {
	row := &catalog.Row{Warnings: []string{
		"name/icon override: item-swap-inherits-display assumption doesn't hold",
		"requires a material exchange (see materials) in addition to/instead of the rune price",
		"equipType 5 has no known item-id offset",
	}}
	got := hazardWarnings(row)
	if len(got) != 1 || got[0] != "equipType 5 has no known item-id offset" {
		t.Fatalf("hazardWarnings = %v", got)
	}
}

// --- weapon reinforcement level staging (2026-07-28) ---

// findItem returns the exact-name items.json entry, failing the test if it's
// not found or ambiguous under ListItems' substring search.
func findItem(t *testing.T, cat *catalog.Catalog, name string) *catalog.Item {
	t.Helper()
	for _, it := range cat.ListItems("", "", name, nil) {
		if it.Name == name {
			return it
		}
	}
	t.Fatalf("item %q not found", name)
	return nil
}

func weaponRow(rowID int64, item *catalog.Item) *catalog.Row {
	return &catalog.Row{
		RowID:    rowID,
		ItemName: item.Name,
		ItemID:   &item.ID,
		IconPath: item.IconPath,
		Fields: map[string]int64{
			"iconId": -1, "nameMsgId": -1, "menuTitleMsgId": -1, "menuIconId": -1,
		},
	}
}

// TestStageItemSwapCoreAppliesPickLevel: picking a weapon-table item with a
// shared PickLevel set stages that level, and EquipID reflects base+level.
func TestStageItemSwapCoreAppliesPickLevel(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	s := newTestState(cat)
	s.PickLevel = 7
	zwei := findItem(t, cat, "Zweihander")
	row := noteRow()
	row.RowID = 1
	s.stageItemSwapCore(s.PendingEdits, row, zwei)

	ic := s.PendingEdits[row.RowID].ItemChange
	if ic == nil {
		t.Fatal("swap not staged")
	}
	if ic.Level != 7 {
		t.Fatalf("Level = %d, want 7", ic.Level)
	}
	if ic.EquipID != *zwei.EquipID+7 {
		t.Fatalf("EquipID = %d, want %d", ic.EquipID, *zwei.EquipID+7)
	}
	if ic.BaseEquipID != *zwei.EquipID {
		t.Fatalf("BaseEquipID = %d, want %d", ic.BaseEquipID, *zwei.EquipID)
	}
}

// TestStageItemSwapCoreClampsPickLevelToItemMax: a PickLevel above the
// picked weapon's own max (e.g. a somber/+10-cap legendary weapon) clamps
// down instead of producing an out-of-range id.
func TestStageItemSwapCoreClampsPickLevelToItemMax(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	s := newTestState(cat)
	s.PickLevel = 99
	moonveil := findItem(t, cat, "Moonveil")
	row := noteRow()
	row.RowID = 1
	s.stageItemSwapCore(s.PendingEdits, row, moonveil)

	ic := s.PendingEdits[row.RowID].ItemChange
	if ic.Level != 10 {
		t.Fatalf("Level = %d, want 10 (Moonveil's real max)", ic.Level)
	}
}

// TestStageItemSwapCoreNonWeaponIgnoresPickLevel: PickLevel must never leak
// onto a non-weapon-table item swap.
func TestStageItemSwapCoreNonWeaponIgnoresPickLevel(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	s := newTestState(cat)
	s.PickLevel = 5
	row := noteRow()
	s.stageItemSwapCore(s.PendingEdits, row, sellableItem("Rune Arc", 190)) // equipType 3 (Good)

	ic := s.PendingEdits[row.RowID].ItemChange
	if ic.Level != 0 {
		t.Fatalf("Level = %d, want 0 for a non-weapon item", ic.Level)
	}
	if ic.EquipID != 190 {
		t.Fatalf("EquipID = %d, want 190 (unchanged)", ic.EquipID)
	}
}

// TestStageItemLevelOnUnswappedRow: leveling a row's own current item (no
// prior pick/drag) synthesizes an ItemChange from the row, and it reads as
// level-only (FromName == ToName), not a swap.
func TestStageItemLevelOnUnswappedRow(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	zwei := findItem(t, cat, "Zweihander")
	s := newTestState(cat)
	row := weaponRow(2, zwei)

	s.StageItemLevel(s.PendingEdits, row, 12)

	ic := s.PendingEdits[row.RowID].ItemChange
	if ic == nil {
		t.Fatal("level change not staged")
	}
	if ic.Level != 12 {
		t.Fatalf("Level = %d, want 12", ic.Level)
	}
	if !ic.IsLevelOnly() {
		t.Fatalf("expected IsLevelOnly, FromName=%q ToName=%q", ic.FromName, ic.ToName)
	}
	if got, want := ic.DisplayName(), "Zweihander +12"; got != want {
		t.Fatalf("DisplayName = %q, want %q", got, want)
	}
	if ic.EquipID != *zwei.EquipID+12 {
		t.Fatalf("EquipID = %d, want %d", ic.EquipID, *zwei.EquipID+12)
	}
}

// TestStageItemLevelAdjustsExistingSwap: leveling a row that already has a
// staged item swap adjusts that swap's level in place rather than creating
// a second entry.
func TestStageItemLevelAdjustsExistingSwap(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	moonveil := findItem(t, cat, "Moonveil")
	s := newTestState(cat)
	s.PickLevel = 3
	row := noteRow()
	row.RowID = 3
	s.stageItemSwapCore(s.PendingEdits, row, moonveil)
	if s.PendingEdits[row.RowID].ItemChange.Level != 3 {
		t.Fatal("setup: expected initial level 3")
	}

	s.StageItemLevel(s.PendingEdits, row, 8)

	ic := s.PendingEdits[row.RowID].ItemChange
	if ic.Level != 8 {
		t.Fatalf("Level = %d, want 8", ic.Level)
	}
	if ic.ToName != "Moonveil" {
		t.Fatalf("ToName = %q, want unchanged %q", ic.ToName, "Moonveil")
	}
	if ic.EquipID != *moonveil.EquipID+8 {
		t.Fatalf("EquipID = %d, want %d", ic.EquipID, *moonveil.EquipID+8)
	}
}

// TestStageItemLevelClampsToMax: an out-of-range requested level clamps to
// the item's real max instead of staging an invalid id.
func TestStageItemLevelClampsToMax(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	moonveil := findItem(t, cat, "Moonveil") // max 10
	s := newTestState(cat)
	row := weaponRow(4, moonveil)

	s.StageItemLevel(s.PendingEdits, row, 999)

	if got := s.PendingEdits[row.RowID].ItemChange.Level; got != 10 {
		t.Fatalf("Level = %d, want 10 (clamped)", got)
	}
}

// TestStageItemLevelZeroWithNoSwapIsNoop: leveling to 0 (the row's already-
// unleveled state) with nothing else staged must not create an empty entry.
func TestStageItemLevelZeroWithNoSwapIsNoop(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	zwei := findItem(t, cat, "Zweihander")
	s := newTestState(cat)
	row := weaponRow(5, zwei)

	s.StageItemLevel(s.PendingEdits, row, 0)

	if _, staged := s.PendingEdits[row.RowID]; staged {
		t.Fatal("leveling to 0 with nothing staged should be a no-op")
	}
}

// --- row-edit bar draft / Apply (2026-07-29) ---

// TestDraftApplyCommitsBatchFieldEdits exercises the exact path the row-edit
// bar's multi-select Price/Quantity fields use: stage into the draft for
// several rows, confirm PendingEdits is untouched until Apply, then confirm
// Apply commits every row's drafted value.
func TestDraftApplyCommitsBatchFieldEdits(t *testing.T) {
	s := newTestState(nil)
	rowA := &catalog.Row{RowID: 1, Fields: map[string]int64{"value": 300}}
	rowB := &catalog.Row{RowID: 2, Fields: map[string]int64{"value": 400}}
	rows := []*catalog.Row{rowA, rowB}

	s.SelectedRowIDs = []int64{1, 2}
	s.ensureDraft()
	for _, row := range rows {
		s.StageField(s.draftEdits, row, "value", 999)
	}
	if len(s.PendingEdits) != 0 {
		t.Fatalf("PendingEdits must stay empty before Apply, got %v", s.PendingEdits)
	}

	s.applyDraft()
	for _, row := range rows {
		e := s.PendingEdits[row.RowID]
		if e == nil {
			t.Fatalf("row %d: not committed after Apply", row.RowID)
		}
		if fc, ok := e.FieldChanges["value"]; !ok || fc.To != 999 {
			t.Fatalf("row %d: value not committed to 999: %+v", row.RowID, e.FieldChanges)
		}
	}
}

// TestDraftDiscardedWhenSelectionChanges: closing or switching the row-edit
// bar's selection without clicking Apply must discard the draft entirely --
// user: "the cost and quantity and the unlocks and all the params changed
// get back to the previous ones." Also covers reopening the SAME row set
// after a discard, which must start fresh rather than resurrect the old
// (never-applied) draft.
func TestDraftDiscardedWhenSelectionChanges(t *testing.T) {
	s := newTestState(nil)
	rowA := &catalog.Row{RowID: 1, Fields: map[string]int64{"value": 300}}

	s.SelectedRowIDs = []int64{1}
	s.ensureDraft()
	s.StageField(s.draftEdits, rowA, "value", 999)
	if s.draftEdits[1] == nil {
		t.Fatal("setup: expected a draft entry")
	}

	s.SelectedRowIDs = nil // bar closes without Apply
	s.ensureDraft()
	if len(s.draftEdits) != 0 {
		t.Fatalf("draft should be discarded on close, got %v", s.draftEdits)
	}
	if len(s.PendingEdits) != 0 {
		t.Fatal("PendingEdits must never see an un-applied draft")
	}

	s.SelectedRowIDs = []int64{1} // reselect the SAME row
	s.ensureDraft()
	if s.draftEdits[1] != nil {
		t.Fatal("reopening the same row after a discard must not resurrect the old draft")
	}
}

// TestCommitTypedFieldsBeforeApplyForcesUnsubmittedText covers the fix for
// the user-reported "changing the values for price and quantity for a
// shared window... doesn't change anything on them": commitInt/
// liveClampedInt only ever staged a value on the editor's own Submit
// (Enter) event, so text typed and left uncommitted got silently dropped
// when Apply was clicked. commitTypedFieldsBeforeApply must force-commit it
// first, exactly as if Enter had been pressed.
func TestCommitTypedFieldsBeforeApplyForcesUnsubmittedText(t *testing.T) {
	s := newTestState(nil)
	rowA := &catalog.Row{RowID: 1, Fields: map[string]int64{"value": 300, "sellQuantity": 5}}
	rowB := &catalog.Row{RowID: 2, Fields: map[string]int64{"value": 400, "sellQuantity": 5}}
	rows := []*catalog.Row{rowA, rowB}

	s.SelectedRowIDs = []int64{1, 2}
	s.ensureDraft()

	s.priceEditor.SetText("999") // typed but never Submitted (no Enter press)
	s.qtyEditor.SetText("7")

	s.commitTypedFieldsBeforeApply(rows)

	for _, row := range rows {
		e := s.draftEdits[row.RowID]
		if e == nil {
			t.Fatalf("row %d: no draft entry after commitTypedFieldsBeforeApply", row.RowID)
		}
		if fc, ok := e.FieldChanges["value"]; !ok || fc.To != 999 {
			t.Fatalf("row %d: price not force-committed: %+v", row.RowID, e.FieldChanges)
		}
		if fc, ok := e.FieldChanges["sellQuantity"]; !ok || fc.To != 7 {
			t.Fatalf("row %d: quantity not force-committed: %+v", row.RowID, e.FieldChanges)
		}
	}

	// Applying should then land both in PendingEdits, same as the
	// already-Submitted case TestDraftApplyCommitsBatchFieldEdits covers.
	s.applyDraft()
	for _, row := range rows {
		e := s.PendingEdits[row.RowID]
		if e == nil || e.FieldChanges["value"].To != 999 || e.FieldChanges["sellQuantity"].To != 7 {
			t.Fatalf("row %d: not committed after Apply: %+v", row.RowID, e)
		}
	}
}

// TestCommitTypedFieldsBeforeApplyLeavesBlankFieldsAlone: an editor left
// blank (showing the "Mixed" hint, nothing actually typed) must not force a
// value onto rows that disagree -- only a field the user actually typed
// into gets force-committed.
func TestCommitTypedFieldsBeforeApplyLeavesBlankFieldsAlone(t *testing.T) {
	s := newTestState(nil)
	rowA := &catalog.Row{RowID: 1, Fields: map[string]int64{"value": 300}}
	s.SelectedRowIDs = []int64{1}
	s.ensureDraft()

	s.commitTypedFieldsBeforeApply([]*catalog.Row{rowA})
	if s.draftEdits[1] != nil {
		t.Fatalf("blank editor must not stage anything, got %+v", s.draftEdits[1])
	}
}

// TestFormFieldsClampPriceAndQuantity covers the fix for "immediately
// adjust the value the user input, like in the weapons level window":
// price/quantity must live-clamp exactly like the weapon-level field
// (liveClampedInt), and commitTypedFieldsBeforeApply must apply the same
// clamp to unsubmitted text so Apply can't bypass it.
func TestFormFieldsClampPriceAndQuantity(t *testing.T) {
	s := newTestState(nil)
	row := &catalog.Row{RowID: 1, Fields: map[string]int64{"value": 300, "sellQuantity": 5}}
	rows := []*catalog.Row{row}

	s.SelectedRowIDs = []int64{1}
	s.ensureDraft()

	s.priceEditor.SetText("-50")
	s.qtyEditor.SetText("999")
	s.commitTypedFieldsBeforeApply(rows)

	e := s.draftEdits[1]
	if e == nil {
		t.Fatal("no draft entry after commitTypedFieldsBeforeApply")
	}
	if fc, ok := e.FieldChanges["value"]; !ok || fc.To != priceMin {
		t.Errorf("price -50 should clamp to %d, got %+v", priceMin, e.FieldChanges["value"])
	}
	if fc, ok := e.FieldChanges["sellQuantity"]; !ok || fc.To != qtyMax {
		t.Errorf("quantity 999 should clamp to %d, got %+v", qtyMax, e.FieldChanges["sellQuantity"])
	}

	// Quantity's floor is -1 (unlimited), not 0 -- must not clamp a valid -1
	// up to 0.
	s.qtyEditor.SetText("-1")
	s.commitTypedFieldsBeforeApply(rows)
	if fc := s.draftEdits[1].FieldChanges["sellQuantity"]; fc.To != -1 {
		t.Errorf("quantity -1 (unlimited) must be preserved, got %d", fc.To)
	}

	// A quantity below -1 must still clamp up to -1, not stay negative.
	s.qtyEditor.SetText("-5")
	s.commitTypedFieldsBeforeApply(rows)
	if fc := s.draftEdits[1].FieldChanges["sellQuantity"]; fc.To != qtyMin {
		t.Errorf("quantity -5 should clamp to %d, got %d", qtyMin, fc.To)
	}
}

// TestDraftUnlockAllStagesEveryDistinctFlag covers the multi-select bar's
// "Unlock all" button: every distinct UnlockFlag among the given rows gets
// drafted unlocked (a shared flag, e.g. a bell-bearing group, collapses to
// one entry), ungated rows (UnlockFlag == 0) are skipped.
func TestDraftUnlockAllStagesEveryDistinctFlag(t *testing.T) {
	s := newTestState(nil)
	rowA := &catalog.Row{RowID: 1, UnlockFlag: 111}
	rowB := &catalog.Row{RowID: 2, UnlockFlag: 111} // shares rowA's flag (bell-bearing group)
	rowC := &catalog.Row{RowID: 3, UnlockFlag: 222}
	rowD := &catalog.Row{RowID: 4, UnlockFlag: 0} // ungated, must be skipped

	s.SelectedChar = 0
	s.charFlagState = map[int64]bool{1: false, 2: false, 3: false}

	s.draftUnlockAll([]*catalog.Row{rowA, rowB, rowC, rowD})

	if len(s.draftGateEdits) != 2 {
		t.Fatalf("draftGateEdits = %v, want exactly 2 distinct flags", s.draftGateEdits)
	}
	if edit, ok := s.draftGateEdits[111]; !ok || !edit.target {
		t.Fatalf("flag 111 not drafted unlocked: %+v", s.draftGateEdits[111])
	}
	if edit, ok := s.draftGateEdits[222]; !ok || !edit.target {
		t.Fatalf("flag 222 not drafted unlocked: %+v", s.draftGateEdits[222])
	}
}

// TestGateActionSpecSingleRow covers formActionsRow's 2nd-slot button for a
// single selected row: an ungated row or an unknown (no character
// selected) gate state must come back disabled rather than omitted (the
// "grey out, never disappear" rule), and the label toggles Unlock/Lock
// with the row's actual state once known.
func TestGateActionSpecSingleRow(t *testing.T) {
	s := newTestState(nil)
	ungated := &catalog.Row{RowID: 1, UnlockFlag: 0}
	if label, enabled, btn := s.gateActionSpec([]*catalog.Row{ungated}); label != "Unlock" || enabled || btn != &s.gateBtn {
		t.Fatalf("ungated row: got (%q, %v), want (Unlock, false, &s.gateBtn)", label, enabled)
	}

	gated := &catalog.Row{RowID: 2, UnlockFlag: 111}
	if _, enabled, _ := s.gateActionSpec([]*catalog.Row{gated}); enabled {
		t.Fatal("gated row with no character selected must be disabled")
	}

	s.SelectedChar = 0
	s.charFlagState = map[int64]bool{2: false}
	if label, enabled, _ := s.gateActionSpec([]*catalog.Row{gated}); label != "Unlock" || !enabled {
		t.Fatalf("locked gated row: got (%q, %v), want (Unlock, true)", label, enabled)
	}

	s.charFlagState[2] = true
	if label, enabled, _ := s.gateActionSpec([]*catalog.Row{gated}); label != "Lock" || !enabled {
		t.Fatalf("unlocked gated row: got (%q, %v), want (Lock, true)", label, enabled)
	}
}

// TestGateActionSpecMultiRow covers the "Unlock all" slot for a
// multi-selection: disabled with no character selected or once every gated
// row is already unlocked, enabled the moment at least one is still locked.
func TestGateActionSpecMultiRow(t *testing.T) {
	s := newTestState(nil)
	rowA := &catalog.Row{RowID: 1, UnlockFlag: 111}
	rowB := &catalog.Row{RowID: 2, UnlockFlag: 222}
	rows := []*catalog.Row{rowA, rowB}

	if _, enabled, btn := s.gateActionSpec(rows); enabled || btn != &s.unlockAllFormBtn {
		t.Fatal("multi-row with no character selected must be disabled")
	}

	s.SelectedChar = 0
	s.charFlagState = map[int64]bool{1: true, 2: true} // both already unlocked
	if label, enabled, _ := s.gateActionSpec(rows); label != "Unlock all" || enabled {
		t.Fatalf("both already unlocked: got (%q, %v), want (Unlock all, false)", label, enabled)
	}

	s.charFlagState[2] = false // one still locked
	if _, enabled, _ := s.gateActionSpec(rows); !enabled {
		t.Fatal("one still-locked row must enable Unlock all")
	}
}

// firstNRowIDs returns n arbitrary distinct real row ids from the fixture,
// for tests that just need "some real row" as a vessel for a synthetic
// ItemChange -- which item/merchant it originally belonged to doesn't
// matter once an ItemChange overrides it.
func firstNRowIDs(t *testing.T, cat *catalog.Catalog, n int) []int64 {
	t.Helper()
	rowsByID, err := cat.RowsByID()
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, n)
	for id := range rowsByID {
		ids = append(ids, id)
		if len(ids) == n {
			break
		}
	}
	if len(ids) < n {
		t.Fatalf("fixture only has %d rows, need %d", len(ids), n)
	}
	return ids
}

// TestBuildEquipParamEditsRealItems covers the 2026-08-01 global-minimum
// redesign (see docs/MERCHANT_DATA.md): sellValue targets the TRUE minimum
// effective price across EVERY row in the whole save currently selling
// that item -- not just the rows in this PendingEdits batch, and not
// clamped against the item's previous sellValue -- so it can shrink OR
// rise depending on what the rest of the save's untouched rows are doing.
// Real fixture prices used below (verified via
// `tools/savescan.py rows save_files/vanilla_fresh_character.dat`, not
// re-derived here): Rune Arc (190) is also sold, untouched, at 4000-8000;
// Dagger (1000000) at exactly 400 on 3 untouched rows; Cracked Pot (9500)
// at 300-1500 on 6 untouched rows.
func TestBuildEquipParamEditsRealItems(t *testing.T) {
	cat := loadedTestCatalog(t)
	s := NewState(cat)
	ids := firstNRowIDs(t, cat, 5)

	runeArc := &ItemChange{ToName: "Rune Arc", EquipType: 3, BaseEquipID: 190, EquipID: 190}      // real sellValue=200
	dagger := &ItemChange{ToName: "Dagger", EquipType: 0, BaseEquipID: 1000000, EquipID: 1000000} // real sellValue=100
	crackedPot := &ItemChange{ToName: "Cracked Pot", EquipType: 3, BaseEquipID: 9500, EquipID: 9500}

	// rowA: swapped to Rune Arc, priced to 500.
	s.PendingEdits[ids[0]] = &RowEdit{
		FieldChanges: map[string]FieldChange{"value": {From: 200, To: 500}},
		ItemChange:   runeArc,
	}
	// rowB: ALSO swapped to Rune Arc, priced LOWER (100) -- same item as
	// rowA: the target must be the MINIMUM across every row selling this
	// item, not whichever row happens to be processed last (map iteration
	// order isn't deterministic). 100 is still below every untouched
	// vanilla Rune Arc vendor (4000-8000), so it stays the global min.
	s.PendingEdits[ids[1]] = &RowEdit{
		FieldChanges: map[string]FieldChange{"value": {From: 500, To: 100}},
		ItemChange:   runeArc,
	}
	// rowC: swapped to Dagger, priced HIGHER (500) than Dagger's 3 real,
	// untouched vanilla vendors (400 each) -- so THIS row isn't the
	// constraint; the global min is 400, ABOVE Dagger's current live
	// sellValue (100). This is the core new behavior: sellValue must be
	// allowed to RISE from 100 to 400, since nothing sells it any cheaper
	// anymore.
	s.PendingEdits[ids[2]] = &RowEdit{
		FieldChanges: map[string]FieldChange{"value": {From: 100, To: 500}},
		ItemChange:   dagger,
	}
	// rowD: swapped to Cracked Pot, priced to 0 -- below every untouched
	// vanilla Cracked Pot vendor (300-1500), so 0 becomes the global min,
	// pulling sellValue down from -1 to 0 (0 and -1 are behaviorally
	// identical for visibility, confirmed 2026-07-31, but the target is
	// still computed as the literal minimum, not left alone just because
	// -1 already happened to be safe).
	s.PendingEdits[ids[3]] = &RowEdit{
		FieldChanges: map[string]FieldChange{"value": {From: 200, To: 0}},
		ItemChange:   crackedPot,
	}
	// rowE: NO item swap, NO price edit -- only an unrelated field
	// (quantity) is staged. Must produce nothing: price/item aren't being
	// touched, so there's nothing to (re)sync.
	s.PendingEdits[ids[4]] = &RowEdit{
		FieldChanges: map[string]FieldChange{"sellQuantity": {From: 1, To: -1}},
	}

	got, err := s.BuildEquipParamEdits()
	if err != nil {
		t.Fatal(err)
	}

	goodsEdits := got["EquipParamGoods.param"]
	if len(goodsEdits) != 2 {
		t.Fatalf("EquipParamGoods.param edits = %d, want exactly 2 (Rune Arc + Cracked Pot): %+v", len(goodsEdits), goodsEdits)
	}
	goodsByRow := map[int64]string{}
	for _, e := range goodsEdits {
		goodsByRow[e.RowID] = string(e.Fields["sellValue"])
	}
	if got := goodsByRow[190]; got != "100" {
		t.Errorf("Rune Arc (190) sellValue = %q, want \"100\" (min of rowA=500/rowB=100, still below every untouched vanilla vendor)", got)
	}
	if got := goodsByRow[9500]; got != "0" {
		t.Errorf("Cracked Pot (9500) sellValue = %q, want \"0\" (rowD's staged price, below every untouched vanilla vendor)", got)
	}

	weaponEdits := got["EquipParamWeapon.param"]
	if len(weaponEdits) != 1 {
		t.Fatalf("EquipParamWeapon.param edits = %d, want exactly 1 (Dagger): %+v", len(weaponEdits), weaponEdits)
	}
	if weaponEdits[0].RowID != 1000000 {
		t.Errorf("edit targets equip id %d, want 1000000 (Dagger)", weaponEdits[0].RowID)
	}
	if got := weaponEdits[0].Fields["sellValue"]; got != "400" {
		t.Errorf("Dagger (1000000) sellValue = %q, want \"400\" (raised from its current 100 -- no vendor sells it below 400 anymore, rowC's own 500 doesn't constrain it)", got)
	}
}

// TestGateActionSpecEniaDisabled covers the fix for a leftover bug
// (2026-07-30, re-verified 2026-08-03 when the informational gate-status
// line was removed from the row-edit form): Enia's rows are genuinely
// gated (UnlockFlag != 0), but her flag ids alias real boss-defeat flags
// (see eniaMerchantName's doc comment), so the Unlock/Lock button itself
// -- not just descriptive text -- must stay disabled for her, in both the
// single-row and multi-select cases, regardless of character selection.
func TestGateActionSpecEniaDisabled(t *testing.T) {
	s := newTestState(nil)
	s.lastMerchant = eniaMerchantName
	s.SelectedChar = -1
	row := &catalog.Row{RowID: 101503, UnlockFlag: 9107}

	if _, enabled, _ := s.gateActionSpec([]*catalog.Row{row}); enabled {
		t.Error("Enia single-row gate button must stay disabled")
	}
	row2 := &catalog.Row{RowID: 101504, UnlockFlag: 9108}
	if _, enabled, _ := s.gateActionSpec([]*catalog.Row{row, row2}); enabled {
		t.Error("Enia multi-select gate button must stay disabled")
	}

	// A non-Enia merchant's row must be unaffected: with no character
	// selected the button now stays enabled (unlocks for every character),
	// per the 2026-08-03 request.
	s.lastMerchant = "Twin Maiden Husks"
	label, enabled, _ := s.gateActionSpec([]*catalog.Row{row})
	if !enabled {
		t.Error("non-Enia gate button with no character selected must be enabled")
	}
	if !strings.Contains(label, "All Characters") {
		t.Errorf("non-Enia no-character-selected label = %q, want it to mention All Characters", label)
	}
}

// TestWeaponLevelInfoNonWeaponRow: a row whose current item isn't in the
// weapon table (e.g. a Good) is never level-eligible.
func TestWeaponLevelInfoNonWeaponRow(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	runeArc := findItem(t, cat, "Rune Arc")
	s := newTestState(cat)
	row := weaponRow(6, runeArc)

	if _, _, _, eligible := s.weaponLevelInfo(row, nil); eligible {
		t.Fatal("a Good-category item must not be level-eligible")
	}
}

// gatedRow fabricates a row with a real, nonzero price and a nonzero
// UnlockFlag -- the shape of a normal gated merchant row (e.g. Twin Maiden
// Husks' bell-bearing tiers) before any edit.
func gatedRow(rowID, price, unlockFlag int64) *catalog.Row {
	p := price
	return &catalog.Row{
		RowID:      rowID,
		UnlockFlag: unlockFlag,
		Price:      &p,
		Fields: map[string]int64{
			"value": price, "eventFlag_forRelease": unlockFlag,
		},
	}
}

// TestZeroPriceNeverTouchesGate covers the corrected understanding
// (2026-07-30 initial fix, reverted 2026-08-02): the flag is what gates the
// SLOT, not the item sitting in it, and must never be staged as a side
// effect of a price/item edit -- only internal/character (one character's own
// flag bit, reversible) may ever change unlock state. The original guard
// here was built on a since-corrected diagnosis: real cause of price-0 rows
// vanishing in-game was the item's sellValue (see docs/MERCHANT_DATA.md's
// 2026-07-31 entry), not the gate, and that's fixed independently now.
func TestZeroPriceNeverTouchesGate(t *testing.T) {
	s := newTestState(nil)
	row := gatedRow(101800, 1000, 1042369416)

	s.StageField(s.PendingEdits, row, "value", 0)

	e := s.PendingEdits[row.RowID]
	if e == nil {
		t.Fatal("no edit staged")
	}
	if _, ok := e.FieldChanges["eventFlag_forRelease"]; ok {
		t.Errorf("price edit must never touch eventFlag_forRelease, got %+v", e.FieldChanges)
	}
	if fc := e.FieldChanges["value"]; fc.To != 0 {
		t.Errorf("value not staged to 0: %+v", fc)
	}
}

// TestZeroPriceOnUngatedRowStillWorks: sanity check that removing the old
// gate guard didn't break plain price staging for a row with no gate at all.
func TestZeroPriceOnUngatedRowStillWorks(t *testing.T) {
	s := newTestState(nil)
	row := gatedRow(100500, 200, 0) // UnlockFlag 0 = ungated

	s.StageField(s.PendingEdits, row, "value", 0)

	e := s.PendingEdits[row.RowID]
	if e == nil {
		t.Fatal("no edit staged")
	}
	if _, ok := e.FieldChanges["eventFlag_forRelease"]; ok {
		t.Errorf("ungated row must not get a flag edit, got %+v", e.FieldChanges)
	}
	if fc := e.FieldChanges["value"]; fc.To != 0 {
		t.Errorf("value not staged to 0: %+v", fc)
	}
}

// TestItemSwapOntoZeroPricedGatedRowNeverTouchesGate covers the Enia
// "Forging" row shape specifically: price is ALREADY 0 in the row's own
// vanilla data and it's still flag-gated. An item swap here must NOT clear
// the gate -- Enia's flags must never be touched by anything in this app
// (they overlap boss-trigger flags, see docs/CHAR_UNLOCK.md), and the same
// rule now applies uniformly to every merchant, not just her.
func TestItemSwapOntoZeroPricedGatedRowNeverTouchesGate(t *testing.T) {
	s := newTestState(nil)
	row := gatedRow(101776, 0, 9130) // price already 0 in vanilla data, like Enia's Forging rows

	s.stageItemSwapCore(s.PendingEdits, row, sellableItem("Rune Arc", 190))

	e := s.PendingEdits[row.RowID]
	if e == nil {
		t.Fatal("no edit staged")
	}
	if _, ok := e.FieldChanges["eventFlag_forRelease"]; ok {
		t.Errorf("item swap must never touch eventFlag_forRelease, got %+v", e.FieldChanges)
	}
	if e.ItemChange == nil {
		t.Error("the item swap itself must still be staged")
	}
}

// --- Reset to Vanilla ---

// TestRowEditFromVanillaDiffFieldOnly: a plain field diff (no item change)
// stages exactly those fields, no ItemChange.
func TestRowEditFromVanillaDiffFieldOnly(t *testing.T) {
	row := noteRow() // Fields["value"] == 500
	d := &catalog.VanillaFieldDiff{RowID: row.RowID, Fields: map[string]int64{"value": 200}}

	e := rowEditFromVanillaDiff(row, "Merchant Kale", d)

	if e.Merchant != "Merchant Kale" {
		t.Errorf("Merchant = %q, want %q", e.Merchant, "Merchant Kale")
	}
	if e.ItemChange != nil {
		t.Fatal("ItemChange must be nil for a field-only diff")
	}
	fc, ok := e.FieldChanges["value"]
	if !ok {
		t.Fatal("value not staged")
	}
	if fc.From != 500 || fc.To != 200 {
		t.Errorf("FieldChanges[value] = %+v, want {From:500 To:200}", fc)
	}
}

// TestRowEditFromVanillaDiffItemChange: an item-identity diff stages a real
// ItemChange with the vanilla values, and deliberately leaves
// ClearOverrides empty (unlike a normal item swap) -- vanilla's own
// override values are restored via ordinary FieldChanges instead (see
// TestRowEditFromVanillaDiffPreservesRealOverrideValue).
func TestRowEditFromVanillaDiffItemChange(t *testing.T) {
	row := noteRow()
	d := &catalog.VanillaFieldDiff{
		RowID: row.RowID, Fields: map[string]int64{},
		ItemChanged: true, VanillaEquipID: 190, VanillaEquipType: 3,
		VanillaBaseEquipID: 190, VanillaLevel: 0,
		VanillaItemName: "Rune Arc", VanillaIconPath: "icons/rune_arc.png",
	}

	e := rowEditFromVanillaDiff(row, "", d)

	ic := e.ItemChange
	if ic == nil {
		t.Fatal("ItemChange not staged")
	}
	if ic.EquipID != 190 || ic.EquipType != 3 || ic.BaseEquipID != 190 {
		t.Errorf("ItemChange = %+v, want EquipID/EquipType/BaseEquipID = 190/3/190", ic)
	}
	if ic.ToName != "Rune Arc" {
		t.Errorf("ToName = %q, want %q", ic.ToName, "Rune Arc")
	}
	if len(ic.ClearOverrides) != 0 {
		t.Errorf("ClearOverrides = %v, want empty -- vanilla overrides are restored via FieldChanges, not forced to -1", ic.ClearOverrides)
	}
}

// TestRowEditFromVanillaDiffItemChangeUnresolvedName: an unresolvable
// vanilla item (e.g. a raw id items.json has no entry for) still stages
// the correct numeric fields, with a fallback display name instead of "".
func TestRowEditFromVanillaDiffItemChangeUnresolvedName(t *testing.T) {
	row := noteRow()
	d := &catalog.VanillaFieldDiff{
		RowID: row.RowID, Fields: map[string]int64{},
		ItemChanged: true, VanillaEquipID: 999999, VanillaEquipType: 3, VanillaBaseEquipID: 999999,
	}
	e := rowEditFromVanillaDiff(row, "", d)
	if e.ItemChange.ToName == "" {
		t.Error("ToName must not be empty even when the vanilla item name can't be resolved")
	}
	if e.ItemChange.EquipID != 999999 {
		t.Errorf("EquipID = %d, want 999999 (numeric fields must stay correct even when unresolved)", e.ItemChange.EquipID)
	}
}

// TestRowEditFromVanillaDiffPreservesRealOverrideValue: a diff carrying a
// non--1 vanilla override value (mirrors noteRow()'s own real shape --
// Kale's Physick Note genuinely has nameMsgId pinned in vanilla) round-trips
// through FieldChanges exactly, not forced to -1.
func TestRowEditFromVanillaDiffPreservesRealOverrideValue(t *testing.T) {
	row := noteRow()
	row.Fields["nameMsgId"] = -1 // pretend the row's CURRENT state lost the vanilla pin
	d := &catalog.VanillaFieldDiff{RowID: row.RowID, Fields: map[string]int64{"nameMsgId": 8752}}

	e := rowEditFromVanillaDiff(row, "", d)

	fc, ok := e.FieldChanges["nameMsgId"]
	if !ok {
		t.Fatal("nameMsgId not staged")
	}
	if fc.From != -1 || fc.To != 8752 {
		t.Errorf("FieldChanges[nameMsgId] = %+v, want {From:-1 To:8752}", fc)
	}
}

// TestVanillaResetItemChangeAttributesCorrectSellValueRef is the direct
// regression guard for the bug a design-review pass caught: staging an
// item-identity reset as raw FieldChanges (instead of a real ItemChange)
// would make equipParamRefForEdit fall back to the row's CURRENT
// (pre-reset) equipId/equipType, silently attributing the reset row's new
// price to the WRONG item's sellValue bookkeeping. Confirms
// rowEditFromVanillaDiff's ItemChange-based approach resolves to the
// vanilla target instead.
func TestVanillaResetItemChangeAttributesCorrectSellValueRef(t *testing.T) {
	row := noteRow()
	row.Fields["equipId"] = 8600 // the row's CURRENT (pre-reset) item
	row.Fields["equipType"] = 3
	row.Fields["value"] = 500

	d := &catalog.VanillaFieldDiff{
		RowID: row.RowID, Fields: map[string]int64{"value": 200},
		ItemChanged: true, VanillaEquipID: 190, VanillaEquipType: 3, VanillaBaseEquipID: 190,
		VanillaItemName: "Rune Arc",
	}
	e := rowEditFromVanillaDiff(row, "", d)

	ref, price, ok := equipParamRefForEdit(row, e)
	if !ok {
		t.Fatal("equipParamRefForEdit reported ok=false -- the sellValue pipeline would never reconsider this row's item at all")
	}
	if ref != (equipParamRef{EquipType: 3, EquipID: 190}) {
		t.Errorf("ref = %+v, want the VANILLA item {3 190} -- got the row's pre-reset current item instead", ref)
	}
	if price != 200 {
		t.Errorf("price = %d, want 200 (the vanilla-reset price)", price)
	}
}

// TestResetToVanillaNoOpOnPristineFixtureClearsFlagEdits: on the pristine
// vanilla fixture, ResetToVanilla stages nothing into PendingEdits (nothing
// differs from vanilla) but still discards any staged-but-unsaved
// character-flag edits -- flags already saved to the file can't be
// reverted, only in-session staged toggles can.
func TestResetToVanillaNoOpOnPristineFixtureClearsFlagEdits(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.PendingFlagEdits[0] = map[int64]bool{123: true}
	s.PendingBellBearingEdits[0] = map[uint32]bool{456: true}
	s.PendingEdits[999] = &RowEdit{Label: "stale staged edit"}

	n, err := s.ResetToVanilla()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("ResetToVanilla() staged %d rows on the pristine vanilla fixture, want 0", n)
	}
	if len(s.PendingEdits) != 0 {
		t.Errorf("PendingEdits = %v, want empty (a prior staged batch must be REPLACED, not merged)", s.PendingEdits)
	}
	if len(s.PendingFlagEdits) != 0 {
		t.Errorf("PendingFlagEdits not cleared: %v", s.PendingFlagEdits)
	}
	if len(s.PendingBellBearingEdits) != 0 {
		t.Errorf("PendingBellBearingEdits not cleared: %v", s.PendingBellBearingEdits)
	}
}

// rowWithItem fabricates a merchant row selling a specific item -- used by
// the applyRowSwap tests below (merchant-grid internal drag-and-drop).
func rowWithItem(rowID int64, itemName string, equipID, level int64) *catalog.Row {
	return &catalog.Row{
		RowID:    rowID,
		ItemName: itemName,
		IconPath: "items/tools/" + itemName + ".png",
		Level:    level,
		Fields: map[string]int64{
			"equipId": equipID + level, "equipType": 0,
			"iconId": -1, "nameMsgId": -1, "menuTitleMsgId": -1, "menuIconId": -1,
		},
	}
}

// TestApplyRowSwapExchangesIdentity: dragging row A onto row B stages each
// row with the OTHER's current item, both directions in one call.
func TestApplyRowSwapExchangesIdentity(t *testing.T) {
	s := newTestState(nil)
	rowA := rowWithItem(101800, "Rune Arc", 190, 0)
	rowB := rowWithItem(101801, "Golden Rune [1]", 200, 0)

	s.applyRowSwap(rowA, rowB)

	eA, eB := s.PendingEdits[rowA.RowID], s.PendingEdits[rowB.RowID]
	if eA == nil || eA.ItemChange == nil || eB == nil || eB.ItemChange == nil {
		t.Fatal("both rows must have a staged item swap")
	}
	if eA.ItemChange.ToName != "Golden Rune [1]" {
		t.Errorf("rowA.ToName = %q, want Golden Rune [1]", eA.ItemChange.ToName)
	}
	if eB.ItemChange.ToName != "Rune Arc" {
		t.Errorf("rowB.ToName = %q, want Rune Arc", eB.ItemChange.ToName)
	}
	if eA.ItemChange.EquipID != 200 {
		t.Errorf("rowA.EquipID = %d, want 200 (rowB's equipId)", eA.ItemChange.EquipID)
	}
	if eB.ItemChange.EquipID != 190 {
		t.Errorf("rowB.EquipID = %d, want 190 (rowA's equipId)", eB.ItemChange.EquipID)
	}
}

// TestApplyRowSwapSameRowIsNoop: dropping a drag back onto its own source
// cell must not corrupt PendingEdits.
func TestApplyRowSwapSameRowIsNoop(t *testing.T) {
	s := newTestState(nil)
	row := rowWithItem(101800, "Rune Arc", 190, 0)

	s.applyRowSwap(row, row)

	if len(s.PendingEdits) != 0 {
		t.Errorf("PendingEdits = %v, want empty (same-row swap must be a no-op)", s.PendingEdits)
	}
}

// TestApplyRowSwapUsesEffectiveIdentity: swapping a row that already has a
// staged swap must hand the OTHER row its CURRENT (staged) item, not the
// stale on-disk one -- effectiveRowIdentity's whole reason to exist.
func TestApplyRowSwapUsesEffectiveIdentity(t *testing.T) {
	s := newTestState(nil)
	rowA := rowWithItem(101800, "Rune Arc", 190, 0)
	rowB := rowWithItem(101801, "Golden Rune [1]", 200, 0)
	rowC := rowWithItem(101802, "Cracked Pot", 300, 0)

	s.applyRowSwap(rowA, rowB) // A<->B: A now shows Golden Rune [1], B shows Rune Arc
	s.applyRowSwap(rowA, rowC) // A<->C: A must hand C "Golden Rune [1]" (its CURRENT item), not "Rune Arc"

	eC := s.PendingEdits[rowC.RowID]
	if eC == nil || eC.ItemChange == nil {
		t.Fatal("rowC must have a staged item swap")
	}
	if eC.ItemChange.ToName != "Golden Rune [1]" {
		t.Errorf("rowC.ToName = %q, want Golden Rune [1] (rowA's CURRENT staged item)", eC.ItemChange.ToName)
	}
	eA := s.PendingEdits[rowA.RowID]
	if eA.ItemChange.ToName != "Cracked Pot" {
		t.Errorf("rowA.ToName = %q, want Cracked Pot", eA.ItemChange.ToName)
	}
}

// TestApplyRowSwapExchangesParams: a swap carries each item's price AND
// quantity to the other slot, not just the item identity (user: "we are
// swapping the items with all their params").
func TestApplyRowSwapExchangesParams(t *testing.T) {
	s := newTestState(nil)
	rowA := rowWithItem(101800, "Rune Arc", 190, 0)
	rowA.Fields["value"], rowA.Fields["sellQuantity"] = 100, 5
	rowB := rowWithItem(101801, "Golden Rune [1]", 200, 0)
	rowB.Fields["value"], rowB.Fields["sellQuantity"] = 300, -1

	s.applyRowSwap(rowA, rowB)

	eA, eB := s.PendingEdits[rowA.RowID], s.PendingEdits[rowB.RowID]
	if fc := eA.FieldChanges["value"]; fc.To != 300 {
		t.Errorf("rowA value.To = %d, want 300 (rowB's price)", fc.To)
	}
	if fc := eA.FieldChanges["sellQuantity"]; fc.To != -1 {
		t.Errorf("rowA sellQuantity.To = %d, want -1 (rowB's qty)", fc.To)
	}
	if fc := eB.FieldChanges["value"]; fc.To != 100 {
		t.Errorf("rowB value.To = %d, want 100 (rowA's price)", fc.To)
	}
	if fc := eB.FieldChanges["sellQuantity"]; fc.To != 5 {
		t.Errorf("rowB sellQuantity.To = %d, want 5 (rowA's qty)", fc.To)
	}
	if eA.ItemChange.ToName != "Golden Rune [1]" || eB.ItemChange.ToName != "Rune Arc" {
		t.Errorf("identity must still swap: A=%q B=%q", eA.ItemChange.ToName, eB.ItemChange.ToName)
	}
}

// TestApplyRowSwapSyncsDraftForSelectedRows: a swapped SOURCE row that is
// selected (so its grid display reads its draft, not PendingEdits) must show
// the swap immediately, not only after the selection later re-seeds the draft
// -- the "swapped slots don't update until I click somewhere" bug.
func TestApplyRowSwapSyncsDraftForSelectedRows(t *testing.T) {
	s := newTestState(nil)
	rowA := rowWithItem(1, "A", 10, 0)
	rowB := rowWithItem(2, "B", 20, 0)

	s.SelectedRowIDs = []int64{1} // rowA selected => in a draft session
	s.ensureDraft()               // seeds draftRowIDs=[1] from (empty) PendingEdits

	s.applyRowSwap(rowA, rowB)

	// The selected source row's effective (draft-backed) display must reflect
	// the swap right away.
	if e := s.effectiveRowEdit(1); e == nil || e.ItemChange == nil || e.ItemChange.ToName != "B" {
		t.Fatalf("selected source row's draft must reflect the swap, got %+v", e)
	}
	// The unselected target reads PendingEdits directly.
	if e := s.effectiveRowEdit(2); e == nil || e.ItemChange == nil || e.ItemChange.ToName != "A" {
		t.Fatalf("target row must show the swapped-in item, got %+v", e)
	}
}

// TestApplyRowSwapPayloadPositional: dragging a multi-selection onto the grid
// pairs sources with targets positionally -- first-selected with the drop
// cell, second with the next cell, and so on.
func TestApplyRowSwapPayloadPositional(t *testing.T) {
	s := newTestState(nil)
	rows := []*catalog.Row{
		rowWithItem(1, "A", 10, 0), rowWithItem(2, "B", 20, 0),
		rowWithItem(3, "C", 30, 0), rowWithItem(4, "D", 40, 0),
		rowWithItem(5, "E", 50, 0),
	}
	// Sources A(1),B(2) dropped at index 2 -> targets C(idx2),D(idx3).
	s.applyRowSwapPayload([]int64{1, 2}, rows, 2)

	want := map[int64]string{1: "C", 3: "A", 2: "D", 4: "B"}
	for id, name := range want {
		e := s.PendingEdits[id]
		if e == nil || e.ItemChange == nil || e.ItemChange.ToName != name {
			t.Errorf("row %d should now show %q, got %+v", id, name, e)
		}
	}
	if _, ok := s.PendingEdits[5]; ok {
		t.Errorf("row 5 (E) had no counterpart and must be untouched")
	}
}

// TestApplyRowSwapPayloadOverflowIgnored: sources with no target cell (the
// selection runs past the grid's end from the drop point) stay put.
func TestApplyRowSwapPayloadOverflowIgnored(t *testing.T) {
	s := newTestState(nil)
	rows := []*catalog.Row{
		rowWithItem(1, "A", 10, 0), rowWithItem(2, "B", 20, 0), rowWithItem(3, "C", 30, 0),
	}
	// Sources A(1),B(2) dropped at index 2: only C(idx2) is a valid target;
	// B's target (idx3) is off the end, so B overflows and is left untouched.
	s.applyRowSwapPayload([]int64{1, 2}, rows, 2)

	if e := s.PendingEdits[1]; e == nil || e.ItemChange.ToName != "C" {
		t.Errorf("row 1 (A) should show C, got %+v", e)
	}
	if e := s.PendingEdits[3]; e == nil || e.ItemChange.ToName != "A" {
		t.Errorf("row 3 (C) should show A, got %+v", e)
	}
	if _, ok := s.PendingEdits[2]; ok {
		t.Errorf("row 2 (B) overflowed and must be untouched, got %+v", s.PendingEdits[2])
	}
}

// TestOpenEditorAfterDrop covers the 2026-08-03 setting: dropping catalog
// items onto stock cells selects exactly the rows that were filled and
// opens the edit modal on them, but only with the setting on. Off is the
// default and must keep the pre-existing behavior (drop stages silently,
// selection untouched).
func TestOpenEditorAfterDrop(t *testing.T) {
	newRows := func() []*catalog.Row {
		return []*catalog.Row{
			{RowID: 1, Fields: map[string]int64{"equipId": 100, "equipType": 3}},
			{RowID: 2, Fields: map[string]int64{"equipId": 101, "equipType": 3}},
		}
	}
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	// Two real, sellable catalog item ids to drop.
	var ids []int64
	for _, it := range cat.ListItems("tools", "", "", nil) {
		if it.EquipID != nil {
			ids = append(ids, it.ID)
		}
		if len(ids) == 2 {
			break
		}
	}
	if len(ids) != 2 {
		t.Fatal("need 2 sellable catalog items for this test")
	}
	payload := fmt.Sprintf("%d,%d", ids[0], ids[1])

	t.Run("off by default", func(t *testing.T) {
		s := NewState(cat)
		rows := newRows()
		s.MerchantRows = rows
		s.applyDrop(payload, rows, 0)

		if len(s.PendingEdits) != 2 {
			t.Fatalf("drop staged %d edits, want 2", len(s.PendingEdits))
		}
		if len(s.SelectedRowIDs) != 0 {
			t.Errorf("SelectedRowIDs = %v, want untouched with the setting off", s.SelectedRowIDs)
		}
		if s.showRowEditor {
			t.Error("edit modal opened with the setting off")
		}
	})

	t.Run("on selects the filled rows and opens", func(t *testing.T) {
		s := NewState(cat)
		s.Settings.OpenEditorAfterDrop = true
		rows := newRows()
		s.MerchantRows = rows
		s.applyDrop(payload, rows, 0)

		if !s.showRowEditor {
			t.Error("edit modal did not open with the setting on")
		}
		if len(s.SelectedRowIDs) != 2 || s.SelectedRowIDs[0] != 1 || s.SelectedRowIDs[1] != 2 {
			t.Errorf("SelectedRowIDs = %v, want [1 2] (the rows the drop filled)", s.SelectedRowIDs)
		}
		// The draft must already reflect the staged swaps, so the modal's
		// first frame shows them rather than one frame of stale state.
		for _, id := range s.SelectedRowIDs {
			if e := s.draftEdits[id]; e == nil || e.ItemChange == nil {
				t.Errorf("row %d: draft has no staged swap on open (stale first frame)", id)
			}
		}
	})

	t.Run("material-locked rows are not selected", func(t *testing.T) {
		s := NewState(cat)
		s.Settings.OpenEditorAfterDrop = true
		rows := newRows()
		rows[0].MaterialLocked = true
		s.MerchantRows = rows
		s.applyDrop(payload, rows, 0)

		for _, id := range s.SelectedRowIDs {
			if id == 1 {
				t.Error("material-locked row 1 was selected; applyDrop skips it, so it holds no new item")
			}
		}
	})
}
