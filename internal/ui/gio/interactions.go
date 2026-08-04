package gio

import (
	"fmt"
	"strconv"
	"strings"

	"gioui.org/widget"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
)

// --- shared selection-state primitives (used by both the merchant row and
// catalog item multi-select schemes below, which mirror each other's
// plain-replace/ctrl-toggle/shift-range click semantics but otherwise track
// separate fields, so only the pure id-slice logic -- not the anchor/
// invalidate/scheme-specific bookkeeping -- is actually shareable) ---

// selContains reports whether id is in sel.
func selContains(sel []int64, id int64) bool {
	for _, sid := range sel {
		if sid == id {
			return true
		}
	}
	return false
}

// selToggle adds id at the end (preserving order) or removes it.
func selToggle(sel []int64, id int64) []int64 {
	for i, sid := range sel {
		if sid == id {
			return append(sel[:i], sel[i+1:]...)
		}
	}
	return append(sel, id)
}

// --- merchant row multi-select (mirrors catalog multi-select below: same
// plain-replace / ctrl-toggle / shift-range scheme, see handleRowClick in
// merchant_panel.go) ---

// isRowSelected reports whether a row id is in the current selection.
func (s *State) isRowSelected(id int64) bool {
	return selContains(s.SelectedRowIDs, id)
}

// clearRowSelection empties the row selection and forgets the shift anchor.
func (s *State) clearRowSelection() {
	// While replacement picking is active, ordinary row-selection cleanup must
	// not close the picker. Its two deliberate exits are CancelPicking (the
	// Catalog button or a press outside the catalog), which also closes the
	// editor, and completing the queued replacement(s).
	if s.Picking() {
		return
	}
	s.SelectedRowIDs = nil
	s.rowSelAnchorSet = false
	s.PickingForRows = nil
	s.showRowEditor = false // no selection => editor closed; next selection reopens via the button
	// Do not leave an invisible draft alive until the next merchant-grid
	// layout. Closing or cancelling is an immediate rollback of everything
	// not yet applied to PendingEdits.
	s.draftRowIDs = nil
	s.draftEdits = make(map[int64]*RowEdit)
	s.draftGateEdits = nil
	s.invalidate()
}

// selectRowPlain replaces the selection with a single row and pins the
// anchor. Re-clicking the sole selected row deselects it.
func (s *State) selectRowPlain(id int64) {
	if len(s.SelectedRowIDs) == 1 && s.SelectedRowIDs[0] == id {
		s.clearRowSelection()
		return
	}
	s.SelectedRowIDs = []int64{id}
	s.rowSelAnchor, s.rowSelAnchorSet = id, true
	s.PickingForRows = nil
	s.invalidate()
}

// toggleSelectRow adds a row at the end (preserving order) or removes it,
// and moves the anchor to it.
func (s *State) toggleSelectRow(id int64) {
	s.SelectedRowIDs = selToggle(s.SelectedRowIDs, id)
	s.rowSelAnchor, s.rowSelAnchorSet = id, true
	s.PickingForRows = nil
	s.invalidate()
}

// selectRowRange replaces the selection with the contiguous run between the
// anchor and the clicked row in the current visible order, leaving the
// anchor unchanged. Falls back to a plain select if the anchor is
// missing/no longer visible.
func (s *State) selectRowRange(rows []*catalog.Row, clickedIdx int) {
	if !s.rowSelAnchorSet {
		s.selectRowPlain(rows[clickedIdx].RowID)
		return
	}
	anchorIdx := -1
	for i, r := range rows {
		if r.RowID == s.rowSelAnchor {
			anchorIdx = i
			break
		}
	}
	if anchorIdx < 0 {
		s.selectRowPlain(rows[clickedIdx].RowID)
		return
	}
	lo, hi := anchorIdx, clickedIdx
	if lo > hi {
		lo, hi = hi, lo
	}
	sel := make([]int64, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		sel = append(sel, rows[i].RowID)
	}
	s.SelectedRowIDs = sel
	// anchor unchanged
	s.PickingForRows = nil
	s.invalidate()
}

// --- catalog multi-select ---

// isSelected reports whether an item id is in the current selection.
func (s *State) isSelected(id int64) bool {
	return selContains(s.SelectedItems, id)
}

// clearSelection empties the selection and forgets the shift anchor.
func (s *State) clearSelection() {
	s.SelectedItems = nil
	s.selAnchorSet = false
	s.invalidate()
}

// selectPlain replaces the selection with a single item and pins the anchor.
// Re-clicking the sole selected item deselects it (same toggle the merchant
// grid has).
func (s *State) selectPlain(id int64) {
	if len(s.SelectedItems) == 1 && s.SelectedItems[0] == id {
		s.clearSelection()
		return
	}
	s.SelectedItems = []int64{id}
	s.selAnchor, s.selAnchorSet = id, true
	s.invalidate()
}

// dragIDCount is how many items a drag starting on id would carry.
func (s *State) dragIDCount(id int64) int {
	if s.isSelected(id) {
		return len(s.SelectedItems)
	}
	return 1
}

// toggleSelect adds an item at the end (preserving order) or removes it, and
// moves the anchor to it.
func (s *State) toggleSelect(id int64) {
	s.SelectedItems = selToggle(s.SelectedItems, id)
	s.selAnchor, s.selAnchorSet = id, true
	// invalidate() unconditionally (the pre-dedup remove branch didn't call
	// it, unlike its toggleSelectRow twin and this function's own add
	// branch -- an inconsistency, not a deliberate difference: every other
	// selection mutator here redraws immediately).
	s.invalidate()
}

// selectRange replaces the selection with the contiguous run between the anchor
// and clicked item in the current visible (filtered) order, leaving the anchor
// unchanged. Falls back to a plain select if the anchor is missing/invisible.
func (s *State) selectRange(items []*catalog.Item, clickedIdx int) {
	if !s.selAnchorSet {
		s.selectPlain(items[clickedIdx].ID)
		return
	}
	anchorIdx := -1
	for i, it := range items {
		if it.ID == s.selAnchor {
			anchorIdx = i
			break
		}
	}
	if anchorIdx < 0 {
		s.selectPlain(items[clickedIdx].ID)
		return
	}
	lo, hi := anchorIdx, clickedIdx
	if lo > hi {
		lo, hi = hi, lo
	}
	sel := make([]int64, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		if items[i].EquipType == nil {
			continue // unsellable items never enter the selection
		}
		sel = append(sel, items[i].ID)
	}
	s.SelectedItems = sel
	// anchor unchanged
	s.invalidate()
}

// itemByID resolves an items.json id to its Item, hidden items INCLUDED --
// it delegates to catalog.ItemByID rather than indexing ListItems, which
// omits hidden entries. A hidden item is only hidden from the browsable
// catalog GRID; a real shop row can still reference one (e.g. Twin Maiden
// Husks' Flask of Wondrous Physick), and everything reached from such a row
// -- the item-info popup above all -- must still resolve it. Indexing
// ListItems here silently returned nil for exactly those items, so
// right-clicking them opened nothing (user-reported 2026-08-03).
func (s *State) itemByID(id int64) *catalog.Item {
	return s.Catalog.ItemByID(id)
}

// dragPayload builds the comma-separated id payload for a drag beginning on
// item id: the whole selection when the dragged item is part of it, else just
// that item. The selection is never mutated here.
func (s *State) dragPayload(id int64) string {
	ids := []int64{id}
	if s.isSelected(id) {
		ids = s.SelectedItems
	}
	parts := make([]string, len(ids))
	for i, v := range ids {
		parts[i] = strconv.FormatInt(v, 10)
	}
	return strings.Join(parts, ",")
}

// rowDragPayload builds a merchant-cell drag's ordered row-id payload: the
// whole current selection (in selection order) when the dragged row is part
// of it -- so a group drag positionally swaps first-with-first, etc. -- else
// just the dragged row itself. Mirrors dragPayload for the catalog grid.
func (s *State) rowDragPayload(id int64) string {
	ids := []int64{id}
	if s.isRowSelected(id) {
		ids = s.SelectedRowIDs
	}
	parts := make([]string, len(ids))
	for i, v := range ids {
		parts[i] = strconv.FormatInt(v, 10)
	}
	return strings.Join(parts, ",")
}

// rowDragIDCount is how many rows a merchant-cell drag starting on id carries
// (the selection size when id is selected, else 1) -- drives the drag ghost's
// count badge and the drop-span highlight width.
func (s *State) rowDragIDCount(id int64) int {
	if s.isRowSelected(id) {
		return len(s.SelectedRowIDs)
	}
	return 1
}

// parseItemPayload decodes a comma-separated id payload back to ids, skipping
// any unparsable token.
func parseItemPayload(payload string) []int64 {
	var out []int64
	for _, tok := range strings.Split(payload, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if v, err := strconv.ParseInt(tok, 10, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// --- item-picking mode ---

// StartPicking enters item-picking mode for a snapshot of rowIDs (the
// selection at the moment "Change item" was clicked) -- a catalog pick
// fans out to all of them.
func (s *State) StartPicking(rowIDs []int64) {
	s.PickingForRows = append([]int64(nil), rowIDs...)
	s.setFooterStatus(s.pickingStatusMessage())
}

// CancelPicking abandons the replacement and closes the whole edit flow. A
// cancelled replacement must not reveal the row-edit window again: its draft
// has not been applied, so leaving it open would invite the user to continue
// an action they explicitly backed out of.
func (s *State) CancelPicking() {
	s.PickingForRows = nil
	s.clearRowSelection()
	s.setFooterStatus("Item replacement cancelled")
}

// Picking reports whether an item is currently being picked for one or more
// rows.
func (s *State) Picking() bool { return len(s.PickingForRows) > 0 }

func (s *State) pickingStatusMessage() string {
	n := len(s.PickingForRows)
	if n == 1 {
		return "Choose a catalog item to replace the selected stock item"
	}
	return fmt.Sprintf("Choose %d catalog items to replace the selected stock items", n)
}

// consumeNextPickingRow pops and returns the FRONT row still queued for a
// picked item (nil once the queue is empty), skipping any id no longer in
// the current merchant's row cache. Each catalog click consumes exactly one
// queued row -- picking N rows requires N picks, one pick never fans out to
// all of them; Picking() flips false once the queue empties, ending the
// dim-overlay automatically. Clicking the same item N times reproduces the
// "same item everywhere" result deliberately, one click at a time.
func (s *State) consumeNextPickingRow() *catalog.Row {
	byID := make(map[int64]*catalog.Row, len(s.MerchantRows))
	for _, r := range s.MerchantRows {
		byID[r.RowID] = r
	}
	for len(s.PickingForRows) > 0 {
		id := s.PickingForRows[0]
		s.PickingForRows = s.PickingForRows[1:]
		if r := byID[id]; r != nil {
			if s.Picking() {
				s.setFooterStatus(s.pickingStatusMessage())
			} else {
				s.setFooterStatus("Item replacement staged — review it in Pending before saving")
			}
			return r
		}
	}
	return nil
}

// --- row-edit bar draft ---

// ensureDraft reconciles the draft against the current selection every
// frame layoutMerchantPanel runs (called unconditionally, whether or not
// the bar is actually shown): the moment SelectedRowIDs no longer matches
// draftRowIDs -- a different selection, or the bar closed to none -- the
// old draft (applied or not) is discarded and a fresh one is seeded from
// PendingEdits for whatever's selected now. This single check is what
// makes "switch selection" and "close then reopen the same rows" both
// correctly drop an unapplied draft, with no separate close-handling
// needed anywhere selection-mutating code runs.
func (s *State) ensureDraft() {
	ids := append([]int64(nil), s.SelectedRowIDs...)
	if int64SliceEqual(s.draftRowIDs, ids) {
		return
	}
	s.draftRowIDs = ids
	s.reseedDraft()
}

// reseedDraft rebuilds draftEdits as a deep copy of PendingEdits for every
// row in draftRowIDs, and clears the gate draft. Deep copies (not shared
// pointers) so mutating the draft can never corrupt an already-committed
// PendingEdits entry -- applyDraft hands draft entries over to PendingEdits
// directly, then calls this again to start the next round from a clean
// copy of what was just committed.
func (s *State) reseedDraft() {
	s.draftEdits = make(map[int64]*RowEdit, len(s.draftRowIDs))
	for _, id := range s.draftRowIDs {
		if e := s.PendingEdits[id]; e != nil {
			s.draftEdits[id] = copyRowEdit(e)
		}
	}
	s.draftGateEdits = nil
}

// copyRowEdit deep-copies a RowEdit so the draft never aliases a committed
// PendingEdits entry's maps/slices.
func copyRowEdit(e *RowEdit) *RowEdit {
	cp := *e
	cp.FieldChanges = make(map[string]FieldChange, len(e.FieldChanges))
	for k, v := range e.FieldChanges {
		cp.FieldChanges[k] = v
	}
	if e.ItemChange != nil {
		ic := *e.ItemChange
		ic.ClearOverrides = append([]string(nil), e.ItemChange.ClearOverrides...)
		cp.ItemChange = &ic
	}
	return &cp
}

// inDraftSession reports whether rowID belongs to the currently open
// row-edit bar's selection (so its DRAFT, not just PendingEdits, is the
// right thing to show -- see effectiveRowEdit).
func (s *State) inDraftSession(rowID int64) bool {
	for _, id := range s.draftRowIDs {
		if id == rowID {
			return true
		}
	}
	return false
}

// effectiveRowEdit resolves what should currently be DISPLAYED for a row
// anywhere outside the row-edit bar itself (the merchant grid's icon/
// border/tooltip): the in-progress draft when the row is part of the open
// bar session (a live preview of changes not yet applied -- and reverted
// if the bar closes without Apply), else the real, already-committed
// PendingEdits entry.
func (s *State) effectiveRowEdit(rowID int64) *RowEdit {
	if s.inDraftSession(rowID) {
		return s.draftEdits[rowID]
	}
	return s.PendingEdits[rowID]
}

// setDraftGate stages a gate-unlock draft for row's UnlockFlag (overwriting
// any prior draft for that same flag this session) -- the shared write path
// for both the single-row Unlock/Lock toggle and the multi-select "Unlock
// all" button (row_edit_form.go).
func (s *State) setDraftGate(row *catalog.Row, target bool) {
	if s.draftGateEdits == nil {
		s.draftGateEdits = make(map[int64]draftGateEdit)
	}
	s.draftGateEdits[row.UnlockFlag] = draftGateEdit{row: row, target: target}
}

// applyDraft commits the current draft into the real PendingEdits (item/
// field edits) and PendingFlagEdits (staged gate toggles, if any) --
// row_edit_form.go's Apply button. The caller (row_edit_form.go) closes the
// bar right after via clearRowSelection -- see its doc comment ("the apply
// button if pressed must close the window") -- so the reseed below mostly
// just leaves draftEdits/draftGateEdits in a clean state for that close
// rather than needing to support continued in-session editing.
func (s *State) applyDraft() {
	for _, id := range s.draftRowIDs {
		if e := s.draftEdits[id]; e != nil {
			s.PendingEdits[id] = e
		} else {
			delete(s.PendingEdits, id)
		}
	}
	for _, edit := range s.draftGateEdits {
		s.setRowUnlockForSelectedChar(edit.row, edit.target)
	}
	s.reseedDraft()
	if s.combinedPendingCount() > 0 {
		s.setFooterStatus("Edits staged — review them in Pending before saving")
	} else {
		s.setFooterStatus("")
	}
	s.invalidate()
}

// --- per-cell retained state ---

func (s *State) itemCell(id int64) *cellState {
	c := s.itemCells[id]
	if c == nil {
		c = &cellState{}
		s.itemCells[id] = c
	}
	return c
}

func (s *State) rowCell(id int64) *cellState {
	c := s.rowCells[id]
	if c == nil {
		c = &cellState{}
		s.rowCells[id] = c
	}
	return c
}

// formItemCell returns the retained hover/click state for one icon in the
// row-edit bar's multi-select preview grid (formMultiItemList) -- kept
// separate from rowCell (the main merchant grid's own per-row state) since
// a row shown in both places would otherwise need its widget.Clickable
// registered twice in the same frame.
func (s *State) formItemCell(id int64) *widget.Clickable {
	c := s.formItemCells[id]
	if c == nil {
		c = &widget.Clickable{}
		s.formItemCells[id] = c
	}
	return c
}

// merchantName parses an optional numeric row-count suffix ("Name (N)")
// back to the canonical name. Some real merchant locations include ordinary
// parentheses (for example "Nomadic Merchant - Caelid (Aeonia Swamp)"), so
// only a final, all-numeric parenthesized suffix is a display count.
func merchantName(label string) string {
	i := strings.LastIndex(label, " (")
	if i < 0 || !strings.HasSuffix(label, ")") {
		return label
	}
	if _, err := strconv.Atoi(label[i+2 : len(label)-1]); err == nil {
		return label[:i]
	}
	return label
}
