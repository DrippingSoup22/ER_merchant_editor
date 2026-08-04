package editor

// Edit staging: capturing per-row pending edits and flattening them into
// shopwrite.Edit records for write-back. All methods run on the UI goroutine
// (staging is only ever driven by frame-loop event handling), so they touch
// PendingEdits without locking. Ported from app/gui/window.py's EditorState
// (stage_field / stage_item_swap / clear_item_swap / remove_row_edits /
// apply_all's edit flattening / suggested_out_path).

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"er_merchant_editor/app/catalog"
	"er_merchant_editor/app/shopwrite"
)

// rowDisplayName is the human label for a row: item name (+ "+N" suffix for
// a leveled weapon, see catalog.Row.DisplayName), else the raw Paramdex
// label, else "Row N". Mirrors the Python's item_name/label/"Row {id}" fallback.
func rowDisplayName(row *catalog.Row) string {
	if row.ItemName != "" {
		return row.DisplayName()
	}
	if row.Label != "" {
		return row.Label
	}
	return fmt.Sprintf("Row %d", row.RowID)
}

// currentEntry returns a mutable copy of the row's staged field changes and
// its current item swap (nil if none) from edits, so callers can edit the
// copy and hand it to setRowEntry. Mirrors the Python's
// dict(entry.get("field_changes", {})). edits is whichever map is the
// current write target -- s.PendingEdits for the immediate paths (drag-and-
// drop), or s.draftEdits for the row-edit bar (see state.go's draft
// doc comment).
func (s *State) currentEntry(edits map[int64]*RowEdit, rowID int64) (map[string]FieldChange, *ItemChange) {
	e := edits[rowID]
	if e == nil {
		return map[string]FieldChange{}, nil
	}
	fc := make(map[string]FieldChange, len(e.FieldChanges))
	for k, v := range e.FieldChanges {
		fc[k] = v
	}
	return fc, e.ItemChange
}

// StageField stages (or unstages) a scalar field edit into edits. field is
// the raw param name ("value", "sellQuantity", "eventFlag_forRelease");
// From is captured from the row's decoded value. Staging a field back to
// its from-value unstages it (and drops the whole row entry if nothing else
// is staged).
func (s *State) StageField(edits map[int64]*RowEdit, row *catalog.Row, field string, to int64) {
	fc, ic := s.currentEntry(edits, row.RowID)
	stageFieldInto(fc, row, field, to)
	s.setRowEntry(edits, row, fc, ic)
}

// stageItemSwapCore stages an item swap for a row into edits, WITHOUT
// touching picking state — the shared core of the interactive pick and the
// drag-and-drop path. A nil EquipType/EquipID can't occur here (only
// sellable items reach it), but is guarded to a zero rather than panicking.
//
// If item is a weapon-table item, the shared catalog-wide PickLevel (see
// catalog_panel.go's level control) is applied, clamped to that item's own
// real max via catalog.MaxUpgradeLevel — so a level set for one weapon can't
// leak an out-of-range id onto a lower-max weapon picked/dragged next.
func (s *State) stageItemSwapCore(edits map[int64]*RowEdit, row *catalog.Row, item *catalog.Item) {
	fc, _ := s.currentEntry(edits, row.RowID)
	ic := &ItemChange{
		FromName:       rowDisplayName(row),
		ToName:         item.Name,
		IconPath:       item.IconPath,
		ClearOverrides: row.DisplayOverrides(),
	}
	if item.EquipID != nil {
		ic.EquipID = *item.EquipID
		ic.BaseEquipID = *item.EquipID
	}
	if item.EquipType != nil {
		ic.EquipType = int64(*item.EquipType)
	}
	if ic.EquipType == 0 {
		if maxLvl, ok := s.Catalog.MaxUpgradeLevel(ic.BaseEquipID); ok {
			ic.Level = clampLevel(s.PickLevel, maxLvl)
			ic.EquipID = ic.BaseEquipID + ic.Level
		}
	}
	s.applyAutoItemDefaults(fc, row)
	s.setRowEntry(edits, row, fc, ic)
}

// applyAutoItemDefaults stages price 0 and/or unlimited (-1) quantity into
// fc, per Settings.AutoFreeItems/AutoUnlimitedItems -- the starting point
// for a freshly swapped item, same as if the user had typed those values in
// right after via StageField (which is exactly what this mirrors: staging
// back to the row's own current value unstages it). The user's own
// subsequent edits, if any, simply overwrite fc[field] afterward same as
// always. 2026-07-31 user request.
func (s *State) applyAutoItemDefaults(fc map[string]FieldChange, row *catalog.Row) {
	if s.Settings.AutoFreeItems {
		stageFieldInto(fc, row, "value", 0)
	}
	if s.Settings.AutoUnlimitedItems {
		stageFieldInto(fc, row, "sellQuantity", -1)
	}
}

// stageFieldInto is StageField's from/to comparison, operating on an
// already-fetched fc map instead of round-tripping through setRowEntry --
// shared by StageField itself and applyAutoItemDefaults, which needs to
// stage more than one field before a single setRowEntry call.
func stageFieldInto(fc map[string]FieldChange, row *catalog.Row, field string, to int64) {
	from := row.Fields[field]
	if to == from {
		delete(fc, field)
		return
	}
	fc[field] = FieldChange{From: from, To: to}
}

// rowEditFromVanillaDiff builds the RowEdit Reset-to-Vanilla stages for one
// row from its catalog.VanillaFieldDiff. Deliberately builds a real
// ItemChange (not raw equipId/equipType FieldChanges) whenever the item
// identity differs -- equipParamRefForEdit (below) only ever reads the
// sellValue-recompute target item ref from ItemChange, falling back to the
// row's CURRENT (pre-reset) equipId/equipType otherwise; staging identity
// as raw fields would silently attribute the reset row's new price to the
// wrong item's sellValue bookkeeping, not just skip the recompute.
//
// Also deliberately does NOT set ItemChange.ClearOverrides (unlike a normal
// item swap, stageItemSwapCore) -- vanilla data sometimes pins a real
// non--1 override (e.g. "Note: Flask of Wondrous Physick" has a genuine
// vanilla nameMsgId). The 4 override fields are diffed/staged as ordinary
// FieldChanges to their exact vanilla value instead, whether that value is
// -1 or not -- d.Fields already carries them for exactly this reason.
func rowEditFromVanillaDiff(row *catalog.Row, merchant string, d *catalog.VanillaFieldDiff) *RowEdit {
	fc := make(map[string]FieldChange, len(d.Fields))
	for field, to := range d.Fields {
		fc[field] = FieldChange{From: row.Fields[field], To: to}
	}
	var ic *ItemChange
	if d.ItemChanged {
		toName := d.VanillaItemName
		if toName == "" {
			toName = fmt.Sprintf("Row %d (vanilla)", d.RowID)
		}
		ic = &ItemChange{
			FromName:    rowDisplayName(row),
			ToName:      toName,
			IconPath:    d.VanillaIconPath,
			EquipID:     d.VanillaEquipID,
			EquipType:   d.VanillaEquipType,
			BaseEquipID: d.VanillaBaseEquipID,
			Level:       d.VanillaLevel,
		}
	}
	return &RowEdit{
		Label:        rowDisplayName(row),
		Merchant:     merchant,
		CostType:     row.CostType,
		IconPath:     row.IconPath,
		FieldChanges: fc,
		ItemChange:   ic,
	}
}

// ResetToVanilla replaces the ENTIRE PendingEdits batch with a save-wide
// revert to vanilla item/price/quantity/display-override values (see
// catalog.Catalog.VanillaDiffs -- already skips material-locked rows), and
// discards every currently staged (unsaved) character-flag edit: flags
// already committed to the save file in a past session cannot be reverted
// (no baseline is tracked for those), only in-session staged toggles are.
// Nothing is written to disk here -- same as every other staging path, an
// explicit Save is still required. 2026-07-31 user request.
func (s *State) ResetToVanilla() (int, error) {
	diffs, err := s.Catalog.VanillaDiffs()
	if err != nil {
		return 0, err
	}
	rowsByID, err := s.Catalog.RowsByID()
	if err != nil {
		return 0, err
	}

	reset := make(map[int64]*RowEdit, len(diffs))
	for id, d := range diffs {
		row := rowsByID[id]
		if row == nil {
			continue
		}
		reset[id] = rowEditFromVanillaDiff(row, s.Catalog.MerchantForRow(id), d)
	}

	s.PendingEdits = reset
	s.PendingFlagEdits = make(map[int]map[int64]bool)
	s.PendingBellBearingEdits = make(map[int]map[uint32]bool)
	s.clearBulkUnlockUndo()
	s.draftEdits = make(map[int64]*RowEdit)
	s.draftRowIDs = nil
	s.clearRowSelection()
	s.PickingForRows = nil
	s.formRowIDs = nil
	s.pendingMerchantFilter = ""
	return len(reset), nil
}

// clampLevel bounds a reinforcement level to [0, max].
func clampLevel(level int64, max int) int64 {
	if level < 0 {
		return 0
	}
	if level > int64(max) {
		return int64(max)
	}
	return level
}

// weaponLevelInfo reports whether row is currently eligible for level
// editing (a staged swap to a weapon-table item, or -- with no swap staged
// -- the row's own current item being a weapon-table item), its base
// (+0) item id, and its real max level. entry is the row's current
// PendingEdits entry, or nil.
func (s *State) weaponLevelInfo(row *catalog.Row, entry *RowEdit) (baseItemID, level int64, maxLevel int, eligible bool) {
	if entry != nil && entry.ItemChange != nil {
		ic := entry.ItemChange
		if ic.EquipType != 0 {
			return 0, 0, 0, false
		}
		max, ok := s.Catalog.MaxUpgradeLevel(ic.BaseEquipID)
		if !ok {
			return 0, 0, 0, false
		}
		return ic.BaseEquipID, ic.Level, max, true
	}
	if row.ItemID == nil {
		return 0, 0, 0, false
	}
	max, ok := s.Catalog.MaxUpgradeLevel(*row.ItemID)
	if !ok {
		return 0, 0, 0, false
	}
	return *row.ItemID, 0, max, true
}

// StageItemLevel stages a reinforcement-level change for a row into edits,
// without requiring a fresh item pick: if a swap is already staged, its
// Level is adjusted in place; otherwise an ItemChange is synthesized from
// the row's own current item (so leveling a merchant's existing stock
// doesn't first require re-picking the same item). level is clamped to the
// item's real max (weaponLevelInfo); a no-op if the row isn't currently
// weapon-eligible.
func (s *State) StageItemLevel(edits map[int64]*RowEdit, row *catalog.Row, level int64) {
	fc, ic := s.currentEntry(edits, row.RowID)
	entry := edits[row.RowID]
	baseID, _, maxLvl, ok := s.weaponLevelInfo(row, entry)
	if !ok {
		return
	}
	level = clampLevel(level, maxLvl)

	if ic != nil {
		ic.Level = level
		ic.EquipID = ic.BaseEquipID + level
		s.setRowEntry(edits, row, fc, ic)
		return
	}
	if level == 0 {
		return // matches the row's current (unleveled) state, nothing to stage
	}
	ic = &ItemChange{
		FromName:       rowDisplayName(row),
		ToName:         rowDisplayName(row),
		IconPath:       row.IconPath,
		EquipType:      0,
		BaseEquipID:    baseID,
		EquipID:        baseID + level,
		Level:          level,
		ClearOverrides: row.DisplayOverrides(),
	}
	s.setRowEntry(edits, row, fc, ic)
}

// applyDrop stages item swaps for a payload of item ids dropped onto the
// visible merchant row at index i: rows are filled from i onward in order,
// material-locked rows are skipped WITHOUT consuming a payload item, and any
// excess payload is silently ignored. Picking state is untouched. Always
// stages directly into s.PendingEdits (immediate, not draft) -- drag-and-
// drop is a standalone action independent of whether the row-edit bar
// happens to be open.
func (s *State) applyDrop(payload string, rows []*catalog.Row, i int) {
	ids := parseItemPayload(payload)
	pi := 0
	var filled []int64
	for ri := i; ri < len(rows) && pi < len(ids); ri++ {
		if rows[ri].MaterialLocked {
			continue // skip locked rows without consuming an item
		}
		if item := s.itemByID(ids[pi]); item != nil {
			s.stageItemSwapCore(s.PendingEdits, rows[ri], item)
			s.syncDraftFromPending(rows[ri].RowID) // keep a selected target cell's display in sync
			filled = append(filled, rows[ri].RowID)
		}
		pi++
	}
	s.openEditorForDropped(filled)
}

// openEditorForDropped selects the rows a catalog drop just filled and
// opens the edit modal on them, when Settings.OpenEditorAfterDrop is on
// (user request 2026-08-03: a freshly dropped item usually still needs a
// price/quantity, so this saves a separate "Edit" click). No-op with the
// setting off, or when the drop staged nothing.
//
// Deliberately only wired to applyDrop (catalog item -> stock cell), not
// to applyRowSwapPayload: swapping two stock rows carries each row's
// existing price/quantity with it, so there's nothing new to fill in.
func (s *State) openEditorForDropped(rowIDs []int64) {
	if !s.Settings.OpenEditorAfterDrop || len(rowIDs) == 0 {
		return
	}
	s.SelectedRowIDs = append([]int64(nil), rowIDs...)
	s.rowSelAnchor, s.rowSelAnchorSet = rowIDs[len(rowIDs)-1], true
	s.showRowEditor = true
	// The draft is keyed to the selection snapshot; reconcile it against the
	// selection just set so the modal opens already showing the just-staged
	// swap, rather than one frame of stale state (ensureDraft would
	// otherwise only catch up on the next merchant-panel layout pass).
	s.ensureDraft()
	s.invalidate()
}

// rowIdentity is one row's current displayed item -- ItemChange's To fields
// if a swap is already staged/drafted for it (effectiveRowEdit), else the
// row's own decoded fields -- everything applyRowSwap needs to hand one
// row's current item to the other.
type rowIdentity struct {
	name, iconPath     string
	equipID, equipType int64
	baseEquipID, level int64
}

// effectiveRowIdentity resolves row's CURRENT displayed item (staged swap
// wins over the row's own on-disk value, same precedence the grid's icon/
// tooltip already use -- see merchant_panel.go's layoutRowGrid), for
// applyRowSwap to hand to the other side of a swap.
func (s *State) effectiveRowIdentity(row *catalog.Row) rowIdentity {
	if e := s.effectiveRowEdit(row.RowID); e != nil && e.ItemChange != nil {
		ic := e.ItemChange
		return rowIdentity{
			name: ic.ToName, iconPath: ic.IconPath,
			equipID: ic.EquipID, equipType: ic.EquipType,
			baseEquipID: ic.BaseEquipID, level: ic.Level,
		}
	}
	return rowIdentity{
		name: row.ItemName, iconPath: row.IconPath,
		equipID: row.Fields["equipId"], equipType: row.Fields["equipType"],
		baseEquipID: row.Fields["equipId"] - row.Level, level: row.Level,
	}
}

// swappableParamFields are the editable shop params carried WITH the item in
// a merchant-grid swap (user: "we are swapping the items with all their
// params" -- price and quantity travel with the item to its new slot). The
// per-slot unlock gate (eventFlag_forRelease) deliberately stays with the
// slot, not the item: it's structural gating cross-referenced by the
// character-unlock feature, not a price/stock attribute of the item itself.
var swappableParamFields = []string{"value", "sellQuantity"}

// effectiveFieldValue is field's CURRENT value for a row: its staged edit's
// To if one exists in fc, else the row's own decoded value -- the same
// staged-wins precedence effectiveRowIdentity/rowPriceQty use, so a swap
// carries a row's already-edited price/quantity, not the stale on-disk one.
func effectiveFieldValue(fc map[string]FieldChange, row *catalog.Row, field string) int64 {
	if c, ok := fc[field]; ok {
		return c.To
	}
	return row.Fields[field]
}

// rowSwapSnapshot is one row's full CURRENT content captured before any swap
// write mutates staging -- the item identity plus the effective swappable
// param values -- so a batch swap reads every source cleanly up front and
// overlapping source/target sets can't read half-mutated state.
type rowSwapSnapshot struct {
	id     rowIdentity
	fields map[string]int64
}

// snapshotRow captures a row's current effective item identity + swappable
// param values from PendingEdits.
func (s *State) snapshotRow(row *catalog.Row) rowSwapSnapshot {
	fc, _ := s.currentEntry(s.PendingEdits, row.RowID)
	fields := make(map[string]int64, len(swappableParamFields))
	for _, f := range swappableParamFields {
		fields[f] = effectiveFieldValue(fc, row, f)
	}
	return rowSwapSnapshot{id: s.effectiveRowIdentity(row), fields: fields}
}

// writeRowFromSnapshot stages row to fully become snap's content: the item
// identity as an ItemChange (row's own DisplayName/DisplayOverrides source
// FromName/ClearOverrides so a later re-swap still resets the right override
// fields) plus each swappable param staged to snap's value (stageFieldInto
// auto-unstages a field that already matches row's own on-disk value).
func (s *State) writeRowFromSnapshot(row *catalog.Row, snap rowSwapSnapshot) {
	fc, _ := s.currentEntry(s.PendingEdits, row.RowID)
	for _, f := range swappableParamFields {
		stageFieldInto(fc, row, f, snap.fields[f])
	}
	s.setRowEntry(s.PendingEdits, row, fc, &ItemChange{
		FromName: rowDisplayName(row), ToName: snap.id.name, IconPath: snap.id.iconPath,
		EquipID: snap.id.equipID, EquipType: snap.id.equipType,
		BaseEquipID: snap.id.baseEquipID, Level: snap.id.level,
		ClearOverrides: row.DisplayOverrides(),
	})
	s.syncDraftFromPending(row.RowID)
}

// syncDraftFromPending refreshes a row's DRAFT copy from its just-updated
// PendingEdits entry, but only when the row is part of the open draft session
// (a selected row). A drag-swap/drop commits straight to PendingEdits; the
// grid displays a SELECTED row from its draft, not PendingEdits
// (effectiveRowEdit), so without this the swapped source cells would keep
// showing their pre-swap item until the selection changed and reseedDraft
// ran -- the "not updated until I click somewhere" bug. Unselected rows read
// PendingEdits directly and need no sync.
func (s *State) syncDraftFromPending(rowID int64) {
	if !s.inDraftSession(rowID) {
		return
	}
	if e := s.PendingEdits[rowID]; e != nil {
		s.draftEdits[rowID] = copyRowEdit(e)
	} else {
		delete(s.draftEdits, rowID)
	}
}

// applyRowSwap stages a full content exchange between two merchant rows --
// item identity AND its swappable params (price/quantity) -- the merchant
// grid's internal drag-and-drop ("swap two shop items" per user request), as
// opposed to applyDrop's catalog-item-onto-row REPLACE. A no-op if src and
// dst are the same row (dropping a drag back onto its own source). Always
// stages directly into s.PendingEdits, same as applyDrop.
func (s *State) applyRowSwap(src, dst *catalog.Row) {
	if src.RowID == dst.RowID {
		return
	}
	srcSnap, dstSnap := s.snapshotRow(src), s.snapshotRow(dst)
	s.writeRowFromSnapshot(src, dstSnap)
	s.writeRowFromSnapshot(dst, srcSnap)
}

// applyRowSwapPayload stages a MULTI-row positional swap: the ordered source
// rows (srcIDs, the drag's selection payload) are paired 1:1 with the visible
// target rows from the drop index forward -- first-selected with first
// target, and so on (user's own framing). Sources with no target counterpart
// (the selection is bigger than the run of cells from the drop point to the
// grid's end) simply stay put. Rows are already material-lock-filtered before
// they reach the grid, so target progression is a plain index walk.
func (s *State) applyRowSwapPayload(srcIDs []int64, rows []*catalog.Row, dropIdx int) {
	byID := make(map[int64]*catalog.Row, len(rows))
	for _, r := range rows {
		byID[r.RowID] = r
	}
	pairs := make([][2]*catalog.Row, 0, len(srcIDs))
	for k, sid := range srcIDs {
		ti := dropIdx + k
		if ti >= len(rows) {
			break // overflow sources have no counterpart -- left untouched
		}
		if src := byID[sid]; src != nil {
			pairs = append(pairs, [2]*catalog.Row{src, rows[ti]})
		}
	}
	s.applyRowSwapBatch(pairs)
}

// applyRowSwapBatch stages every (src, dst) pair as a content swap. Every
// involved row is snapshotted from the pre-edit state FIRST, then all writes
// applied, so a row that is both a source and a target (overlapping selection
// and drop run) still swaps against the other rows' original content rather
// than a partially-rewritten state.
func (s *State) applyRowSwapBatch(pairs [][2]*catalog.Row) {
	snaps := make(map[int64]rowSwapSnapshot, len(pairs)*2)
	snap := func(row *catalog.Row) {
		if _, ok := snaps[row.RowID]; !ok {
			snaps[row.RowID] = s.snapshotRow(row)
		}
	}
	for _, p := range pairs {
		if p[0].RowID == p[1].RowID {
			continue
		}
		snap(p[0])
		snap(p[1])
	}
	for _, p := range pairs {
		if p[0].RowID == p[1].RowID {
			continue
		}
		s.writeRowFromSnapshot(p[0], snaps[p[1].RowID])
		s.writeRowFromSnapshot(p[1], snaps[p[0].RowID])
	}
}

// ClearItemSwap drops a row's staged item swap from edits, keeping any field
// changes (and dropping the whole entry if the swap was all that was staged).
func (s *State) ClearItemSwap(edits map[int64]*RowEdit, rowID int64) {
	e := edits[rowID]
	if e == nil {
		return
	}
	if len(e.FieldChanges) == 0 {
		delete(edits, rowID)
		return
	}
	e.ItemChange = nil
}

// RemoveRowEdits discards all of a row's staged edits. Also drops any
// in-progress row-edit-bar DRAFT for the same row -- the bar can be open on
// a row whose (already-committed) PendingEdits entry gets Removed from the
// Pending Edits modal at the same time (the modal draws on top but doesn't
// close the bar underneath); without this, clicking the bar's own Apply
// afterward would silently resurrect the edit this Remove just discarded.
func (s *State) RemoveRowEdits(rowID int64) {
	delete(s.PendingEdits, rowID)
	delete(s.draftEdits, rowID)
	s.clearFooterStatusWhenNoPending()
}

// RemoveMerchantEdits discards every staged edit for the given merchant
// group key (pendingMerchantKey, pending_edits.go) -- the Pending Edits
// modal's per-merchant "Remove all" button. Same draft-sync as
// RemoveRowEdits, for the same reason.
func (s *State) RemoveMerchantEdits(merchant string) {
	for id, e := range s.PendingEdits {
		if pendingMerchantKey(e) == merchant {
			delete(s.PendingEdits, id)
			delete(s.draftEdits, id)
		}
	}
	s.clearFooterStatusWhenNoPending()
}

// setRowEntry writes the row's staged edits into edits, removing the entry
// entirely when nothing is staged (mirrors the Python's _set_row_entry
// pruning).
func (s *State) setRowEntry(edits map[int64]*RowEdit, row *catalog.Row, fc map[string]FieldChange, ic *ItemChange) {
	if len(fc) == 0 && ic == nil {
		delete(edits, row.RowID)
		return
	}
	edits[row.RowID] = &RowEdit{
		Label:        rowDisplayName(row),
		Merchant:     s.lastMerchant,
		CostType:     row.CostType,
		IconPath:     row.IconPath,
		FieldChanges: fc,
		ItemChange:   ic,
	}
}

// PendingCount reports how many rows have staged edits.
func (s *State) PendingCount() int { return len(s.PendingEdits) }

// BuildEdits flattens the staged edits into shopwrite.Edit records, exactly like
// the Python's apply_all: each field change contributes its To value; an item
// change contributes equipId + equipType. Rows are emitted in ascending row-id
// order for determinism (write-back is order-independent across distinct rows).
func (s *State) BuildEdits() []shopwrite.Edit {
	ids := make([]int64, 0, len(s.PendingEdits))
	for id := range s.PendingEdits {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	edits := make([]shopwrite.Edit, 0, len(ids))
	for _, id := range ids {
		e := s.PendingEdits[id]
		fields := make(map[string]json.Number, len(e.FieldChanges)+2)
		for name, ch := range e.FieldChanges {
			fields[name] = json.Number(strconv.FormatInt(ch.To, 10))
		}
		if e.ItemChange != nil {
			fields["equipId"] = json.Number(strconv.FormatInt(e.ItemChange.EquipID, 10))
			fields["equipType"] = json.Number(strconv.FormatInt(e.ItemChange.EquipType, 10))
			for _, name := range e.ItemChange.ClearOverrides {
				fields[name] = json.Number("-1")
			}
		}
		edits = append(edits, shopwrite.Edit{RowID: id, Fields: fields})
	}
	return edits
}

// equipParamRef identifies one EquipParam* row via (equipType, equipId) --
// package-scoped (not BuildEquipParamEdits-local) so the debug-mode
// Pending Edits preview (pending_edits.go) can share the exact same
// resolution/aggregation computeEquipParamTargets does, rather than
// risking the UI's preview ever disagreeing with what actually gets
// written.
type equipParamRef struct {
	EquipType int64
	EquipID   int64
}

// equipParamTarget is one item's sellValue bookkeeping for the current
// PendingEdits batch: Current is its live (currently on-disk) sellValue
// (CurrentOK false if unresolvable), Target is what BuildEquipParamEdits
// will write for it (== Current when nothing needs to change).
type equipParamTarget struct {
	Current   int64
	CurrentOK bool
	Target    int64
}

// equipParamRefForEdit resolves a single row's effective (item ref, price)
// pair for sellValue bookkeeping, or ok=false if this edit doesn't touch
// price or item at all (an edit to an unrelated field, e.g. quantity,
// leaves the row's price and item alone, so there's nothing to (re)sync).
// Shared by computeEquipParamTargets and the debug-mode Pending Edits
// preview.
func equipParamRefForEdit(row *catalog.Row, e *RowEdit) (ref equipParamRef, price int64, ok bool) {
	valueChange, hasValueEdit := e.FieldChanges["value"]
	if !hasValueEdit && e.ItemChange == nil {
		return equipParamRef{}, 0, false
	}
	price = int64(-1)
	if row.Price != nil {
		price = *row.Price
	}
	if hasValueEdit {
		price = valueChange.To
	}
	if e.ItemChange != nil {
		ref = equipParamRef{e.ItemChange.EquipType, e.ItemChange.BaseEquipID}
	} else {
		ref = equipParamRef{row.Fields["equipType"], row.Fields["equipId"]}
	}
	return ref, price, true
}

// currentRowRef is a row's own (unedited) effective (item ref, price) --
// the fallback used for every row not staged in PendingEdits, and for
// staged rows whose edit doesn't touch price/item (e.g. quantity-only).
func currentRowRef(row *catalog.Row) (ref equipParamRef, price int64) {
	price = int64(-1)
	if row.Price != nil {
		price = *row.Price
	}
	return equipParamRef{row.Fields["equipType"], row.Fields["equipId"]}, price
}

// computeEquipParamTargets computes, for every item touched by this
// PendingEdits batch, the exact maximum-safe sellValue -- the single
// shared source of truth for both BuildEquipParamEdits (what actually gets
// written) and the debug-mode Pending Edits preview (rowRefs maps a row id
// back to which item/target it contributes to, for display), so the two
// can never disagree. Confirmed 2026-07-30/31 (see docs/MERCHANT_DATA.md)
// that visibility in a merchant's in-game stock is a plain
// `price >= sellValue` comparison -- no special "-1 sentinel" behavior (0
// and -1 are empirically identical: both always satisfy price>=sellValue
// since price is never negative).
//
// Target is the TRUE MINIMUM effective price across EVERY row in the
// entire save currently selling this item -- not just the rows in this
// batch, and not clamped against the item's previous sellValue. The same
// item can be sold by many ShopLineupParam rows across many merchants at
// once; raising sellValue is only safe when NO row anywhere still sells it
// cheaper. Scanning the whole table (via RowsByID, using each row's staged
// price/item where one is pending, else its own current value) gives that
// answer directly and lets sellValue correctly rise again once every
// remaining instance's price allows it -- no cross-session memory needed,
// since it's recomputed fresh from the save's actual current state (which
// already includes whatever prior sessions wrote) every call.
//
// A row with raw price == -1 (row.Price == nil) can never be beaten --
// -1 is always the lowest possible effective price -- so once one is found
// for a given item, that item's target is settled at -1 and further rows
// selling it are skipped (they can't change the answer).
func (s *State) computeEquipParamTargets() (targets map[equipParamRef]*equipParamTarget, rowRefs map[int64]equipParamRef, err error) {
	if len(s.PendingEdits) == 0 {
		return nil, nil, nil
	}
	rowsByID, err := s.Catalog.RowsByID()
	if err != nil {
		return nil, nil, err
	}

	touchedRefs := map[equipParamRef]bool{}
	rowRefs = map[int64]equipParamRef{}
	for rowID, e := range s.PendingEdits {
		row := rowsByID[rowID]
		if row == nil {
			continue
		}
		ref, _, ok := equipParamRefForEdit(row, e)
		if !ok {
			continue
		}
		rowRefs[rowID] = ref
		touchedRefs[ref] = true
	}
	if len(touchedRefs) == 0 {
		return nil, rowRefs, nil
	}

	globalMin := map[equipParamRef]int64{}
	settled := map[equipParamRef]bool{}
	for rowID, row := range rowsByID {
		var ref equipParamRef
		var price int64
		if e, staged := s.PendingEdits[rowID]; staged {
			if r, p, ok := equipParamRefForEdit(row, e); ok {
				ref, price = r, p
			} else {
				ref, price = currentRowRef(row)
			}
		} else {
			ref, price = currentRowRef(row)
		}

		if !touchedRefs[ref] || settled[ref] {
			continue
		}
		if price == -1 {
			globalMin[ref] = -1
			settled[ref] = true
			continue
		}
		if cur, seen := globalMin[ref]; !seen || price < cur {
			globalMin[ref] = price
		}
	}

	targets = map[equipParamRef]*equipParamTarget{}
	for ref := range touchedRefs {
		current, ok := s.Catalog.SellValue(ref.EquipType, ref.EquipID)
		targets[ref] = &equipParamTarget{Current: current, CurrentOK: ok, Target: globalMin[ref]}
	}
	return targets, rowRefs, nil
}

// BuildEquipParamEdits flattens computeEquipParamTargets' per-item targets
// into the shopwrite.Edit records combinedApplyWorker's sellValue pipeline
// stage writes, one entry per distinct EquipParam* table touched.
func (s *State) BuildEquipParamEdits() (map[string][]shopwrite.Edit, error) {
	targets, _, err := s.computeEquipParamTargets()
	if err != nil {
		return nil, err
	}

	out := map[string][]shopwrite.Edit{}
	for ref, t := range targets {
		if !t.CurrentOK || t.Target == t.Current {
			continue // unresolvable, or already matches -- nothing to do
		}
		entryName, ok := shopwrite.EquipParamEntryName(ref.EquipType)
		if !ok {
			continue
		}
		out[entryName] = append(out[entryName], shopwrite.Edit{
			RowID:  ref.EquipID,
			Fields: map[string]json.Number{"sellValue": json.Number(strconv.FormatInt(t.Target, 10))},
		})
	}
	return out, nil
}

// SuggestedOutPath is the default Save-As destination next to the loaded save:
// "<dir>/<stem>-edited<ext>". Empty when no save is loaded.
func (s *State) SuggestedOutPath() string {
	p := s.Catalog.SavePath()
	if p == "" {
		return ""
	}
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(filepath.Base(p), ext)
	return filepath.Join(filepath.Dir(p), stem+"-edited"+ext)
}
