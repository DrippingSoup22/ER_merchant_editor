// Package charflags resolves Elden Ring event-flag IDs (the same IDs
// ShopLineupParam rows reference via eventFlag_forRelease, see
// internal/catalog's enrich.go) to their byte/bit position within a character
// slot's event-flags bitfield, and reads/sets them.
//
// The resolution algorithm and its two data tables (bst.go,
// event_flags_exceptions.go) are ported from EldenRing-SaveForge
// (github.com/oisis/EldenRing-SaveForge, GPLv3) — see
// docs/SAVEFORGE_REFERENCE.md for attribution. The packed bit layout could
// not be independently reconstructed from public Paramdex/eventparam data
// (confirmed by a falsifying counter-example, flag id 60150 — see
// docs/PROJECT.md); this project relicensed MIT -> GPLv3 specifically to
// permit this reuse.
package flags

import "fmt"

// FlagsByteCount is the fixed length of a character slot's event-flags
// bitfield.
const FlagsByteCount = 0x1BF99F

// resolvePosition returns the byte offset and bit index (bit 7 = MSB) for
// an event flag ID within the FlagsByteCount-byte bitfield. Resolution
// order: hand-verified exception table, then the BST block table, then the
// trivial fallback formula (byte=id/8, bit=7-(id%8)) — same order as
// upstream's resolveEventFlagPosition.
func resolvePosition(id uint32) (byteIdx uint32, bitIdx uint8) {
	if info, ok := eventFlagExceptions[id]; ok {
		return info.Byte, info.Bit
	}
	loadBST()
	block := id / bstFlagDivisor
	if pos, ok := bstBlockPos[block]; ok {
		idx := id % bstFlagDivisor
		return pos*bstBlockSize + idx/8, uint8(7 - idx%8)
	}
	return id / 8, uint8(7 - id%8)
}

// Position returns the byte offset and bit index (bit 7 = MSB) flag id
// resolves to, without touching any buffer. Exposed for callers (e.g.
// internal/character's write-path round-trip check) that need to reason about
// which bytes a Set call will touch before calling it.
func Position(id uint32) (byteIdx uint32, bitIdx uint8) {
	return resolvePosition(id)
}

// Get reports whether flag id is set in flags (a FlagsByteCount-byte
// bitfield, e.g. a character slot's raw event-flags region).
func Get(flags []byte, id uint32) (bool, error) {
	byteIdx, bitIdx := resolvePosition(id)
	if int(byteIdx) >= len(flags) {
		return false, fmt.Errorf("charflags: flag %d (byte 0x%X) out of bounds (flags len %d)", id, byteIdx, len(flags))
	}
	return flags[byteIdx]&(1<<bitIdx) != 0, nil
}

// Set sets or clears flag id in flags in place.
func Set(flags []byte, id uint32, value bool) error {
	byteIdx, bitIdx := resolvePosition(id)
	if int(byteIdx) >= len(flags) {
		return fmt.Errorf("charflags: flag %d (byte 0x%X) out of bounds (flags len %d)", id, byteIdx, len(flags))
	}
	if value {
		flags[byteIdx] |= 1 << bitIdx
	} else {
		flags[byteIdx] &^= 1 << bitIdx
	}
	return nil
}
