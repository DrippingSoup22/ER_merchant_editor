package gio

import (
	"gioui.org/widget"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/character"
)

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
		states, err := character.LockStates(s.charSaveData, idx, group)
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
