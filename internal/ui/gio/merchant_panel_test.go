package gio

// Pure state-machine tests for the Shop Editor grid's character-aware
// lock display, and for the merchant combo's row-count label (no window/
// frame loop involved).

import (
	"image"
	"reflect"
	"testing"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
)

func TestMerchantLabelRespectsShowMerchantRowCounts(t *testing.T) {
	m := catalog.Merchant{Name: "Patches", RowCount: 19, EditableRowCount: 19}
	if got := merchantLabel(m, false); got != "Patches" {
		t.Errorf("merchantLabel(showCounts=false) = %q, want %q", got, "Patches")
	}
	if got := merchantLabel(m, true); got != "Patches (19)" {
		t.Errorf("merchantLabel(showCounts=true) = %q, want %q", got, "Patches (19)")
	}
}

func TestMerchantNameStripsOnlyNumericCountSuffix(t *testing.T) {
	cases := []struct {
		label string
		want  string
	}{
		{"Patches (19)", "Patches"},
		{"Nomadic Merchant - Caelid (Aeonia Swamp)", "Nomadic Merchant - Caelid (Aeonia Swamp)"},
		{"Nomadic Merchant - Caelid (Aeonia Swamp) (8)", "Nomadic Merchant - Caelid (Aeonia Swamp)"},
	}
	for _, tc := range cases {
		if got := merchantName(tc.label); got != tc.want {
			t.Errorf("merchantName(%q) = %q, want %q", tc.label, got, tc.want)
		}
	}
}

func TestMerchantHeaderLabelCompactsGenericMerchantLocations(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Nomadic Merchant - Caelid (Aeonia Swamp)", "Nomadic — Aeonia Swamp"},
		{"Isolated Merchant - Academy of Raya Lucaria", "Isolated — Raya Lucaria"},
		{"Hermit Merchant - Mountaintops of the Giants", "Hermit — Mountaintops"},
		{"Patches", "Patches"},
	}
	for _, tc := range cases {
		if got := merchantHeaderLabel(tc.name); got != tc.want {
			t.Errorf("merchantHeaderLabel(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestMerchantStockFilterUsesTopLevelCategories guards the merchant grid
// against regressing to its old subcategory filter. The merchant panel must
// match the Catalog's broad categories so a player can isolate, for example,
// all Tools without knowing each item's narrower subtype.
func TestMerchantStockFilterUsesTopLevelCategories(t *testing.T) {
	s := &State{
		MerchantRows: []*catalog.Row{
			{RowID: 30, Category: "tools", SubCategory: "consumables"},
			{RowID: 10, Category: "ashes", SubCategory: "spirit_ashes"},
			{RowID: 20, Category: "tools", SubCategory: "other"},
		},
	}
	s.MerchantCategoryCombo.SetOptionsWithLabels(
		[]string{"", "tools", "ashes"},
		[]string{"All Categories", "Tools", "Spirit Ashes"},
	)
	s.MerchantCategoryCombo.SetValue("tools")

	rows := s.visibleRows()
	if len(rows) != 2 {
		t.Fatalf("visibleRows() returned %d rows for Tools, want 2", len(rows))
	}
	for _, row := range rows {
		if row.Category != "tools" {
			t.Errorf("category filter returned %q row, want only tools", row.Category)
		}
	}
	if rows[0].RowID != 20 || rows[1].RowID != 30 {
		t.Errorf("edit layout order = [%d %d], want RowID order [20 30]", rows[0].RowID, rows[1].RowID)
	}
}

// TestSyncMerchantsPreservesSelectionOnSettingToggle checks that flipping
// Settings.ShowMerchantRowCounts (no file reload involved) relabels the
// combo without losing the current selection or resetting the merchant
// row list -- only an actual path change should do either of those.
func TestSyncMerchantsPreservesSelectionOnSettingToggle(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.syncMerchants()
	if len(s.MerchantCombo.Options()) == 0 {
		t.Fatal("expected at least one merchant option after syncMerchants")
	}
	selected := s.MerchantCombo.Value()
	if selected == "" {
		t.Fatal("expected a merchant to be selected after syncMerchants")
	}
	rowsBefore := s.MerchantRows

	s.Settings.ShowMerchantRowCounts = true
	s.syncMerchants()

	if got := s.MerchantCombo.Value(); got != selected {
		t.Errorf("selection after toggling the setting = %q, want %q (preserved)", got, selected)
	}
	if len(s.MerchantRows) != len(rowsBefore) {
		t.Errorf("MerchantRows changed after a labels-only rebuild (len %d -> %d), want unchanged", len(rowsBefore), len(s.MerchantRows))
	}
}

// TestMerchantViewsKeepEditingStableAndPreviewGameOrder verifies the intended
// two-view workflow: Edit layout never moves a dropped row, while Game preview
// shows the exact order that a save/reload will produce in Elden Ring.
func TestMerchantViewsKeepEditingStableAndPreviewGameOrder(t *testing.T) {
	c := loadedTestCatalog(t)
	rows, err := c.MerchantRows("Twin Maiden Husks")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatal("Twin Maiden Husks fixture needs at least two rows")
	}

	s := newTestState(c)
	s.MerchantRows = rows
	editBefore := s.visibleRows()
	editBeforeIDs := make([]int64, len(editBefore))
	for i, row := range editBefore {
		editBeforeIDs[i] = row.RowID
	}
	target := rows[len(rows)-1]
	s.stageItemSwapCore(s.PendingEdits, target, sellableItem("Rune Arc", 190))

	editAfter := s.visibleRows()
	editAfterIDs := make([]int64, len(editAfter))
	for i, row := range editAfter {
		editAfterIDs[i] = row.RowID
	}
	if !reflect.DeepEqual(editBeforeIDs, editAfterIDs) {
		t.Fatalf("stable edit layout moved after a drop\nbefore: %v\nafter:  %v", editBeforeIDs, editAfterIDs)
	}

	s.merchantGamePreview = true
	preview := s.visibleRows()
	previewIDs := make([]int64, len(preview))
	runeArcIndex, daggerIndex := -1, -1
	for i, row := range preview {
		previewIDs[i] = row.RowID
		itemID, ok := s.merchantMenuItemID(row)
		if !ok || c.ItemByID(itemID) == nil {
			continue
		}
		switch c.ItemByID(itemID).Name {
		case "Rune Arc":
			runeArcIndex = i
		case "Dagger":
			daggerIndex = i
		}
	}
	if runeArcIndex < 0 || daggerIndex < 0 {
		t.Fatalf("fixture preview missing Rune Arc (%d) or Dagger (%d)", runeArcIndex, daggerIndex)
	}
	if runeArcIndex >= daggerIndex {
		t.Fatalf("Game preview must show Tools before weapons: Rune Arc index %d, Dagger index %d", runeArcIndex, daggerIndex)
	}

	out := t.TempDir() + "/merchant-menu-order.dat"
	if _, err := c.ApplyEdits(s.BuildEdits(), out); err != nil {
		t.Fatal(err)
	}
	saved, err := c.MerchantRows("Twin Maiden Husks")
	if err != nil {
		t.Fatal(err)
	}
	reloaded := newTestState(c)
	reloaded.MerchantRows = saved
	reloaded.merchantGamePreview = true
	savedPreview := reloaded.visibleRows()
	savedPreviewIDs := make([]int64, len(savedPreview))
	for i, row := range savedPreview {
		savedPreviewIDs[i] = row.RowID
	}
	if !reflect.DeepEqual(previewIDs, savedPreviewIDs) {
		t.Fatalf("Game preview differs from saved game order\npreview: %v\nsaved:   %v", previewIDs, savedPreviewIDs)
	}
}

// TestRowLockedForDisplayBindsToSelectedCharacter checks the states
// rowLockedForDisplay must distinguish: no character selected (v6:
// always locked -- restored after v5 had briefly dropped this, per user
// feedback that deselecting must make everything look locked again, not
// leave the previously-selected character's unlocks stuck showing as
// normal), a committed-unlocked row for the selected character (not
// locked), a committed-locked row with a staged-but-unsaved unlock (also
// not locked -- staging must be visible immediately, not just after
// Save), and re-deselecting that same character (locked again).
func TestRowLockedForDisplayBindsToSelectedCharacter(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	rowsByID, err := s.Catalog.RowsByID()
	if err != nil {
		t.Fatal(err)
	}

	var anyGated int64 = -1
	for id, r := range rowsByID {
		if r.UnlockFlag != 0 {
			anyGated = id
			break
		}
	}
	if anyGated < 0 {
		t.Fatal("no gated row found in fixture")
	}
	if !s.rowLockedForDisplay(rowsByID[anyGated]) {
		t.Error("expected a gated row to show locked with no character selected")
	}

	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()

	var lockedRowID, unlockedRowID int64 = -1, -1
	for rowID, unlocked := range s.charFlagState {
		if unlocked && unlockedRowID < 0 {
			unlockedRowID = rowID
		}
		if !unlocked && lockedRowID < 0 {
			lockedRowID = rowID
		}
	}
	if unlockedRowID >= 0 {
		if s.rowLockedForDisplay(rowsByID[unlockedRowID]) {
			t.Errorf("row %d is committed-unlocked for this character, want not locked for display", unlockedRowID)
		}
		// The exact bug reported: deselecting the character afterward
		// must make it show locked again, not stay looking normal.
		s.selectCharacter(7) // toggles off (selectCharacter's own convention)
		if !s.rowLockedForDisplay(rowsByID[unlockedRowID]) {
			t.Errorf("row %d unlocked for character 7 still shows unlocked after deselecting, want locked", unlockedRowID)
		}
		s.selectCharacter(7) // back on, for the rest of this test
	}
	if lockedRowID >= 0 {
		row := rowsByID[lockedRowID]
		if !s.rowLockedForDisplay(row) {
			t.Errorf("row %d is committed-locked, want locked for display", lockedRowID)
		}
		s.PendingFlagEdits[s.SelectedChar] = map[int64]bool{lockedRowID: true}
		if s.rowLockedForDisplay(row) {
			t.Errorf("row %d has a staged (unsaved) unlock, want not locked for display", lockedRowID)
		}
	}
	if lockedRowID < 0 && unlockedRowID < 0 {
		t.Skip("no gated row found for this character")
	}
}

// TestQtyBadgeText covers the merchant grid's corner stock-count badge:
// -1 (unlimited) must render blank -- no natural stack-count analog -- and
// every other value (including 0) renders as its plain number.
func TestQtyBadgeText(t *testing.T) {
	cases := []struct {
		qty  int64
		want string
	}{
		{-1, ""},
		{0, "0"},
		{1, "1"},
		{999, "999"},
	}
	for _, c := range cases {
		if got := qtyBadgeText(c.qty); got != c.want {
			t.Errorf("qtyBadgeText(%d) = %q, want %q", c.qty, got, c.want)
		}
	}
}

// TestCurrencyIconPath covers the on-cell price footer's currency icon
// resolution: costType 0 (runes, the overwhelming majority) and any
// unrecognized costType fall back to the rune icon; the rare named cost
// types (see field_meta.go's namedCostTypes) resolve to their own real item
// icon instead.
func TestCurrencyIconPath(t *testing.T) {
	cases := []struct {
		costType int64
		want     string
	}{
		{0, runeIconPath},
		{1, "items/key_items/dragon_heart.png"},
		{2, "items/tools/starlight_shards.png"},
		{5, "items/key_items/heart_of_bayle.png"},
		{99, runeIconPath}, // unrecognized -- fall back, don't guess
	}
	for _, c := range cases {
		if got := currencyIconPath(c.costType); got != c.want {
			t.Errorf("currencyIconPath(%d) = %q, want %q", c.costType, got, c.want)
		}
	}
}

// TestRowPriceQtyPrefersEffectiveEdit covers the merchant grid's price
// footer / qty badge reading through the same "effective" (staged draft, or
// committed PendingEdits) source the icon swap already uses, so a pending
// price/quantity edit shows up on the grid cell too -- not just once the
// row-edit bar is opened.
func TestRowPriceQtyPrefersEffectiveEdit(t *testing.T) {
	s := newTestState(nil)
	orig := int64(300)
	row := &catalog.Row{RowID: 1, Price: &orig, Quantity: 5}

	price, qty := s.rowPriceQty(row)
	if price == nil || *price != 300 || qty != 5 {
		t.Fatalf("no edit: got price=%v qty=%d, want 300/5", price, qty)
	}

	s.PendingEdits[1] = &RowEdit{FieldChanges: map[string]FieldChange{
		"value":        {From: 300, To: 999},
		"sellQuantity": {From: 5, To: -1},
	}}
	price, qty = s.rowPriceQty(row)
	if price == nil || *price != 999 || qty != -1 {
		t.Errorf("with pending edit: got price=%v qty=%d, want 999/-1", price, qty)
	}

	// A row with no price at all (raw -1, Price == nil) and no staged
	// override must stay nil, not silently become 0.
	rowNoPrice := &catalog.Row{RowID: 2, Price: nil, Quantity: 1}
	price, _ = s.rowPriceQty(rowNoPrice)
	if price != nil {
		t.Errorf("priceless row: got price=%v, want nil", price)
	}
}

// TestCellBorder covers the merchant cell's border precedence:
// selected > pending > has-warnings > none. Unlike its neighbors
// rowLockedForDisplay/rowPriceQty, cellBorder had no direct test. Returned
// pointers are the package-level border color vars, so pointer identity is
// the precise assertion. (No draft session is active, so effectiveRowEdit
// reads PendingEdits directly -- see inDraftSession.)
func TestCellBorder(t *testing.T) {
	// A hazard warning (not one of the nonHazardWarningPrefixes filtered out
	// of red-square treatment).
	const hazard = "equipType 5 has no known item-id offset"

	newState := func() *State { return newTestState(nil) }

	t.Run("none", func(t *testing.T) {
		s := newState()
		if got := s.cellBorder(&catalog.Row{RowID: 1}); got != nil {
			t.Errorf("plain row border = %v, want nil", got)
		}
	})

	t.Run("warn only", func(t *testing.T) {
		s := newState()
		row := &catalog.Row{RowID: 1, Warnings: []string{hazard}}
		if got := s.cellBorder(row); got != &borderWarn {
			t.Errorf("warned row border = %v, want &borderWarn", got)
		}
	})

	t.Run("pending does not hide warning", func(t *testing.T) {
		s := newState()
		row := &catalog.Row{RowID: 1, Warnings: []string{hazard}}
		s.PendingEdits[1] = &RowEdit{}
		if got := s.cellBorder(row); got != &borderWarn {
			t.Errorf("pending+warn row border = %v, want &borderWarn", got)
		}
	})

	t.Run("pending only uses marker not border", func(t *testing.T) {
		s := newState()
		row := &catalog.Row{RowID: 1}
		s.PendingEdits[1] = &RowEdit{}
		if got := s.cellBorder(row); got != nil {
			t.Errorf("pending row border = %v, want nil", got)
		}
	})

	t.Run("selected beats pending and warn", func(t *testing.T) {
		s := newState()
		row := &catalog.Row{RowID: 1, Warnings: []string{hazard}}
		s.PendingEdits[1] = &RowEdit{}
		s.SelectedRowIDs = []int64{1}
		if got := s.cellBorder(row); got != &borderSelected {
			t.Errorf("selected+pending+warn row border = %v, want &borderSelected", got)
		}
	})
}

// TestRowLockedForDisplayPreviewsUnappliedGateDraft: the grid's lock badge
// must react to an in-progress row-edit-bar gate DRAFT (Unlock/Lock or
// "Unlock all", not yet Applied), and revert the moment the draft is
// discarded -- user: "if i click unlock all the lock icon must disappear
// from the items, it should get back if i don't click apply thou."
func TestRowLockedForDisplayPreviewsUnappliedGateDraft(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	rowsByID, err := s.Catalog.RowsByID()
	if err != nil {
		t.Fatal(err)
	}
	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()

	var lockedRowID int64 = -1
	for rowID, unlocked := range s.charFlagState {
		if !unlocked {
			lockedRowID = rowID
			break
		}
	}
	if lockedRowID < 0 {
		t.Skip("no locked gated row found for character 7")
	}
	row := rowsByID[lockedRowID]
	if !s.rowLockedForDisplay(row) {
		t.Fatalf("setup: row %d expected locked before any draft", lockedRowID)
	}

	s.setDraftGate(row, true)
	if s.rowLockedForDisplay(row) {
		t.Errorf("row %d still shows locked with an unapplied unlock draft", lockedRowID)
	}
	if _, ok := s.PendingFlagEdits[s.SelectedChar][lockedRowID]; ok {
		t.Error("a draft must not touch PendingFlagEdits before Apply")
	}

	// Discarding the draft (bar closes without Apply) must bring the badge
	// back.
	s.draftGateEdits = nil
	if !s.rowLockedForDisplay(row) {
		t.Errorf("row %d should show locked again once the draft is discarded", lockedRowID)
	}
}

// TestRowEffectiveItemIDPrefersStagedSwap: the item-info popup must resolve
// a row with a pending item swap to the NEW item, not the stale on-disk one
// -- matching the grid's own icon/tooltip precedence (effectiveRowEdit).
func TestRowEffectiveItemIDPrefersStagedSwap(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState(cat)

	matches := cat.ListItems("", "", "Zweihander", nil)
	if len(matches) == 0 {
		t.Fatal("Zweihander not found in items.json")
	}
	zweihander := matches[0]

	staleID := int64(999999999) // deliberately not a real item id
	row := &catalog.Row{RowID: 100800, ItemID: &staleID}

	// No staged edit yet: falls back to the row's own ItemID.
	got, ok := s.rowEffectiveItemID(row)
	if !ok || got != staleID {
		t.Fatalf("rowEffectiveItemID (unstaged) = (%d, %v), want (%d, true)", got, ok, staleID)
	}

	s.PendingEdits[row.RowID] = &RowEdit{
		ItemChange: &ItemChange{
			ToName: zweihander.Name, EquipID: *zweihander.EquipID, EquipType: int64(*zweihander.EquipType),
		},
	}
	got, ok = s.rowEffectiveItemID(row)
	if !ok || got != zweihander.ID {
		t.Fatalf("rowEffectiveItemID (staged swap) = (%d, %v), want (%d, true) -- must resolve the NEW item", got, ok, zweihander.ID)
	}
}

// TestSelectionNeverOpensEditor: selecting rows (plain or Ctrl/Shift) must
// leave the docked edit form closed -- it opens only via the "Edit (N)"
// button (showRowEditor) so a selection stays usable for a drag-swap. And
// clearing the selection must close the form again.
func TestSelectionNeverOpensEditor(t *testing.T) {
	s := &State{}

	s.selectRowPlain(1)
	if s.showRowEditor {
		t.Error("a plain-click selection must NOT open the edit form")
	}
	s.toggleSelectRow(2) // ctrl-click adds a second row
	if s.showRowEditor {
		t.Error("a Ctrl/Shift selection must NOT open the edit form")
	}

	// The "Edit (N)" button opens it; clearing the selection closes it.
	s.showRowEditor = true
	s.clearRowSelection()
	if s.showRowEditor {
		t.Error("clearRowSelection must close the edit form")
	}
	if len(s.SelectedRowIDs) != 0 {
		t.Errorf("clearRowSelection must empty the selection, got %v", s.SelectedRowIDs)
	}
}

func TestCancelPickingClosesEditorAndClearsSelection(t *testing.T) {
	s := &State{
		SelectedRowIDs: []int64{10, 20},
		showRowEditor:  true,
		PickingForRows: []int64{10, 20},
		draftRowIDs:    []int64{10, 20},
		draftEdits:     map[int64]*RowEdit{10: {Label: "unapplied"}},
	}

	s.CancelPicking()

	if s.Picking() {
		t.Error("CancelPicking must leave replacement-picking mode")
	}
	if s.showRowEditor {
		t.Error("CancelPicking must close the row-edit window")
	}
	if len(s.SelectedRowIDs) != 0 {
		t.Errorf("CancelPicking must clear the edit selection, got %v", s.SelectedRowIDs)
	}
	if len(s.draftRowIDs) != 0 || len(s.draftEdits) != 0 || len(s.draftGateEdits) != 0 {
		t.Error("CancelPicking must discard every unapplied draft edit")
	}
	if got, want := s.footerStatusMessage(), "Open a save file to begin"; got != want {
		t.Errorf("footer status after cancellation = %q, want %q", got, want)
	}
}

// TestWrapButtonsNeverSqueezes locks in the 2026-08-03 fix for the row-edit
// modal's action row: a plain horizontal Flex of Rigids squeezes its
// TRAILING children when they overrun the available width, which rendered
// "Apply" as an unreadable sliver (user screenshot). wrapButtons must
// instead keep every child at its natural width and wrap to another line.
//
// A dimension-assertion test, not a "doesn't panic" one -- this bug family
// produces a visually broken but crash-free layout (see docs/EDITOR.md's
// panelSurface/flexSpacer history).
func TestWrapButtonsNeverSqueezes(t *testing.T) {
	const (
		btnW = 200
		btnH = 30
		gap  = 8
	)
	fixed := func(gtx layout.Context) layout.Dimensions {
		// A child that reports a fixed natural size regardless of
		// constraints -- stands in for a real button, and makes any
		// squeezing by the layout detectable as a size mismatch.
		return layout.Dimensions{Size: image.Pt(btnW, btnH)}
	}
	children := []layout.Widget{fixed, fixed, fixed, fixed}

	cases := []struct {
		name      string
		availW    int
		wantLines int
	}{
		// 4x200 + 3x8 gaps = 824
		{"all on one line", 900, 1},
		{"wraps to two lines", 500, 2},  // 2 per line: 408 fits, 616 doesn't
		{"wraps to four lines", 210, 4}, // only 1 fits per line
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gtx := headlessGtx()
			gtx.Constraints.Max.X = tc.availW
			gtx.Constraints.Min = image.Point{}

			dims := wrapButtons(gtx, unit.Dp(gap), children)

			wantH := tc.wantLines*btnH + (tc.wantLines-1)*gap
			if dims.Size.Y != wantH {
				t.Errorf("height = %d, want %d (%d line(s) of %ddp + %ddp gaps) -- "+
					"wrong line count means children were squeezed onto fewer lines instead of wrapping",
					dims.Size.Y, wantH, tc.wantLines, btnH, gap)
			}
			if dims.Size.X != tc.availW {
				t.Errorf("width = %d, want the full available %d", dims.Size.X, tc.availW)
			}
		})
	}
}

// TestFormActionsRowFitsModalWidth: at the row-edit modal's real width, all
// four action buttons must fit on ONE line even with the longest gate label
// ("Unlock all (All Characters)", the no-character-selected multi-select
// case) -- the configuration that overflowed before the fix. Guards the
// modal-width/label-length relationship, which is otherwise easy to break by
// editing either side alone.
func TestFormActionsRowFitsModalWidth(t *testing.T) {
	th := material.NewTheme()
	s := newTestState(nil)
	s.lastMerchant = "Twin Maiden Husks"
	s.SelectedChar = -1 // drives the longest ("All Characters") labels
	rows := []*catalog.Row{
		{RowID: 1, UnlockFlag: 100},
		{RowID: 2, UnlockFlag: 101},
	}
	if label, _, _ := s.gateActionSpec(rows); label != "Unlock all (All Characters)" {
		t.Fatalf("precondition: gate label = %q, want the longest variant", label)
	}

	// The modal's own content width: rowEditModalMaxWidth minus the
	// Backdrop's 16dp inset on each side, minus the fields groupBox's inset.
	gtx := headlessGtx()
	gtx.Constraints.Max.X = 720 - 2*16 - 2*12
	gtx.Constraints.Min = image.Point{}

	children := s.formActionsRow(th, rows)
	// The action row itself is the last FlexChild; lay the whole set out and
	// measure just it.
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	if dims.Size.Y > 80 {
		t.Errorf("action area height = %d, want a single button line (<=80): "+
			"the buttons wrapped, so they no longer fit the modal width", dims.Size.Y)
	}
}
