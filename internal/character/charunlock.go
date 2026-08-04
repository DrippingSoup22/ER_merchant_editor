// Package charunlock cross-references ShopLineupParam rows'
// eventFlag_forRelease gate (internal/catalog's Row.UnlockFlag) against a
// chosen character's event-flags bitfield (internal/character/flags,
// internal/character/slot), to answer "which merchant rows does this character
// still have locked?". Read-only for now (Phase 1 of the per-character
// merchant-unlock feature — see docs/CHAR_UNLOCK.md); the write path
// (clearing a flag) is a later phase.
package character

import (
	"fmt"

	charflags "github.com/DrippingSoup22/ER_merchant_editor/internal/character/flags"
	charslot "github.com/DrippingSoup22/ER_merchant_editor/internal/character/slot"
)

// Gate is the minimal row information required to inspect a character's
// unlock state. Catalog rows satisfy it without this package importing the
// catalog package.
type Gate interface {
	CharacterGate() (rowID, flagID int64)
}

// Character is one save slot's identity, as shown in a character picker.
type Character struct {
	Index int
	charslot.Identity
}

// ListCharacters returns every non-empty character slot in saveData, in
// slot order. A slot whose identity can't be read (unrecognized/corrupt)
// is skipped rather than erroring the whole list.
func ListCharacters(saveData []byte) []Character {
	var out []Character
	for i, slot := range charslot.Slots(saveData) {
		if charslot.IsEmpty(slot) {
			continue
		}
		id, err := charslot.ReadIdentity(slot)
		if err != nil {
			continue
		}
		out = append(out, Character{Index: i, Identity: id})
	}
	return out
}

// eventFlags returns the charIndex'th slot's raw event-flags bitfield
// slice, backed by saveData (mutating it mutates saveData).
func eventFlags(saveData []byte, charIndex int) ([]byte, error) {
	slots := charslot.Slots(saveData)
	if charIndex < 0 || charIndex >= len(slots) {
		return nil, fmt.Errorf("charunlock: character index %d out of range [0,%d)", charIndex, len(slots))
	}
	slot := slots[charIndex]
	if charslot.IsEmpty(slot) {
		return nil, fmt.Errorf("charunlock: character slot %d is empty", charIndex)
	}
	off, err := charslot.EventFlagsOffset(slot)
	if err != nil {
		return nil, err
	}
	end := off + charflags.FlagsByteCount
	if end > len(slot) {
		return nil, fmt.Errorf("charunlock: event-flags region [0x%X,0x%X) exceeds slot size 0x%X", off, end, len(slot))
	}
	return slot[off:end], nil
}

// LockedRows returns the subset of rows that are gated
// (row.UnlockFlag != 0) and not yet released for the given character
// charslot. Rows with no gate (UnlockFlag == 0) are never included. A row
// whose flag can't be resolved to a real bitfield position (seen only on
// a handful of internal, non-merchant rows with a shared sentinel flag
// value -- see docs/CHAR_UNLOCK.md) is skipped rather than failing the
// whole batch.
func LockedRows[T Gate](saveData []byte, charIndex int, rows []T) ([]T, error) {
	flags, err := eventFlags(saveData, charIndex)
	if err != nil {
		return nil, err
	}
	var locked []T
	for _, r := range rows {
		_, flagID := r.CharacterGate()
		if flagID == 0 {
			continue
		}
		set, err := charflags.Get(flags, uint32(flagID))
		if err != nil {
			continue
		}
		if !set {
			locked = append(locked, r)
		}
	}
	return locked, nil
}

// LockStates returns, for every gated row (UnlockFlag != 0) in rows,
// whether it's currently released for the given character charslot. Unlike
// LockedRows this includes already-unlocked rows too -- callers rendering
// a full checkbox list (checked = unlocked) need the state of every gated
// row, not just the locked ones. Ungated rows and rows whose flag can't
// be resolved (see LockedRows's doc comment) are omitted from the map.
func LockStates[T Gate](saveData []byte, charIndex int, rows []T) (map[int64]bool, error) {
	flags, err := eventFlags(saveData, charIndex)
	if err != nil {
		return nil, err
	}
	states := make(map[int64]bool, len(rows))
	for _, r := range rows {
		rowID, flagID := r.CharacterGate()
		if flagID == 0 {
			continue
		}
		set, err := charflags.Get(flags, uint32(flagID))
		if err != nil {
			continue
		}
		states[rowID] = set
	}
	return states, nil
}

// FlagStates is LockStates' row-agnostic counterpart: resolves raw flag
// IDs directly, for callers with no backing catalog.Row (e.g. Twin Maiden
// Husks bell-bearing acquisition toggles — see BellBearing). An id whose
// position can't be resolved is omitted rather than failing the whole
// batch, mirroring LockStates' treatment of unresolvable rows.
func FlagStates(saveData []byte, charIndex int, flagIDs []uint32) (map[uint32]bool, error) {
	flags, err := eventFlags(saveData, charIndex)
	if err != nil {
		return nil, err
	}
	states := make(map[uint32]bool, len(flagIDs))
	for _, id := range flagIDs {
		set, err := charflags.Get(flags, id)
		if err != nil {
			continue
		}
		states[id] = set
	}
	return states, nil
}

// IsUnlocked reports whether a single gated row is released for the given
// character charslot. Rows with no gate are always reported unlocked.
func IsUnlocked[T Gate](saveData []byte, charIndex int, row T) (bool, error) {
	_, flagID := row.CharacterGate()
	if flagID == 0 {
		return true, nil
	}
	flags, err := eventFlags(saveData, charIndex)
	if err != nil {
		return false, err
	}
	return charflags.Get(flags, uint32(flagID))
}
