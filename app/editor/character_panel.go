package editor

// Characters view (see docs/CHAR_UNLOCK.md): the app's landing view. Top
// bar: an Open-file bar (typed path + Load, or Browse...). Middle, 3
// columns: characters -> merchants with gated stock for the picked
// character -> that merchant's gated rows as checkboxes (checked =
// unlocked) -- toggle either direction.
//
// A checkbox toggle only stages a change (PendingFlagEdits, mirroring
// PendingEdits' staging model for item edits); writing happens through the
// shared Save button (state.go's startCombinedSave), which commits staged
// flag edits via app/charunlock -- a completely different file region/write
// engine than item edits' regulation.bin (see startCombinedSave's doc
// comment for how the two merge into one output file) -- alongside any
// staged item edits.
//
// Each gated row resolves to exactly one UnlockFlag (app/catalog decodes
// it), so a flag IS a row's checkbox -- no separate flag-browsing step.

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"er_merchant_editor/app/catalog"
	"er_merchant_editor/app/charunlock"
)

// charColWidth / merchantColWidth are the two narrow columns' fixed
// widths; the flags column (checkbox labels only) takes whatever remains.
const (
	charColWidth     = unit.Dp(220)
	merchantColWidth = unit.Dp(340)
)

// eniaMerchantName is excluded from the Characters view entirely, and MUST
// stay excluded from any bulk-unlock path. SAFETY-CRITICAL: her unlock
// flags alias the game's real boss-defeat flags (e.g. her Radahn armor rows
// and the "Remembrance of the Starscourge" trigger both use flag 9130), so
// toggling them via the normal per-row flag mechanism falsely marks bosses
// as defeated -- confirmed in-game as a forced boss cutscene/teleport.
// Do not add her back here without reading docs/CHAR_UNLOCK.md's "Enia
// excluded entirely" section first.
const eniaMerchantName = "Enia"

// twinMaidenHusksMerchantName selects her dedicated 3-section grid layout
// (layoutTMHFlagsGrid) instead of the plain flat list every other merchant
// gets -- see docs/CHAR_UNLOCK.md and app/charunlock.BellBearing.
const twinMaidenHusksMerchantName = "Twin Maiden Husks"

// --- retained per-item widget state (mirrors pending_edits.go's removeBtn) ---

func (s *State) charBtn(idx int) *widget.Clickable {
	b := s.charBtns[idx]
	if b == nil {
		b = &widget.Clickable{}
		s.charBtns[idx] = b
	}
	return b
}

func (s *State) merchantUnlockBtn(name string) *widget.Clickable {
	b := s.merchantBtns[name]
	if b == nil {
		b = &widget.Clickable{}
		s.merchantBtns[name] = b
	}
	return b
}

// flagCheck returns the retained checkbox state for a flag GROUP (keyed by
// UnlockFlag, not row id -- several rows can share one flag, see
// groupFlagRows, so one checkbox represents the whole group), creating it
// (seeded to false) on first use. Callers must keep .Value in sync
// themselves -- this only allocates; selectFlagMerchant does the seeding.
func (s *State) flagCheck(flagID int64) *widget.Bool {
	b := s.flagChecks[flagID]
	if b == nil {
		b = &widget.Bool{}
		s.flagChecks[flagID] = b
	}
	return b
}

// bellBearingCheck is flagCheck's counterpart for bell-bearing flags
// (uint32-keyed, no backing catalog.Row).
func (s *State) bellBearingCheck(flagID uint32) *widget.Bool {
	b := s.bellBearingChecks[flagID]
	if b == nil {
		b = &widget.Bool{}
		s.bellBearingChecks[flagID] = b
	}
	return b
}

// flagGroup is every gated row that shares one UnlockFlag -- e.g. a bell
// bearing purchase (Twin Maiden Husks) releases a whole batch of items
// under a single flag, so they must be shown/toggled as one unit rather
// than as independent checkboxes that could visually desync (see
// docs/CHAR_UNLOCK.md).
type flagGroup struct {
	FlagID int64
	Rows   []*catalog.Row
}

// pendingBoolSnapshot preserves the exact pending-edit state that existed
// before a bulk unlock changed a flag. "staged == false" means there was no
// edit at all, not that the desired value was false.
type pendingBoolSnapshot struct {
	staged bool
	value  bool
}

// bulkUnlockUndo is one reversible bulk action for one character. It records
// only flags that the action itself changed, so Undo restores any earlier
// manual staging rather than indiscriminately locking every merchant again.
type bulkUnlockUndo struct {
	charIndex int
	flags     map[int64]pendingBoolSnapshot
	bearings  map[uint32]pendingBoolSnapshot
}

// groupFlagRows groups rows by UnlockFlag, first-seen order.
func groupFlagRows(rows []*catalog.Row) []flagGroup {
	idx := make(map[int64]int, len(rows))
	var groups []flagGroup
	for _, r := range rows {
		if i, ok := idx[r.UnlockFlag]; ok {
			groups[i].Rows = append(groups[i].Rows, r)
			continue
		}
		idx[r.UnlockFlag] = len(groups)
		groups = append(groups, flagGroup{FlagID: r.UnlockFlag, Rows: []*catalog.Row{r}})
	}
	return groups
}

// flagGroupLabel joins a group's item names ("A / B / C"), capping at 3
// names before falling back to "and N more". Used to append the raw flag
// id in Debug mode; that mode is gone (2026-08-03) and the number meant
// nothing to a player, so it isn't shown at all now.
func flagGroupLabel(g flagGroup) string {
	names := make([]string, 0, len(g.Rows))
	for _, r := range g.Rows {
		n := r.DisplayName()
		if n == "" {
			n = r.Label
		}
		names = append(names, n)
	}
	label := strings.Join(names, " / ")
	if len(names) > 3 {
		label = fmt.Sprintf("%s and %d more", strings.Join(names[:3], " / "), len(names)-3)
	}
	return label
}

// --- caches, invalidated on save/selection change ---

// ensureCharList (re)reads the loaded save's raw bytes and enumerates its
// characters, only when the save path changed since the last call —
// os.ReadFile on a ~29MB save every frame would be wasteful. A fresh load
// also drops any staged-but-unsaved flag edits: they're meaningless
// against a different file.
func (s *State) ensureCharList() {
	path := s.Catalog.SavePath()
	if path == "" || path == s.charDataPath {
		return
	}
	s.charDataPath = path
	s.SelectedChar = -1
	s.UnlockMerchant = ""
	s.FlagRows = nil
	s.FlagState = nil
	s.gatedCacheChar = -2
	s.PendingFlagEdits = make(map[int]map[int64]bool)
	s.clearBulkUnlockUndo()
	data, err := os.ReadFile(path)
	if err != nil {
		s.charSaveData = nil
		s.CharList = nil
		return
	}
	s.charSaveData = data
	s.CharList = charunlock.ListCharacters(data)
}

// ensureMerchantGated (re)computes, for s.SelectedChar, every real
// merchant's gated-FLAG-GROUP total + currently-unlocked count (on-disk
// state, not pending-aware) — one count per groupFlagRows group, matching
// the number of checkboxes layoutFlagsColumn actually renders (rows
// sharing one UnlockFlag collapse to a single checkbox; counting raw rows
// here previously showed "N/M unlocked" with M larger than the real
// number of buttons for any merchant with a grouped flag, e.g. Twin Maiden
// Husks) — plus a char-wide rowID -> {unlocked, merchant} view
// (charFlagState/charFlagMerchant) that effectiveRowUnlocked and
// displayMerchantUnlocked build on — only recomputed when the selection
// or save changed since the last call.
func (s *State) ensureMerchantGated() {
	if s.SelectedChar < 0 {
		return
	}
	if s.gatedCachePath == s.charDataPath && s.gatedCacheChar == s.SelectedChar {
		return
	}
	s.gatedCachePath = s.charDataPath
	s.gatedCacheChar = s.SelectedChar
	merchants, err := s.Catalog.ListMerchants()
	if err != nil {
		s.merchantGatedTotal, s.merchantGatedUnlocked = nil, nil
		s.charFlagState, s.charFlagMerchant = nil, nil
		s.charFlagFlag, s.readOnlyGateRows, s.tmhBellCommitted = nil, nil, nil
		return
	}
	total := make(map[string]int, len(merchants))
	unlocked := make(map[string]int, len(merchants))
	rowState := make(map[int64]bool)
	rowMerchant := make(map[int64]string)
	rowFlag := make(map[int64]int64)
	readOnlyRows := make(map[int64]bool)
	for _, m := range merchants {
		if m.Name == eniaMerchantName {
			// Enia's event flags are real boss-progress flags and must never
			// be written by this app. Reading them is safe, however, and lets
			// the Shop Editor show the selected character's actual stock
			// state instead of marking every gated Enia item locked.
			rows, err := s.Catalog.MerchantRows(m.Name)
			if err != nil {
				continue
			}
			states, err := charunlock.LockStates(s.charSaveData, s.SelectedChar, rows)
			if err != nil {
				continue
			}
			for rowID, isUnlocked := range states {
				rowState[rowID] = isUnlocked
				readOnlyRows[rowID] = true
			}
			for _, r := range rows {
				if _, ok := states[r.RowID]; ok {
					rowFlag[r.RowID] = r.UnlockFlag
				}
			}
			continue
		}
		rows, err := s.Catalog.MerchantRows(m.Name)
		if err != nil {
			continue
		}
		states, err := charunlock.LockStates(s.charSaveData, s.SelectedChar, rows)
		if err != nil || len(states) == 0 {
			continue
		}
		for rowID, isUnlocked := range states {
			rowState[rowID] = isUnlocked
			rowMerchant[rowID] = m.Name
		}
		// Group by UnlockFlag before counting — every row in a group shares
		// the exact same on-disk flag, so states[group.Rows[0].RowID] is
		// authoritative for the whole group, not an approximation.
		gatedRows := make([]*catalog.Row, 0, len(states))
		for _, r := range rows {
			if _, ok := states[r.RowID]; ok {
				gatedRows = append(gatedRows, r)
				rowFlag[r.RowID] = r.UnlockFlag
			}
		}
		groups := groupFlagRows(gatedRows)
		u := 0
		for _, g := range groups {
			if states[g.Rows[0].RowID] {
				u++
			}
		}
		total[m.Name] = len(groups)
		unlocked[m.Name] = u
	}
	// Twin Maiden Husks' count also includes her bell-bearing ("NPC")
	// buttons, which layoutFlagsColumn renders alongside the gated-row groups
	// and flagsColumnLockedCount already counts for the "Check all remaining"
	// header -- without this the middle-column "N/M unlocked" undercounted her
	// total against the buttons actually shown.
	s.tmhBellCommitted = nil
	if _, ok := total[twinMaidenHusksMerchantName]; ok {
		bb := charunlock.BellBearingsForUI()
		ids := make([]uint32, len(bb))
		for i, b := range bb {
			ids[i] = b.FlagID
		}
		if bbStates, err := charunlock.FlagStates(s.charSaveData, s.SelectedChar, ids); err == nil {
			s.tmhBellCommitted = bbStates
			total[twinMaidenHusksMerchantName] += len(bb)
			for _, b := range bb {
				if bbStates[b.FlagID] {
					unlocked[twinMaidenHusksMerchantName]++
				}
			}
		}
	}

	s.merchantGatedTotal = total
	s.merchantGatedUnlocked = unlocked
	s.charFlagState = rowState
	s.charFlagMerchant = rowMerchant
	s.charFlagFlag = rowFlag
	s.readOnlyGateRows = readOnlyRows
}

// charName returns a character's name by slot index, falling back to
// "Character <idx>" if it's not (or no longer) in CharList -- used by the
// shared Pending dropdown (pending_edits.go), which can list a character
// other than the currently selected one.
func (s *State) charName(idx int) string {
	for _, c := range s.CharList {
		if c.Index == idx {
			return c.Name
		}
	}
	return fmt.Sprintf("Character %d", idx)
}

// selectedCharName returns the selected character's name ("" if none
// selected).
func (s *State) selectedCharName() string {
	if s.SelectedChar < 0 {
		return ""
	}
	return s.charName(s.SelectedChar)
}

// effectiveRowUnlocked reports whether a gated row is currently unlocked
// for the selected character, preferring a staged (not yet saved) edit
// over the on-disk value -- so both the Shop Editor's purple-lock display
// and the merchant list's own colors react immediately to a checkbox
// toggle in the flags column, not just after Save. known=false when
// there's no character context to answer from (no character selected, or
// the row isn't a tracked gated row for one).
func (s *State) effectiveRowUnlocked(rowID int64) (unlocked, known bool) {
	if s.SelectedChar < 0 {
		return false, false
	}
	committed, ok := s.charFlagState[rowID]
	if !ok {
		return false, false
	}
	if s.readOnlyGateRows[rowID] {
		return committed, true
	}
	if target, staged := s.PendingFlagEdits[s.SelectedChar][rowID]; staged {
		return target, true
	}
	return committed, true
}

// displayMerchantUnlocked overlays any staged (unsaved) flag edits onto
// merchantGatedUnlocked's on-disk counts, so a merchant's list color
// (layoutMerchantRow) updates live as the user (un)checks flags instead
// of only after Save Character/Save All.
func (s *State) displayMerchantUnlocked() map[string]int {
	out := make(map[string]int, len(s.merchantGatedUnlocked))
	for name, n := range s.merchantGatedUnlocked {
		out[name] = n
	}
	// Overlay staged flag edits collapsed PER FLAG-GROUP: rows sharing an
	// UnlockFlag are staged together (stageFlag is called once per row) but
	// the base count in ensureMerchantGated counts the group as one button.
	// Counting each staged row separately moved the tally by the group's row
	// count -- e.g. a 19-row Twin Maiden Husks group produced the -19 the
	// user hit -- so dedupe by UnlockFlag and move it by 1.
	seenFlag := make(map[int64]bool)
	for rowID, target := range s.PendingFlagEdits[s.SelectedChar] {
		flag, ok := s.charFlagFlag[rowID]
		if !ok || seenFlag[flag] {
			continue
		}
		committed, ok := s.charFlagState[rowID]
		if !ok || target == committed {
			continue
		}
		seenFlag[flag] = true
		name, ok := s.charFlagMerchant[rowID]
		if !ok {
			continue
		}
		if target {
			out[name]++
		} else {
			out[name]--
		}
	}
	// Overlay staged bell-bearing ("NPC") edits -- one flag per button, so no
	// grouping needed; folded into Twin Maiden Husks' count to match the
	// base total from ensureMerchantGated.
	for flagID, target := range s.PendingBellBearingEdits[s.SelectedChar] {
		if target == s.tmhBellCommitted[flagID] {
			continue
		}
		if target {
			out[twinMaidenHusksMerchantName]++
		} else {
			out[twinMaidenHusksMerchantName]--
		}
	}
	return out
}

func (s *State) selectCharacter(idx int) {
	if s.SelectedChar == idx {
		s.SelectedChar = -1
	} else {
		s.SelectedChar = idx
	}
	s.UnlockMerchant = ""
	s.FlagRows = nil
	s.FlagState = nil
	s.bellBearingState = nil
	s.gatedCacheChar = -2
	s.clearBulkUnlockUndo()
}

// selectFlagMerchant picks (or deselects) the merchant whose gated rows
// are shown in the flags column, seeding each row's checkbox from any
// staged edit if one exists, else the save's real current state.
func (s *State) selectFlagMerchant(name string) {
	if name == eniaMerchantName {
		return // defensive -- see eniaMerchantName's doc comment; the middle
		// column's own list already excludes her, so this shouldn't be
		// reachable through normal UI interaction
	}
	if s.UnlockMerchant == name {
		s.UnlockMerchant = ""
		s.FlagRows = nil
		s.FlagState = nil
		s.bellBearingState = nil
		return
	}
	s.UnlockMerchant = name
	allRows, err := s.Catalog.MerchantRows(name)
	if err != nil {
		s.FlagRows, s.FlagState = nil, nil
		return
	}
	states, err := charunlock.LockStates(s.charSaveData, s.SelectedChar, allRows)
	if err != nil {
		s.FlagRows, s.FlagState = nil, nil
		return
	}
	rows := make([]*catalog.Row, 0, len(states))
	for _, r := range allRows {
		if _, ok := states[r.RowID]; ok {
			rows = append(rows, r)
		}
	}
	s.FlagRows = rows
	s.FlagState = states
	pending := s.PendingFlagEdits[s.SelectedChar]
	for _, g := range groupFlagRows(rows) {
		val := states[g.Rows[0].RowID] // shared flag -> same committed value for every row in the group
		for _, r := range g.Rows {
			if target, ok := pending[r.RowID]; ok {
				val = target
				break
			}
		}
		s.flagCheck(g.FlagID).Value = val
	}

	if name != twinMaidenHusksMerchantName {
		s.bellBearingState = nil
		return
	}
	bb := charunlock.BellBearingsForUI()
	ids := make([]uint32, len(bb))
	for i, b := range bb {
		ids[i] = b.FlagID
	}
	bbStates, err := charunlock.FlagStates(s.charSaveData, s.SelectedChar, ids)
	if err != nil {
		s.bellBearingState = nil
		return
	}
	s.bellBearingState = bbStates
	pendingBB := s.PendingBellBearingEdits[s.SelectedChar]
	for _, b := range bb {
		val := bbStates[b.FlagID]
		if target, ok := pendingBB[b.FlagID]; ok {
			val = target
		}
		s.bellBearingCheck(b.FlagID).Value = val
	}
}

// stageFlag records (or clears) a staged toggle for one row under the
// currently selected character. Staging back to the committed (on-disk)
// value removes the entry, same rule as item-edit staging in staging.go.
func (s *State) stageFlag(rowID int64, target bool) {
	s.stageFlagAgainstCommitted(rowID, s.FlagState[rowID], target)
}

// stageFlagAgainstCommitted is stageFlag with the caller supplying the
// on-disk value. It lets the character-wide bulk action use the same
// unstage-at-committed rule without having to change the currently opened
// merchant's FlagRows.
func (s *State) stageFlagAgainstCommitted(rowID int64, committed, target bool) {
	if target == committed {
		if m := s.PendingFlagEdits[s.SelectedChar]; m != nil {
			delete(m, rowID)
		}
		return
	}
	m := s.PendingFlagEdits[s.SelectedChar]
	if m == nil {
		m = make(map[int64]bool)
		s.PendingFlagEdits[s.SelectedChar] = m
	}
	m[rowID] = target
}

func (s *State) pendingFlagSnapshot(rowID int64) pendingBoolSnapshot {
	target, staged := s.PendingFlagEdits[s.SelectedChar][rowID]
	return pendingBoolSnapshot{staged: staged, value: target}
}

// stageBellBearing records (or clears) a staged bell-bearing acquisition
// toggle for the currently selected character. Same un-stage-at-committed-
// value rule as stageFlag.
func (s *State) stageBellBearing(flagID uint32, target bool) {
	s.stageBellBearingAgainstCommitted(flagID, s.bellBearingState[flagID], target)
}

// stageBellBearingAgainstCommitted is stageBellBearing's character-wide
// counterpart. tmhBellCommitted is available even while another merchant is
// selected, whereas bellBearingState intentionally only exists in TMH's
// details view.
func (s *State) stageBellBearingAgainstCommitted(flagID uint32, committed, target bool) {
	if target == committed {
		if m := s.PendingBellBearingEdits[s.SelectedChar]; m != nil {
			delete(m, flagID)
		}
		return
	}
	m := s.PendingBellBearingEdits[s.SelectedChar]
	if m == nil {
		m = make(map[uint32]bool)
		s.PendingBellBearingEdits[s.SelectedChar] = m
	}
	m[flagID] = target
}

func (s *State) pendingBellBearingSnapshot(flagID uint32) pendingBoolSnapshot {
	target, staged := s.PendingBellBearingEdits[s.SelectedChar][flagID]
	return pendingBoolSnapshot{staged: staged, value: target}
}

// pendingBellBearingCount returns how many bell-bearing flags have a
// staged (unsaved) toggle for the given character.
func (s *State) pendingBellBearingCount(charIndex int) int {
	return len(s.PendingBellBearingEdits[charIndex])
}

// totalPendingBellBearingCount sums pendingBellBearingCount across every
// character with a staged bell-bearing edit.
func (s *State) totalPendingBellBearingCount() int {
	n := 0
	for _, m := range s.PendingBellBearingEdits {
		n += len(m)
	}
	return n
}

// stageFlagForRow is stageFlag's counterpart for callers outside the
// Characters view (the Shop Editor row-edit bar's Unlock/Lock button, see
// setRowUnlockForSelectedChar) -- same PendingFlagEdits map, but keyed
// off the char-wide charFlagState cache (ensureMerchantGated) instead of
// the merchant-scoped FlagState cache selectFlagMerchant fills, since the
// Shop Editor has no notion of "which merchant is expanded in the
// Characters view" and shouldn't need one to unlock a row it can already see.
func (s *State) stageFlagForRow(rowID int64, target bool) {
	if s.readOnlyGateRows[rowID] {
		return
	}
	committed, ok := s.charFlagState[rowID]
	if !ok {
		return
	}
	s.stageFlagAgainstCommitted(rowID, committed, target)
}

// RemovePendingFlagsForChar discards every staged character-flag edit for
// one character slot in one click -- both row-gate toggles
// (PendingFlagEdits) and bell-bearing acquisition toggles
// (PendingBellBearingEdits) -- backing the Pending Edits modal's
// per-character "Remove all".
func (s *State) RemovePendingFlagsForChar(charIndex int) {
	delete(s.PendingFlagEdits, charIndex)
	delete(s.PendingBellBearingEdits, charIndex)
	if (s.merchantUnlockUndo != nil && s.merchantUnlockUndo.charIndex == charIndex) ||
		(s.allMerchantsUndo != nil && s.allMerchantsUndo.charIndex == charIndex) {
		s.clearBulkUnlockUndo()
	}
	s.clearFooterStatusWhenNoPending()
}

// removeFlagBtn returns the retained per-character "Remove all" button
// state for the Pending Edits modal's character-unlocks section, creating
// it on first use.
func (s *State) removeFlagBtn(charIndex int) *widget.Clickable {
	b := s.removeFlagBtns[charIndex]
	if b == nil {
		b = &widget.Clickable{}
		s.removeFlagBtns[charIndex] = b
	}
	return b
}

// setRowUnlockForSelectedChar sets a gated row's unlock state for
// s.SelectedChar to target -- the Shop Editor row-edit bar's Unlock/Lock
// button routes through this (via the bar's own gate DRAFT, applied on
// Apply -- see row_edit_form.go's toggleDraftGate/applyDraft), wired to
// have the identical effect as toggling that row's checkbox in the
// Characters view. Falls through to setRowUnlockForAllChars when no
// character is selected (user request 2026-08-03: the button should
// still be usable then, applying to every character at once, rather than
// being a dead no-op).
//
// Stages EVERY row in s.MerchantRows sharing row's UnlockFlag, not just row
// itself -- e.g. Twin Maiden Husks' bell-bearing purchase releases a whole
// batch of rows under one shared flag, shown as ONE checkbox/flagGroup in
// the Characters view; the row-edit bar must match that grouping instead of
// only affecting the one row it happens to be open on.
func (s *State) setRowUnlockForSelectedChar(row *catalog.Row, target bool) {
	if s.lastMerchant == eniaMerchantName {
		return
	}
	if s.SelectedChar < 0 {
		s.setRowUnlockForAllChars(row, target)
		return
	}
	s.ensureMerchantGated()
	for _, r := range s.MerchantRows {
		if r.UnlockFlag == row.UnlockFlag {
			s.stageFlagForRow(r.RowID, target)
		}
	}
	// The merchant grid already rendered this frame (layoutMerchantPanel
	// draws it before the docked row-edit bar below it) using the OLD lock
	// state, same one-frame-stale issue SelectRow's own invalidate() call
	// guards against -- without this the badge only catches up on whatever
	// next redraw happens to occur, which reads as "didn't work".
	s.invalidate()
}

// setRowUnlockForAllChars is setRowUnlockForSelectedChar's no-character-
// selected counterpart (user request 2026-08-03): stages target for row's
// UnlockFlag group across EVERY character in the save, not just one,
// mirroring stageFlagForRow's per-character committed-value check so each
// character's PendingFlagEdits only gets an entry when it actually
// changes something for them.
//
// Defensive Enia re-check even though gateActionSpec (row_edit_form.go)
// must already disable the button entirely for her merchant -- see
// eniaMerchantName's doc comment (flag ids alias real boss-defeat flags;
// this must never run for her rows, from any call path).
func (s *State) setRowUnlockForAllChars(row *catalog.Row, target bool) {
	if s.lastMerchant == eniaMerchantName {
		return
	}
	s.ensureCharList()
	var group []*catalog.Row
	for _, r := range s.MerchantRows {
		if r.UnlockFlag == row.UnlockFlag {
			group = append(group, r)
		}
	}
	for _, c := range s.CharList {
		idx := c.Index
		states, err := charunlock.LockStates(s.charSaveData, idx, group)
		if err != nil {
			continue
		}
		for _, r := range group {
			committed := states[r.RowID]
			if target == committed {
				if m := s.PendingFlagEdits[idx]; m != nil {
					delete(m, r.RowID)
				}
				continue
			}
			m := s.PendingFlagEdits[idx]
			if m == nil {
				m = make(map[int64]bool)
				s.PendingFlagEdits[idx] = m
			}
			m[r.RowID] = target
		}
	}
	s.invalidate()
}

// pendingFlagCount returns how many flag-checkbox changes are staged for the
// given character. One checkbox can control several stock rows that share an
// UnlockFlag; all of those rows must remain in PendingFlagEdits so the save
// writer updates the entire group, but they are one user action and therefore
// count as one pending change in the UI.
func (s *State) pendingFlagCount(charIndex int) int {
	return s.pendingFlagGroupCount(s.PendingFlagEdits[charIndex])
}

// pendingFlagGroupCount collapses rows that share an UnlockFlag into the one
// checkbox the Characters view exposes. The fallback keeps hand-built test
// state and any unknown legacy row distinct rather than accidentally merging
// it with a real flag.
func (s *State) pendingFlagGroupCount(edits map[int64]bool) int {
	type key struct {
		flag  int64
		known bool
	}
	seen := make(map[key]bool, len(edits))
	for rowID := range edits {
		flag, known := s.charFlagFlag[rowID]
		if !known {
			// A row ID is unique, so it is a safe distinct fallback key.
			flag = rowID
		}
		seen[key{flag: flag, known: known}] = true
	}
	return len(seen)
}

// totalPendingFlagCount sums pending checkbox changes across every character
// with a staged edit. It deliberately does not count each backing stock row.
func (s *State) totalPendingFlagCount() int {
	n := 0
	for _, edits := range s.PendingFlagEdits {
		n += s.pendingFlagGroupCount(edits)
	}
	return n
}

// --- layout ---

// layoutCharactersPanel is the whole view shown instead of the two editor
// panels while State.view == viewCharacters: an Open-file bar and a
// 3-column character/merchant/flags drill-down. Saving staged flag edits
// happens through the shared footer -- see startCombinedSave in state.go.
func (s *State) layoutCharactersPanel(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s.ensureCharList()

	// Drain clicks before recomputing the caches they invalidate, so a
	// click's effect is reflected in this same frame's render instead of
	// lagging one frame behind.
	if s.loadTypedBtn.Clicked(gtx) {
		if p := strings.TrimSpace(s.pathEditor.Text()); p != "" {
			s.StartLoadSave(p)
		}
	}
	if s.openBtn.Clicked(gtx) {
		s.openFileDialog()
	}
	for i := range s.CharList {
		idx := s.CharList[i].Index
		if s.charBtn(idx).Clicked(gtx) {
			s.selectCharacter(idx)
		}
	}
	s.ensureMerchantGated()
	gatedMerchants := s.sortedGatedMerchants()
	for _, name := range gatedMerchants {
		if s.merchantUnlockBtn(name).Clicked(gtx) {
			s.selectFlagMerchant(name)
		}
	}
	for _, g := range groupFlagRows(s.FlagRows) {
		chk := s.flagCheck(g.FlagID)
		if chk.Update(gtx) {
			for _, r := range g.Rows {
				s.stageFlag(r.RowID, chk.Value)
			}
		}
	}
	if s.UnlockMerchant == twinMaidenHusksMerchantName {
		for _, b := range charunlock.BellBearingsForUI() {
			chk := s.bellBearingCheck(b.FlagID)
			if chk.Update(gtx) {
				s.stageBellBearing(b.FlagID, chk.Value)
			}
		}
	}
	lockedHere := s.flagsColumnLockedCount()
	lockedEverywhere := s.allMerchantsLockedCount()
	if clicked := s.unlockAllBtn.Clicked(gtx); clicked && (s.merchantUnlockUndo != nil || lockedHere > 0) {
		undoing := s.merchantUnlockUndo != nil
		s.toggleMerchantUnlocks()
		if undoing {
			if s.combinedPendingCount() > 0 {
				s.setFooterStatus("Removed this merchant's staged unlocks")
			}
		} else {
			s.setFooterStatus(fmt.Sprintf("Staged %d unlocks for %s", lockedHere, s.UnlockMerchant))
		}
	}
	if clicked := s.unlockAllMerchantsBtn.Clicked(gtx); clicked && (s.allMerchantsUndo != nil || lockedEverywhere > 0) {
		undoing := s.allMerchantsUndo != nil
		s.toggleEveryMerchantUnlocks()
		if undoing {
			if s.combinedPendingCount() > 0 {
				s.setFooterStatus("Removed all staged merchant unlocks")
			}
		} else {
			s.setFooterStatus(fmt.Sprintf("Staged %d merchant unlocks", lockedEverywhere))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return barSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutOpenBar(gtx, th)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutCharactersColumns(gtx, th, gatedMerchants)
			})
		}),
	)
}

// effectiveFlagGroupUnlocked returns the committed state overlaid with a
// staged change. A group normally has every row staged together, but scanning
// the group keeps the bulk controls correct even if an old pending edit only
// touched one row in the shared flag group.
func (s *State) effectiveFlagGroupUnlocked(g flagGroup) bool {
	value := s.FlagState[g.Rows[0].RowID]
	for _, r := range g.Rows {
		if target, staged := s.PendingFlagEdits[s.SelectedChar][r.RowID]; staged {
			return target
		}
	}
	return value
}

func (s *State) effectiveBellBearingUnlocked(flagID uint32) bool {
	if target, staged := s.PendingBellBearingEdits[s.SelectedChar][flagID]; staged {
		return target
	}
	return s.bellBearingState[flagID]
}

// toggleMerchantUnlocks unlocks every remaining checkbox in the open
// merchant, or restores exactly the values changed by its previous click.
func (s *State) toggleMerchantUnlocks() {
	if undo := s.merchantUnlockUndo; undo != nil {
		s.restoreBulkUnlockUndo(undo)
		s.merchantUnlockUndo = nil
		s.invalidate()
		return
	}
	if s.SelectedChar < 0 || s.UnlockMerchant == "" {
		return
	}
	undo := &bulkUnlockUndo{charIndex: s.SelectedChar, flags: make(map[int64]pendingBoolSnapshot), bearings: make(map[uint32]pendingBoolSnapshot)}
	for _, g := range groupFlagRows(s.FlagRows) {
		if s.effectiveFlagGroupUnlocked(g) {
			continue
		}
		s.flagCheck(g.FlagID).Value = true
		for _, r := range g.Rows {
			undo.flags[r.RowID] = s.pendingFlagSnapshot(r.RowID)
			s.stageFlag(r.RowID, true)
		}
	}
	if s.UnlockMerchant == twinMaidenHusksMerchantName {
		for _, b := range charunlock.BellBearingsForUI() {
			if s.effectiveBellBearingUnlocked(b.FlagID) {
				continue
			}
			s.bellBearingCheck(b.FlagID).Value = true
			undo.bearings[b.FlagID] = s.pendingBellBearingSnapshot(b.FlagID)
			s.stageBellBearing(b.FlagID, true)
		}
	}
	if len(undo.flags) != 0 || len(undo.bearings) != 0 {
		s.merchantUnlockUndo = undo
		s.invalidate()
	}
}

// toggleEveryMerchantUnlocks is the character-wide counterpart of
// toggleMerchantUnlocks. Enia's rows exist in charFlagState only for their
// read-only Shop Editor lock display, so readOnlyGateRows keeps them out of
// this write path. TMH's talk-script bearing flags are added separately
// because they have no ShopLineup row.
func (s *State) toggleEveryMerchantUnlocks() {
	if undo := s.allMerchantsUndo; undo != nil {
		s.restoreBulkUnlockUndo(undo)
		s.allMerchantsUndo = nil
		s.invalidate()
		return
	}
	if s.SelectedChar < 0 {
		return
	}
	s.ensureMerchantGated()
	undo := &bulkUnlockUndo{charIndex: s.SelectedChar, flags: make(map[int64]pendingBoolSnapshot), bearings: make(map[uint32]pendingBoolSnapshot)}
	for rowID, committed := range s.charFlagState {
		if s.readOnlyGateRows[rowID] {
			continue
		}
		_, staged := s.PendingFlagEdits[s.SelectedChar][rowID]
		if staged && s.PendingFlagEdits[s.SelectedChar][rowID] {
			continue
		}
		if !committed || staged {
			undo.flags[rowID] = s.pendingFlagSnapshot(rowID)
		}
		s.stageFlagAgainstCommitted(rowID, committed, true)
	}
	for _, b := range charunlock.BellBearingsForUI() {
		_, staged := s.PendingBellBearingEdits[s.SelectedChar][b.FlagID]
		if staged && s.PendingBellBearingEdits[s.SelectedChar][b.FlagID] {
			continue
		}
		if !s.tmhBellCommitted[b.FlagID] || staged {
			undo.bearings[b.FlagID] = s.pendingBellBearingSnapshot(b.FlagID)
		}
		s.stageBellBearingAgainstCommitted(b.FlagID, s.tmhBellCommitted[b.FlagID], true)
	}
	if len(undo.flags) != 0 || len(undo.bearings) != 0 {
		s.allMerchantsUndo = undo
		s.invalidate()
	}
}

func (s *State) restoreBulkUnlockUndo(undo *bulkUnlockUndo) {
	if undo == nil || undo.charIndex != s.SelectedChar {
		return
	}
	for rowID, old := range undo.flags {
		if old.staged {
			if s.PendingFlagEdits[s.SelectedChar] == nil {
				s.PendingFlagEdits[s.SelectedChar] = make(map[int64]bool)
			}
			s.PendingFlagEdits[s.SelectedChar][rowID] = old.value
		} else if pending := s.PendingFlagEdits[s.SelectedChar]; pending != nil {
			delete(pending, rowID)
		}
	}
	for flagID, old := range undo.bearings {
		if old.staged {
			if s.PendingBellBearingEdits[s.SelectedChar] == nil {
				s.PendingBellBearingEdits[s.SelectedChar] = make(map[uint32]bool)
			}
			s.PendingBellBearingEdits[s.SelectedChar][flagID] = old.value
		} else if pending := s.PendingBellBearingEdits[s.SelectedChar]; pending != nil {
			delete(pending, flagID)
		}
	}
}

func (s *State) clearBulkUnlockUndo() {
	s.merchantUnlockUndo = nil
	s.allMerchantsUndo = nil
}

// layoutOpenBar is the er_pvp_mod-style top bar: a typed-path editor +
// Load (no-op if empty), a Browse... button (native dialog), and a status
// message to the right of Browse.
// pathEditorWidthNum/Den cap the typed-path Editor at 2/5 of the bar's own
// width -- left uncapped (as a Flexed child) it swallows all remaining
// space after the buttons.
const pathEditorWidthNum, pathEditorWidthDen = 2, 5

// filenameSlotWidth is the fixed width of the loaded-filename slot at
// the end of the Open bar (layoutFilenameSlot, save_switcher.go).
const filenameSlotWidth = unit.Dp(220)

// dividerBarHeight caps verticalDivider's height when used inside a
// single-line bar (a Rigid child of a Vertical Flex effectively has an
// unbounded Max.Y -- verticalDivider trusts Max.Y completely, so without
// this it would paint a divider far taller than the bar itself).
const dividerBarHeight = unit.Dp(28)

// layoutPathEditor renders the typed-path Editor capped to
// pathEditorWidthNum/Den of barWidth, and FORCED to fill that width (not
// just capped to it) -- Min.X = Max.X, since a Rigid Flex child is free to
// report any width up to Max, and an Editor with short/empty content
// otherwise shrinks to fit its placeholder text instead. A method rather
// than an inline closure so character_panel_test.go can assert its rendered
// width directly (a real width bug once slipped through as an inline closure).
func (s *State) layoutPathEditor(gtx layout.Context, th *material.Theme, barWidth int) layout.Dimensions {
	gtx.Constraints.Max.X = barWidth * pathEditorWidthNum / pathEditorWidthDen
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	ed := material.Editor(th, &s.pathEditor, "Save file path...")
	return boxed(gtx, th, ed.Layout)
}

func (s *State) layoutOpenBar(gtx layout.Context, th *material.Theme) layout.Dimensions {
	barWidth := gtx.Constraints.Max.X
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutPathEditor(gtx, th, barWidth)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Button(th, &s.loadTypedBtn, "Load").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Button(th, &s.openBtn, "Browse...").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutOpenMessage(gtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.Y = gtx.Dp(dividerBarHeight)
			return verticalDivider(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(filenameSlotWidth)
			return s.layoutFilenameSlot(gtx, th)
		}),
	)
}

// sortedGatedMerchants orders using catalog.MerchantSortKey (TMH first,
// then named NPCs, then the 5 wandering-merchant shop families, then DLC)
// so this column matches the Shop Editor filter dropdown's ordering.
func (s *State) sortedGatedMerchants() []string {
	names := make([]string, 0, len(s.merchantGatedTotal))
	for name := range s.merchantGatedTotal {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		gi, ni := catalog.MerchantSortKey(names[i])
		gj, nj := catalog.MerchantSortKey(names[j])
		if gi != gj {
			return gi < gj
		}
		return ni < nj
	})
	return names
}

func (s *State) layoutCharactersColumns(gtx layout.Context, th *material.Theme, gatedMerchants []string) layout.Dimensions {
	if !s.Catalog.Loaded() {
		lbl := material.Body1(th, "Open a save file above to get started.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}
	if len(s.CharList) == 0 {
		lbl := material.Body1(th, "No characters found in this save.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(charColWidth)
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return s.layoutCharColumn(gtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return verticalDivider(gtx) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(merchantColWidth)
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return s.layoutMerchantColumn(gtx, th, gatedMerchants, s.displayMerchantUnlocked())
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return verticalDivider(gtx) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutFlagsColumn(gtx, th)
		}),
	)
}

func (s *State) layoutCharColumn(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s.charColList.Axis = layout.Vertical
	return material.List(th, &s.charColList).Layout(gtx, len(s.CharList), func(gtx layout.Context, i int) layout.Dimensions {
		return s.layoutCharRow(gtx, th, s.CharList[i])
	})
}

func (s *State) layoutCharRow(gtx layout.Context, th *material.Theme, ch charunlock.Character) layout.Dimensions {
	label := fmt.Sprintf("%s\nlvl %d", ch.Name, ch.Level)
	if n := s.pendingFlagCount(ch.Index) + s.pendingBellBearingCount(ch.Index); n > 0 {
		label = fmt.Sprintf("%s\nlvl %d (%d pending)", ch.Name, ch.Level, n)
	}
	btn := formButton(th, s.charBtn(ch.Index), label)
	if s.SelectedChar == ch.Index {
		btn.Background = colorAccent
		btn.Color = th.Palette.ContrastFg
	} else {
		btn.Background = colorInputBg
		btn.Color = colorFg
	}
	return layout.Inset{Bottom: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, btn.Layout)
}

func (s *State) layoutMerchantColumn(gtx layout.Context, th *material.Theme, gatedMerchants []string, unlockedDisplay map[string]int) layout.Dimensions {
	if s.SelectedChar < 0 {
		lbl := material.Body2(th, "Pick a character.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}
	if len(gatedMerchants) == 0 {
		lbl := material.Body2(th, "No merchant has gated stock for this character.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}
	s.merchantColList.Axis = layout.Vertical
	return material.List(th, &s.merchantColList).Layout(gtx, len(gatedMerchants), func(gtx layout.Context, i int) layout.Dimensions {
		return s.layoutMerchantRow(gtx, th, gatedMerchants[i], unlockedDisplay)
	})
}

// layoutMerchantRow's unlocked count comes from unlockedDisplay (staged
// edits overlaid on top of the on-disk count, see displayMerchantUnlocked)
// so the row recolors live as the user checks/unchecks flags, not just
// after a save.
func (s *State) layoutMerchantRow(gtx layout.Context, th *material.Theme, name string, unlockedDisplay map[string]int) layout.Dimensions {
	total, unlocked := s.merchantGatedTotal[name], unlockedDisplay[name]
	label := fmt.Sprintf("%s\n%d/%d gates unlocked", name, unlocked, total)
	btn := formButton(th, s.merchantUnlockBtn(name), label)
	switch {
	case s.UnlockMerchant == name:
		btn.Background = colorAccent
		btn.Color = th.Palette.ContrastFg
	case unlocked < total:
		btn.Background = colorInputBg
		btn.Color = colorWarnTxt
	default:
		btn.Background = colorInputBg
		btn.Color = colorMuted
	}
	return layout.Inset{Bottom: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, btn.Layout)
}

func (s *State) layoutFlagsColumn(gtx layout.Context, th *material.Theme) layout.Dimensions {
	if s.SelectedChar < 0 {
		lbl := material.Body2(th, "Pick a character, then a merchant, to see its flags.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}
	if s.UnlockMerchant == "" {
		lbl := material.Body2(th, "Pick a merchant to see its flags.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}

	lockedCount := s.flagsColumnLockedCount()
	allLockedCount := s.allMerchantsLockedCount()
	merchantUndo := s.merchantUnlockUndo != nil
	allUndo := s.allMerchantsUndo != nil

	header := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(material.H6(th, s.UnlockMerchant).Layout),
			layout.Flexed(1, flexSpacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := fmt.Sprintf("Unlock all merchants (%d)", allLockedCount)
				if allUndo {
					label = "Undo all-merchant unlock"
				}
				return actionButton(gtx, th, &s.unlockAllMerchantsBtn, label, allUndo || allLockedCount > 0)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := fmt.Sprintf("Unlock this merchant (%d)", lockedCount)
				if merchantUndo {
					label = "Undo merchant unlock"
				}
				return actionButton(gtx, th, &s.unlockAllBtn, label, merchantUndo || lockedCount > 0)
			}),
		)
	})

	body := layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		if s.UnlockMerchant == twinMaidenHusksMerchantName {
			return s.layoutTMHFlagsGrid(gtx, th)
		}
		if scrollFlagsMerchants[s.UnlockMerchant] {
			return s.layoutScrollFlagsGrid(gtx, th)
		}
		groups := groupFlagRows(s.FlagRows)
		s.flagColList.Axis = layout.Vertical
		return material.List(th, &s.flagColList).Layout(gtx, len(groups), func(gtx layout.Context, i int) layout.Dimensions {
			return s.layoutFlagRow(gtx, th, groups[i])
		})
	})

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		header,
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(horizontalDivider),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		body,
	)
}

// allMerchantsLockedCount is the total number of still-locked checkbox
// groups across the currently selected character. It intentionally uses the
// same pending-aware merchant counts as the left-hand merchant list so the
// button label stays truthful immediately after any toggle.
func (s *State) allMerchantsLockedCount() int {
	if s.SelectedChar < 0 {
		return 0
	}
	unlocked := s.displayMerchantUnlocked()
	n := 0
	for name, total := range s.merchantGatedTotal {
		n += total - unlocked[name]
	}
	return n
}

// flagsColumnLockedCount is the header's "Unlock this merchant (N)" count --
// her own gated-row groups plus, for Twin Maiden Husks specifically, every
// still-unacquired bell bearing (charunlock.BellBearingsForUI), since that
// button covers both staging mechanisms (see the unlockAllBtn handler in
// layoutCharactersPanel).
func (s *State) flagsColumnLockedCount() int {
	n := 0
	for _, g := range groupFlagRows(s.FlagRows) {
		if !s.effectiveFlagGroupUnlocked(g) {
			n++
		}
	}
	if s.UnlockMerchant == twinMaidenHusksMerchantName {
		for _, b := range charunlock.BellBearingsForUI() {
			if !s.effectiveBellBearingUnlocked(b.FlagID) {
				n++
			}
		}
	}
	return n
}

// flagGroupCheckbox resolves a flag group's retained checkbox and re-seeds
// its .Value from FlagState overlaid with any staged PendingFlagEdits (staged
// wins), returning whether a staged edit was found so the caller can amber-
// tint the label. Shared by layoutFlagRow and layoutFlagRowNamed, whose only
// real difference is which label/subtext they draw around this same checkbox.
// Overwriting .Value here is safe: this runs during layout, AFTER
// layoutCharactersPanel's own chk.Update(gtx)/stageFlag pass already consumed
// this frame's click and updated PendingFlagEdits, so recomputing from it
// reflects that same click, not a stale prior one. Necessary because a toggle
// staged from OUTSIDE this column (the Shop Editor row-edit bar's Unlock/Lock
// button via setRowUnlockForSelectedChar) only updates PendingFlagEdits, so a
// column left expanded across a view switch would otherwise show a stale
// checkbox.
func (s *State) flagGroupCheckbox(g flagGroup) (chk *widget.Bool, staged bool) {
	val := s.FlagState[g.Rows[0].RowID]
	pending := s.PendingFlagEdits[s.SelectedChar]
	for _, r := range g.Rows {
		if target, ok := pending[r.RowID]; ok {
			val, staged = target, true
			break
		}
	}
	chk = s.flagCheck(g.FlagID)
	chk.Value = val
	return chk, staged
}

// layoutFlagRow draws one flag-group checkbox, re-deriving its checked state
// via flagGroupCheckbox (see there for why re-deriving every render, rather
// than trusting the last-seeded .Value, is both necessary and safe).
func (s *State) layoutFlagRow(gtx layout.Context, th *material.Theme, g flagGroup) layout.Dimensions {
	chk, staged := s.flagGroupCheckbox(g)

	label := flagGroupLabel(g)
	box := material.CheckBox(th, chk, label)
	if staged {
		box.Color = colorAmber
	}
	return layout.Inset{Top: 2, Bottom: 2}.Layout(gtx, box.Layout)
}
