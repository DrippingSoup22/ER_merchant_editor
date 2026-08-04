// Package charslot locates and reads the parts of a PS4 .sl2 save's
// character-slot region needed by the per-character merchant-unlock
// feature: each slot's event-flags bitfield (consumed by internal/character/flags)
// and basic character identity (name/level, for a slot picker UI).
//
// The top-level container layout (header size, slot count/size) was
// already established independently in this repo (see
// tools/savescan.py's PS4_HEADER_SIZE/SLOT_SIZE/NUM_SLOTS, cross-checked
// against docs/ER_PVP_MOD_REFERENCE.md). The event-flags anchor (an
// 8-byte TutorialData magic + constant 0x425 offset) was also found
// independently by inspecting this project's own fixture saves — see
// docs/PROJECT.md's 2026-07-28 entry. Only the character-identity anchor
// (magicPattern + offCharacterName/offLevel) is ported from
// EldenRing-SaveForge (GPLv3) — see docs/SAVEFORGE_REFERENCE.md.
package slot

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"unicode/utf16"
)

const (
	// HeaderSize is the fixed PS4 .sl2 header preceding the first slot.
	HeaderSize = 0x70
	// SlotSize is the fixed size of every character slot.
	SlotSize = 0x280000
	// NumSlots is the fixed number of character slots in a PS4 save.
	NumSlots = 10
)

// Slots splits a full save file's bytes into its NumSlots fixed-size
// character-slot regions (after HeaderSize). Panics if data is too short
// — callers should validate file size against USERDATA11's own bounds
// check first, as the rest of this codebase already does.
func Slots(data []byte) [][]byte {
	out := make([][]byte, NumSlots)
	for i := 0; i < NumSlots; i++ {
		start := HeaderSize + i*SlotSize
		out[i] = data[start : start+SlotSize]
	}
	return out
}

// Version is the u32 at the very start of a slot; 0 for an empty/unused
// slot, nonzero for a real character (confirmed empirically against both
// fixture saves — every nonzero-version slot has a real character, every
// version==0 slot has none of the markers below).
func Version(slot []byte) uint32 {
	return binary.LittleEndian.Uint32(slot[0:4])
}

// IsEmpty reports whether a slot holds no character.
func IsEmpty(slot []byte) bool {
	return Version(slot) == 0
}

// tutorialMagic marks the start of the slot's TutorialData block.
// Independently found (2026-07-28) by inspecting this project's own
// fixture saves — appears exactly once per real slot.
var tutorialMagic = []byte{0xAE, 0x00, 0x01, 0x00, 0x00, 0x04, 0x00, 0x00}

// eventFlagsOffsetFromTutorial is the constant byte distance from the
// start of tutorialMagic to the start of the event-flags bitfield.
// Independently verified byte-exact against all 15 real slots across both
// of this project's fixture saves.
const eventFlagsOffsetFromTutorial = 0x425

// EventFlagsOffset returns the byte offset (within slot) of the start of
// the event-flags bitfield internal/character/flags operates on. Returns an error if
// the anchor pattern isn't found (e.g. slot is empty).
func EventFlagsOffset(slot []byte) (int, error) {
	i := bytes.Index(slot, tutorialMagic)
	if i == -1 {
		return 0, fmt.Errorf("charslot: TutorialData magic not found (empty or unrecognized slot)")
	}
	off := i + eventFlagsOffsetFromTutorial
	if off < 0 || off >= len(slot) {
		return 0, fmt.Errorf("charslot: computed EventFlagsOffset 0x%X out of bounds", off)
	}
	return off, nil
}

// magicPattern anchors PlayerGameData, from which character name and
// level are read at fixed negative offsets. Ported from
// EldenRing-SaveForge (github.com/oisis/EldenRing-SaveForge), GPLv3 —
// backend/core/structures.go's MagicPattern — see
// docs/SAVEFORGE_REFERENCE.md.
var magicPattern = []byte{
	0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// Offsets relative to the found magicPattern position. Ported from
// EldenRing-SaveForge's offset_defs.go (OffCharacterName, OffLevel),
// GPLv3 — see docs/SAVEFORGE_REFERENCE.md.
const (
	offCharacterName = -0x11B // 16 x uint16 UTF-16LE, null-terminated
	offLevel         = -335
)

// Identity is a character slot's display identity for a slot picker UI.
type Identity struct {
	Name  string
	Level uint32
}

// ReadIdentity locates magicPattern in slot and reads the character name
// and level relative to it.
func ReadIdentity(slot []byte) (Identity, error) {
	i := bytes.Index(slot, magicPattern)
	if i == -1 {
		return Identity{}, fmt.Errorf("charslot: identity magic pattern not found (empty or unrecognized slot)")
	}
	nameOff := i + offCharacterName
	levelOff := i + offLevel
	if nameOff < 0 || nameOff+32 > len(slot) || levelOff < 0 || levelOff+4 > len(slot) {
		return Identity{}, fmt.Errorf("charslot: identity offsets out of bounds (magic at 0x%X)", i)
	}
	units := make([]uint16, 16)
	for k := range units {
		units[k] = binary.LittleEndian.Uint16(slot[nameOff+k*2:])
	}
	end := len(units)
	for k, u := range units {
		if u == 0 {
			end = k
			break
		}
	}
	name := string(utf16.Decode(units[:end]))
	level := binary.LittleEndian.Uint32(slot[levelOff:])
	return Identity{Name: name, Level: level}, nil
}
