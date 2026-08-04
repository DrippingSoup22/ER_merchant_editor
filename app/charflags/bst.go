package charflags

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

// eventflagBSTRaw is the block-position table for the BST (block-search-tree
// style) bit-packing scheme the game uses for most event flags: flags are
// grouped into blocks of 1000 consecutive IDs, and each block is packed at
// some (non-sequential) 125-byte-aligned position within the flags bitfield.
// Ported verbatim from EldenRing-SaveForge
// (github.com/oisis/EldenRing-SaveForge), backend/db/data/eventflag_bst.txt,
// GPLv3 — see docs/SAVEFORGE_REFERENCE.md. Could not be independently
// reconstructed from public Paramdex/eventparam documentation (see
// docs/PROJECT.md and the character-flag-unlock-feature research notes).
//
//go:embed eventflag_bst.txt
var eventflagBSTRaw string

// bstBlockPos maps a block number (flagID / bstFlagDivisor) to its packed
// position in the flags bitfield, in units of bstBlockSize bytes.
var bstBlockPos map[uint32]uint32

var bstOnce sync.Once

func loadBST() {
	bstOnce.Do(func() {
		lines := strings.Split(eventflagBSTRaw, "\n")
		bstBlockPos = make(map[uint32]uint32, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ",", 2)
			if len(parts) != 2 {
				continue
			}
			block, err1 := strconv.ParseUint(parts[0], 10, 32)
			pos, err2 := strconv.ParseUint(parts[1], 10, 32)
			if err1 != nil || err2 != nil {
				continue
			}
			bstBlockPos[uint32(block)] = uint32(pos)
		}
	})
}

const (
	// bstBlockSize is the number of bytes per BST block (1000 flags / 8 = 125 bytes).
	bstBlockSize = 125
	// bstFlagDivisor is the number of flags per BST block.
	bstFlagDivisor = 1000
)
