// Package character reads and writes the event flags that gate merchant rows
// and bell bearings, one character at a time.
package character

import (
	"fmt"

	"github.com/DrippingSoup22/ER_shared/charslot"
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
	slots, err := charslot.Slots(saveData)
	if err != nil {
		return nil
	}
	var out []Character
	for i, slot := range slots {
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
	slots, err := charslot.Slots(saveData)
	if err != nil {
		return nil, err
	}
	if charIndex < 0 || charIndex >= len(slots) {
		return nil, fmt.Errorf("charunlock: character index %d out of range [0,%d)", charIndex, len(slots))
	}
	slot := slots[charIndex]
	if charslot.IsEmpty(slot) {
		return nil, fmt.Errorf("charunlock: character slot %d is empty", charIndex)
	}
	return charslot.EventFlags(slot)
}

// LockedRows is the gated rows the character has not released.
func LockedRows[T Gate](saveData []byte, charIndex int, rows []T) ([]T, error) {
	states, err := LockStates(saveData, charIndex, rows)
	if err != nil {
		return nil, err
	}
	var locked []T
	for _, r := range rows {
		rowID, _ := r.CharacterGate()
		if released, gated := states[rowID]; gated && !released {
			locked = append(locked, r)
		}
	}
	return locked, nil
}

// LockStates is whether the character has released each gated row, for a
// checkbox list. Ungated rows are left out, and so are rows whose flag is not
// in the game's table (only internal loadout rows).
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
		set, err := charslot.GetFlag(flags, uint32(flagID))
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
		set, err := charslot.GetFlag(flags, id)
		if err != nil {
			continue
		}
		states[id] = set
	}
	return states, nil
}
