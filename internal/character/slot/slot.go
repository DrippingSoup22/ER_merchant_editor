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
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"unicode/utf16"
)

const (
	// HeaderSize is the fixed PS4 .sl2 header preceding the first slot.
	HeaderSize = 0x70
	// PCHeaderSize is the PC BND4 container header preceding the first slot.
	PCHeaderSize = 0x300
	// DigestSize is the MD5(region body) prefix PC gives every region. PS4
	// regions carry no digest.
	DigestSize = 0x10
	// SlotSize is the fixed size of every character slot.
	SlotSize = 0x280000
	// NumSlots is the fixed number of character slots in a save.
	NumSlots = 10
)

// isPC reports whether data is a PC BND4 container rather than a decrypted
// PlayStation one. Detection is by leading magic only, matching
// internal/savefile's classifier; the two must not disagree about a file.
func isPC(data []byte) bool {
	return len(data) >= 4 && string(data[0:4]) == "BND4"
}

// slotStart returns the byte offset of the charIndex'th slot body.
func slotStart(data []byte, i int) int {
	if isPC(data) {
		// PC prefixes every region with MD5(body), so each slot start is
		// shifted by one digest and the running total gains one per slot.
		return PCHeaderSize + i*(DigestSize+SlotSize) + DigestSize
	}
	return HeaderSize + i*SlotSize
}

// Slots splits a full save file's bytes into its NumSlots fixed-size
// character-slot regions. The container is detected from the data, so the
// same call works for both platforms. Panics if data is too short —
// callers should validate file size against USERDATA11's own bounds
// check first, as the rest of this codebase already does.
//
// The returned slices are backed by data: mutating one mutates the save,
// which is what the unlock path relies on. On PC that also invalidates the
// slot's digest, so a mutating caller must call RefreshDigest.
func Slots(data []byte) [][]byte {
	out := make([][]byte, NumSlots)
	for i := 0; i < NumSlots; i++ {
		start := slotStart(data, i)
		out[i] = data[start : start+SlotSize]
	}
	return out
}

// RefreshDigest recomputes the PC container's MD5 for the charIndex'th slot,
// in place. It is a no-op on PlayStation saves, which carry no digests.
//
// The game validates these digests, so a PC save whose slot was edited
// without this is rejected on load. Call it after every mutation.
func RefreshDigest(data []byte, i int) error {
	if !isPC(data) {
		return nil
	}
	if i < 0 || i >= NumSlots {
		return fmt.Errorf("charslot: slot index %d out of range [0,%d)", i, NumSlots)
	}
	start := slotStart(data, i)
	end := start + SlotSize
	if end > len(data) {
		return fmt.Errorf("charslot: slot %d region [0x%X,0x%X) exceeds file size 0x%X", i, start, end, len(data))
	}
	sum := md5.Sum(data[start:end])
	copy(data[start-DigestSize:start], sum[:])
	return nil
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
