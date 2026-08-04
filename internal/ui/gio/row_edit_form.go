package gio

// Per-row edit form (a floating modal since 2026-08-03, see
// layoutRowEditOverlay) for the selected row(s): an item header, Change
// item / Undo swap buttons, price/quantity/level number inputs, and an
// unlock-gate action. Material-locked rows show an explanatory message
// instead of editable fields.
//
// Generalized to N selected rows (multi-select, state.go's SelectedRowIDs):
// a single row keeps the full icon/name header; N>1 rows collapse to a
// scrollable icon-only preview grid. The Price/Quantity/Level fields show
// their shared value when every selected row agrees, else blank with a
// "Mixed" placeholder -- committing then applies to every selected row (see
// syncFormEditors/handleFormEvents). The gate row is hidden for a
// multi-selection: different rows can carry different UnlockFlags, so there's
// no single "mixed lock state" to render.
//
// Every edit here lands in a DRAFT first (state.go's draftEdits/
// draftGateEdits), not directly in PendingEdits -- only Apply (applyDraft)
// commits, and closes the bar. The X button and clicking outside both close
// WITHOUT applying, discarding the draft. See state.go's draft doc comment.

import (
	"fmt"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/ui/gio/components"
)

// editingRows returns the currently selected merchant rows, in
// SelectedRowIDs' order (selection order, not grid order). An id no longer
// present in the current merchant's row cache (e.g. the merchant was
// switched with a stale selection) is silently dropped.
func (s *State) editingRows() []*catalog.Row {
	if len(s.SelectedRowIDs) == 0 {
		return nil
	}
	byID := make(map[int64]*catalog.Row, len(s.MerchantRows))
	for _, r := range s.MerchantRows {
		byID[r.RowID] = r
	}
	rows := make([]*catalog.Row, 0, len(s.SelectedRowIDs))
	for _, id := range s.SelectedRowIDs {
		if r := byID[id]; r != nil {
			rows = append(rows, r)
		}
	}
	return rows
}

// layoutRowEditOverlay draws the per-row edit form as a floating modal --
// full-window dim scrim + a centered bordered panel -- matching Pending
// Edits/Item Info exactly (2026-08-03 user request; previously a bar docked
// inline below the merchant grid, shrinking it to make room). Mirrors
// layoutItemInfoOverlay's shape via the shared components.Backdrop helper.
//
// Shown whenever one or more rows are selected AND showRowEditor is set
// (the "Edit (N)" button, see layoutStockCountRow) -- but NOT while
// item-picking: the old docked bar could stay visible-but-dimmed during a
// "Change item" pick because it lived in the merchant column, off to the
// side of the catalog grid the user must click; as a window-covering
// modal it would otherwise block that same catalog grid with its own
// scrim. Hidden entirely during Picking() instead (the catalog panel's own
// picking banner/Cancel button, catalog_panel.go, covers cancellation). A
// completed pick returns to the editor; cancelling ends the edit flow and
// discards its unapplied draft.
//
// Closes via its own X button, clicking the scrim outside the panel, or
// Apply (which commits first) -- all three end up at clearRowSelection,
// which also discards any unapplied draft -- see ensureDraft.
func (s *State) layoutRowEditOverlay(gtx layout.Context, th *material.Theme) {
	rows := s.editingRows()
	if len(rows) == 0 || !s.showRowEditor || s.Picking() {
		return
	}
	for s.rowEditScrim.Clicked(gtx) {
		s.clearRowSelection()
	}

	components.Backdrop(gtx,
		components.BackdropStyle{
			Scrim:        pendingModalScrim,
			PanelBg:      colorPanelBg,
			BorderColor:  colorDivider,
			BorderWidth:  unit.Dp(1),
			CornerRadius: unit.Dp(4),
			Inset:        unit.Dp(16),
		},
		&s.rowEditScrim,
		&s.rowEditPanelBlocker,
		&s.rowEditScrimPressTag,
		s.clearRowSelection,
		func(gtx *layout.Context) {
			// 720dp: wide enough for formActionsRow's 4 buttons on one line
			// even with the longest gate label ("Unlock all (All
			// Characters)"), with headroom for the button the user plans to
			// add. wrapButtons still wraps gracefully in a narrow window.
			maxW := gtx.Dp(unit.Dp(720))
			if avail := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(48)); avail < maxW {
				maxW = avail
			}
			gtx.Constraints.Max.X = maxW
			gtx.Constraints.Max.Y = gtx.Constraints.Max.Y * 8 / 10
		},
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return s.layoutRowEditPanel(gtx, th, rows)
		},
		func(gtx layout.Context) {
			s.pressListener(gtx, &s.rowEditModalTag, gtx.Constraints.Max, &s.rowEditModalHit)
			s.layoutFooterStatusOverlay(gtx, th)
		},
	)
}

// layoutRowEditPanel is the docked bar's inner content: a title (+ an
// item-count pill for a multi-selection) / X header, a gold accent rule,
// then the row edit form body. Apply lives in the body's own action row
// (formActionsRow); only X stays up here.
//
// Apply commits the draft AND closes the bar. The X button discards the
// draft and closes without committing -- clicking outside does the same.
// Before committing, Apply force-commits any price/quantity/level text
// sitting in an editor that was never Submitted (Enter never pressed) --
// see commitTypedFieldsBeforeApply -- otherwise a value typed but not
// Entered before clicking Apply would be silently dropped.
func (s *State) layoutRowEditPanel(gtx layout.Context, th *material.Theme, rows []*catalog.Row) layout.Dimensions {
	if s.applyDraftBtn.Clicked(gtx) {
		s.commitTypedFieldsBeforeApply(rows)
		s.applyDraft()
		s.clearRowSelection()
	}
	if s.closeDraftBtn.Clicked(gtx) {
		s.clearRowSelection()
	}

	// The raw row id means nothing to a player -- the merchant this row
	// belongs to is the meaningful identifier. (Debug mode used to title
	// this "Row N"; removed 2026-08-03.)
	title := s.lastMerchant
	titleChildren := []layout.FlexChild{
		layout.Rigid(material.H6(th, title).Layout),
	}
	if len(rows) > 1 {
		titleChildren = append(titleChildren,
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return countPill(gtx, th, fmt.Sprintf("%d items selected", len(rows)))
			}),
		)
	}

	header := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, titleChildren...)
		}),
		layout.Flexed(1, flexSpacer),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return barButton(th, &s.closeDraftBtn, "X").Layout(gtx)
		}),
	)
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return header }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(goldRule),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.layoutRowEditBody(gtx, th, rows) }),
	)
	return dims
}

// layoutRowEditBody renders the edit-form content for the selected row(s)
// (no surrounding chrome — the popup supplies the panel + header).
func (s *State) layoutRowEditBody(gtx layout.Context, th *material.Theme, rows []*catalog.Row) layout.Dimensions {
	if len(rows) == 1 && rows[0].MaterialLocked {
		row := rows[0]
		msg := material.Body2(th,
			"This is a material trade ("+materialSummary(row)+"), e.g. Enia's "+
				"Remembrance hand-ins. These rows are tied to quest/boss-reward "+
				"logic and are deliberately not editable.")
		msg.Color = colorMuted
		return msg.Layout(gtx)
	}

	// Process button clicks and number-input commits before rendering.
	s.handleFormEvents(gtx, rows)
	s.syncFormEditors(rows)

	maxLvl, levelEligible := s.batchLevelInfo(rows)

	fieldRows := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.formPriceRow(gtx, th, rows) }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.formQtyRow(gtx, th) }),
	}
	if levelEligible {
		fieldRows = append(fieldRows,
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.formLevelRow(gtx, th, maxLvl) }),
		)
	}
	fieldRows = append(fieldRows, s.formActionsRow(th, rows)...)

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.formHeader(gtx, th, rows) }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return groupBox(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, fieldRows...)
			})
		}),
	}
	// The gate status line (formGateRow) was removed 2026-08-03: the
	// Unlock/Lock button's own label now carries the state ("Unlock"/
	// "Lock" with a character selected, "Unlock (All Characters)"/
	// "Unlock all (All Characters)" without one, disabled for Enia) --
	// see formActionsRow/gateActionSpec.
	//
	// Data-quality warnings and the display-override note were shown here in
	// Debug mode; both are internal bookkeeping with no player-facing
	// meaning, so they went away with that mode (2026-08-03).
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// batchLevelInfo reports the tightest max reinforcement level among rows
// that are currently weapon-eligible (current-or-DRAFTED item), and
// whether at least one is. The shared Level field clamps live typing to
// this tighter cap so one typed value stays valid for every eligible row
// it applies to; StageItemLevel still independently re-clamps per row
// regardless, so a lower max here is a UX nicety, not a correctness
// requirement.
func (s *State) batchLevelInfo(rows []*catalog.Row) (maxLvl int, eligible bool) {
	for _, row := range rows {
		entry := s.draftEdits[row.RowID]
		_, _, m, ok := s.weaponLevelInfo(row, entry)
		if !ok || m == 0 {
			continue
		}
		eligible = true
		if maxLvl == 0 || m < maxLvl {
			maxLvl = m
		}
	}
	return maxLvl, eligible
}

// handleFormEvents processes the form's button clicks and number-input
// Enter/live commits for the selected row(s), applying scalar field commits
// to every row in the selection. Everything here writes into s.draftEdits
// (or the gate draft), NOT PendingEdits -- see applyDraft.
func (s *State) handleFormEvents(gtx layout.Context, rows []*catalog.Row) {
	if !s.Picking() && s.changeItemBtn.Clicked(gtx) {
		s.StartPicking(s.SelectedRowIDs)
	}
	if s.undoSwapBtn.Clicked(gtx) {
		for _, row := range rows {
			s.ClearItemSwap(s.draftEdits, row.RowID)
		}
	}
	if len(rows) == 1 && rows[0].UnlockFlag != 0 && s.gateBtn.Clicked(gtx) {
		s.toggleDraftGate(rows[0])
	}
	if s.unlockAllFormBtn.Clicked(gtx) {
		s.draftUnlockAll(rows)
	}
	if v, ok := liveClampedInt(gtx, &s.priceEditor, priceMin, priceMax); ok {
		for _, row := range rows {
			s.StageField(s.draftEdits, row, "value", v)
		}
	}
	if v, ok := liveClampedInt(gtx, &s.qtyEditor, qtyMin, qtyMax); ok {
		for _, row := range rows {
			s.StageField(s.draftEdits, row, "sellQuantity", v)
		}
	}
	// Always drain the level editor (even when not currently eligible/shown)
	// so no event lingers unconsumed into a frame where it becomes eligible
	// again; the clamp ceiling only matters while eligible so a fallback
	// max is harmless otherwise.
	maxLvl, eligible := s.batchLevelInfo(rows)
	clampMax := int64(maxLvl)
	if clampMax == 0 {
		clampMax = pickLevelMax
	}
	if v, ok := liveClampedInt(gtx, &s.levelFormEditor, 0, clampMax); ok && eligible {
		for _, row := range rows {
			s.StageItemLevel(s.draftEdits, row, v)
		}
	}
}

// draftEffectiveRowUnlocked resolves a row's unlocked/known state for the
// row-edit bar's OWN display: the gate draft when IT specifically targets
// row's UnlockFlag, else the real (committed) effectiveRowUnlocked.
func (s *State) draftEffectiveRowUnlocked(row *catalog.Row) (unlocked, known bool) {
	if edit, ok := s.draftGateEdits[row.UnlockFlag]; ok {
		return edit.target, true
	}
	return s.effectiveRowUnlocked(row.RowID)
}

// toggleDraftGate stages a gate-unlock DRAFT for row (only committed on
// Apply -- see applyDraft/setRowUnlockForSelectedChar), so an unclicked-Apply
// toggle reverts along with every other drafted change when the bar closes.
// !known with no character selected still proceeds (target ends up
// !unlocked == true, i.e. "Unlock" -- never "Lock" -- since there's no
// single per-character value to toggle off of, matching draftUnlockAll's
// own "always unlock, never lock" rule for the ambiguous case); !known
// otherwise (a character IS selected but the state couldn't be resolved)
// still no-ops, same as before.
func (s *State) toggleDraftGate(row *catalog.Row) {
	unlocked, known := s.draftEffectiveRowUnlocked(row)
	if !known && s.SelectedChar >= 0 {
		return
	}
	s.setDraftGate(row, !unlocked)
}

// draftUnlockAll drafts an unlock (target=true) for every distinct
// UnlockFlag among rows that's actually gated -- the multi-select bar's
// "Unlock all" button. A row with unknown state (no character selected)
// is still included, same "always unlock" exception toggleDraftGate
// applies. Committed only on Apply, same as a single row's Unlock/Lock
// toggle.
func (s *State) draftUnlockAll(rows []*catalog.Row) {
	for _, row := range rows {
		if row.UnlockFlag == 0 {
			continue
		}
		if _, known := s.draftEffectiveRowUnlocked(row); !known && s.SelectedChar >= 0 {
			continue
		}
		s.setDraftGate(row, true)
	}
}

// parseEditorInt parses an editor's current text as an integer, ignoring
// blank/lone-minus/unparsable text (the same "not a real value yet" cases
// commitInt and liveClampedInt already treat as no-ops).
func parseEditorInt(ed *widget.Editor) (int64, bool) {
	text := strings.TrimSpace(ed.Text())
	if text == "" || text == "-" {
		return 0, false
	}
	v, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// commitTypedFieldsBeforeApply force-commits whatever's currently typed in
// the price/quantity/level editors into the draft, as if each had just been
// Submitted (Enter pressed) -- commitInt/liveClampedInt otherwise only ever
// stage a value on that specific event, so clicking Apply right after
// typing a shared-field value (without an extra Enter first) silently
// dropped it. Called from Apply, right before applyDraft.
func (s *State) commitTypedFieldsBeforeApply(rows []*catalog.Row) {
	if v, ok := parseEditorInt(&s.priceEditor); ok {
		v = clampRange(v, priceMin, priceMax)
		for _, row := range rows {
			s.StageField(s.draftEdits, row, "value", v)
		}
	}
	if v, ok := parseEditorInt(&s.qtyEditor); ok {
		v = clampRange(v, qtyMin, qtyMax)
		for _, row := range rows {
			s.StageField(s.draftEdits, row, "sellQuantity", v)
		}
	}
	if maxLvl, eligible := s.batchLevelInfo(rows); eligible {
		if v, ok := parseEditorInt(&s.levelFormEditor); ok {
			clamped := clampLevel(v, maxLvl)
			for _, row := range rows {
				s.StageItemLevel(s.draftEdits, row, clamped)
			}
		}
	}
}

// rowIDsOf extracts row ids in rows' own order, for comparing against the
// form editors' last-seeded selection (formRowIDs).
func rowIDsOf(rows []*catalog.Row) []int64 {
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.RowID
	}
	return ids
}

func int64SliceEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// batchCommon reports whether every row in rows currently resolves to the
// SAME value via get (each row's current-or-drafted value), returning that
// value, or mixed=true if they disagree. rows is never empty here --
// callers only run this while at least one row is selected.
func batchCommon(rows []*catalog.Row, get func(*catalog.Row) int64) (val int64, mixed bool) {
	val = get(rows[0])
	for _, row := range rows[1:] {
		if get(row) != val {
			return 0, true
		}
	}
	return val, false
}

// syncFormEditors re-seeds the price/quantity/level editors when the
// selection changes, or when a computed shared value (or its mixed-ness)
// diverges from what the editors last showed (e.g. after Remove or a
// submit). A mixed field seeds an empty editor whose hint text reads
// "Mixed" (see labeledEditor) rather than any one row's value. User
// keystrokes never trigger a re-seed (the target value only moves on
// commit), so in-progress typing is preserved. Reads from s.draftEdits
// (the bar's own working copy), not PendingEdits.
func (s *State) syncFormEditors(rows []*catalog.Row) {
	priceOf := func(row *catalog.Row) int64 {
		v := int64(-1)
		if row.Price != nil {
			v = *row.Price
		}
		if e := s.draftEdits[row.RowID]; e != nil {
			if pc, ok := e.FieldChanges["value"]; ok {
				v = pc.To
			}
		}
		return v
	}
	qtyOf := func(row *catalog.Row) int64 {
		v := row.Quantity
		if e := s.draftEdits[row.RowID]; e != nil {
			if qc, ok := e.FieldChanges["sellQuantity"]; ok {
				v = qc.To
			}
		}
		return v
	}
	levelOf := func(row *catalog.Row) int64 {
		_, lvl, _, _ := s.weaponLevelInfo(row, s.draftEdits[row.RowID])
		return lvl
	}

	wantPrice, priceMixed := batchCommon(rows, priceOf)
	wantQty, qtyMixed := batchCommon(rows, qtyOf)
	wantLevel, levelMixed := batchCommon(rows, levelOf)

	ids := rowIDsOf(rows)
	if !int64SliceEqual(s.formRowIDs, ids) {
		seedNumEditor(&s.priceEditor, wantPrice, priceMixed)
		seedNumEditor(&s.qtyEditor, wantQty, qtyMixed)
		seedNumEditor(&s.levelFormEditor, wantLevel, levelMixed)
		s.formRowIDs = ids
		s.formPrice, s.formPriceMixed = wantPrice, priceMixed
		s.formQty, s.formQtyMixed = wantQty, qtyMixed
		s.formLevel, s.formLevelMixed = wantLevel, levelMixed
		return
	}
	if wantPrice != s.formPrice || priceMixed != s.formPriceMixed {
		seedNumEditor(&s.priceEditor, wantPrice, priceMixed)
		s.formPrice, s.formPriceMixed = wantPrice, priceMixed
	}
	if wantQty != s.formQty || qtyMixed != s.formQtyMixed {
		seedNumEditor(&s.qtyEditor, wantQty, qtyMixed)
		s.formQty, s.formQtyMixed = wantQty, qtyMixed
	}
	if wantLevel != s.formLevel || levelMixed != s.formLevelMixed {
		seedNumEditor(&s.levelFormEditor, wantLevel, levelMixed)
		s.formLevel, s.formLevelMixed = wantLevel, levelMixed
	}
}

// seedEditorText sets ed's text and moves the caret to its end (SetText
// alone resets the caret to 0 -- liveClampedInt already does this
// explicitly for its own live-clamp rewrites; the seed helpers below were
// missing it, which could leave the caret sitting at the START of a
// freshly-seeded value).
func seedEditorText(ed *widget.Editor, text string) {
	ed.SetText(text)
	ed.SetCaret(len(text), len(text))
}

// seedNumEditor seeds ed with v's decimal text (caret at end via
// seedEditorText), or clears it for a mixed multi-selection so ed's "Mixed"
// hint shows instead of any one row's value. Shared by the price/quantity/
// level editors, which differed only in which editor they targeted.
func seedNumEditor(ed *widget.Editor, v int64, mixed bool) {
	if mixed {
		seedEditorText(ed, "")
		return
	}
	seedEditorText(ed, strconv.FormatInt(v, 10))
}

// gateActionSpec resolves formActionsRow's 2nd button: "Unlock all" for a
// multi-selection (always unlocks, never locks -- a mixed selection can
// have rows in different states already, so "lock all" would be
// ambiguous; enabled once a character is selected AND at least one
// selected row is still locked for them, OR no character is selected AND
// at least one row is gated -- see below), or a single row's own
// Unlock/Lock TOGGLE (can go either direction, matching the pre-existing
// single-row behavior) for exactly one selected row.
//
// Enia is checked FIRST and disables the button unconditionally,
// regardless of selection/character state -- her rows are genuinely
// gated (UnlockFlag != 0) but her flag ids alias real boss-defeat flags
// (see eniaMerchantName's doc comment), so this must be a real disabled
// state, not just the informational status line that used to cover it
// (removed -- see docs/EDITOR.md).
//
// With no character selected, a gated row's state is unknown (!known)
// per-character, but the button stays enabled anyway (user request
// 2026-08-03): clicking it unlocks that row for EVERY character at once
// (setRowUnlockForAllChars via setRowUnlockForSelectedChar), never
// "Lock" -- there's no single per-character value to toggle off of.
func (s *State) gateActionSpec(rows []*catalog.Row) (label string, enabled bool, btn *widget.Clickable) {
	if s.lastMerchant == eniaMerchantName {
		if len(rows) > 1 {
			return "Unlock all", false, &s.unlockAllFormBtn
		}
		return "Unlock", false, &s.gateBtn
	}
	if len(rows) > 1 {
		anyGated := false
		anyStillLocked := false
		for _, row := range rows {
			if row.UnlockFlag == 0 {
				continue
			}
			anyGated = true
			if unlocked, known := s.draftEffectiveRowUnlocked(row); known && !unlocked {
				anyStillLocked = true
				break
			}
		}
		enabled := s.SelectedChar >= 0 && anyStillLocked || s.SelectedChar < 0 && anyGated
		label := "Unlock all"
		if s.SelectedChar < 0 {
			label = "Unlock all (All Characters)"
		}
		return label, enabled, &s.unlockAllFormBtn
	}
	row := rows[0]
	if row.UnlockFlag == 0 {
		return "Unlock", false, &s.gateBtn
	}
	unlocked, known := s.draftEffectiveRowUnlocked(row)
	if !known {
		return "Unlock (All Characters)", s.SelectedChar < 0, &s.gateBtn
	}
	if unlocked {
		return "Lock", true, &s.gateBtn
	}
	return "Unlock", true, &s.gateBtn
}

// batchPriceLabel is priceFieldLabel for the selection's cost type when
// every row agrees, else a generic "Price" (a mixed-cost-type batch is rare
// -- most merchants price everything in runes -- so this is a cosmetic
// fallback, not a validation concern).
func batchPriceLabel(rows []*catalog.Row) string {
	ct := rows[0].CostType
	for _, r := range rows[1:] {
		if r.CostType != ct {
			return "Price"
		}
	}
	return priceFieldLabel(ct)
}

// Price/quantity live-clamp bounds. Price can't go negative; sellQuantity
// is capped to [-1, 255]. -1 is the game's unlimited-stock convention. A
// finite row's purchases are recorded by Elden Ring in an 8-bit stock event,
// so 256+ wraps and makes stock appear to be consumed incorrectly.
//
// priceMax is a practical UI ceiling, not the raw s32 field width: an
// in-game controlled test (docs/MERCHANT_DATA.md) showed real corruption
// well below s32 max -- prices around 1e9 render with no price text at all
// (purchase-gating logic still reads the real value correctly), and a 7th
// displayed digit visibly glitches the shop's footer icon. 999999 (6
// digits) is the largest value confirmed clean of both symptoms.
const (
	priceMin int64 = 0
	priceMax int64 = 999999
	qtyMin   int64 = -1
	qtyMax   int64 = 255
)

// batchPriceIcon resolves the row-edit bar's Price field icon to the
// selection's real per-cost-type icon (the same one the merchant grid's
// price footer already shows, via currencyIcon/currencyIconPath) --
// mirrors batchPriceLabel's own "mixed selection" handling: nil (no icon,
// labeledEditor already supports that) for an empty or cost-type-mixed
// selection, rather than a placeholder.
func batchPriceIcon(rows []*catalog.Row) layout.Widget {
	if len(rows) == 0 {
		return nil
	}
	ct := rows[0].CostType
	for _, r := range rows[1:] {
		if r.CostType != ct {
			return nil
		}
	}
	path := currencyIconPath(ct)
	return func(gtx layout.Context) layout.Dimensions {
		return fixedIcon(gtx, currencyIcon(path), fieldIconSize)
	}
}

// clampRange bounds v into [min, max].
func clampRange(v, min, max int64) int64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// liveClampedInt drains an editor's events and clamps the parsed value into
// [min, max] as the user types, not just on Enter -- typing past max (or
// below min, for editors whose Filter still allows "-") immediately
// rewrites the field to the clamped value instead of letting an out-of-range
// number sit there until Submit. Blank/lone-minus mid-edit text is left
// alone (not yet a number). Returns the latest valid value and whether
// anything changed this frame.
func liveClampedInt(gtx layout.Context, ed *widget.Editor, min, max int64) (int64, bool) {
	var val int64
	var got bool
	for {
		ev, ok := ed.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.ChangeEvent, widget.SubmitEvent:
		default:
			continue
		}
		text := strings.TrimSpace(ed.Text())
		if text == "" || text == "-" {
			continue
		}
		v, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			continue
		}
		clamped := clampRange(v, min, max)
		if clamped != v {
			s := strconv.FormatInt(clamped, 10)
			ed.SetText(s)
			ed.SetCaret(len(s), len(s))
		}
		val, got = clamped, true
	}
	return val, got
}
