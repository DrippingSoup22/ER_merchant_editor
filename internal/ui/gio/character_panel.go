package gio

// Characters view (see docs/CHAR_UNLOCK.md): the app's landing view. Top
// bar: an Open-file bar (typed path + Load, or Browse...). Middle, 3
// columns: characters -> merchants with gated stock for the picked
// character -> that merchant's gated rows as checkboxes (checked =
// unlocked) -- toggle either direction.
//
// A checkbox toggle only stages a change (PendingFlagEdits, mirroring
// PendingEdits' staging model for item edits); writing happens through the
// shared Save button (state.go's startCombinedSave), which commits staged
// flag edits via internal/character -- a completely different file region/write
// engine than item edits' regulation.bin (see startCombinedSave's doc
// comment for how the two merge into one output file) -- alongside any
// staged item edits.
//
// Each gated row resolves to exactly one UnlockFlag (internal/catalog decodes
// it), so a flag IS a row's checkbox -- no separate flag-browsing step.

import (
	"fmt"
	"os"
	"strings"

	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/character"
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
// gets -- see docs/CHAR_UNLOCK.md and character.BellBearing.
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
	s.CharList = character.ListCharacters(data)
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
			states, err := character.LockStates(s.charSaveData, s.SelectedChar, rows)
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
		states, err := character.LockStates(s.charSaveData, s.SelectedChar, rows)
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
		bb := character.BellBearingsForUI()
		ids := make([]uint32, len(bb))
		for i, b := range bb {
			ids[i] = b.FlagID
		}
		if bbStates, err := character.FlagStates(s.charSaveData, s.SelectedChar, ids); err == nil {
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
	states, err := character.LockStates(s.charSaveData, s.SelectedChar, allRows)
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
	bb := character.BellBearingsForUI()
	ids := make([]uint32, len(bb))
	for i, b := range bb {
		ids[i] = b.FlagID
	}
	bbStates, err := character.FlagStates(s.charSaveData, s.SelectedChar, ids)
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
