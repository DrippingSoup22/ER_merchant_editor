package character

import (
	"fmt"
	"os"

	charflags "github.com/DrippingSoup22/ER_merchant_editor/internal/character/flags"
	charslot "github.com/DrippingSoup22/ER_merchant_editor/internal/character/slot"
)

// FlagTarget is one flag's desired released state, for a mixed-direction
// batch write (SetReleaseBatch) — e.g. the GUI staging some rows/bell
// bearings to unlock and others to re-lock for the same character before
// one Save. Row-agnostic: carries the raw flag ID directly, since
// SetReleaseBatch never needed catalog.Row for anything but its
// UnlockFlag — this lets non-row flags (Twin Maiden Husks bell-bearing
// acquisitions, see character.BellBearing) reuse the exact same
// round-trip-checked write path. Label is optional, diagnostic-only text
// folded into error messages (e.g. "row 101801" or a bell bearing's
// name) — purely cosmetic, never affects behavior.
type FlagTarget struct {
	FlagID   int64
	Released bool
	Label    string
}

// diagLabel returns t.Label if set, else a generic "flag <id>" fallback,
// for error-message text only.
func (t FlagTarget) diagLabel() string {
	if t.Label != "" {
		return t.Label
	}
	return fmt.Sprintf("flag %d", t.FlagID)
}

// SetReleaseBatch sets each target's row to its Released state for the
// given character slot, in place in saveData — unlike SetRelease, targets
// may mix both directions in one call. Rows already at their target value
// are left alone; rows with no gate (UnlockFlag == 0) are ignored.
//
// Safety: no slot-INTERNAL checksum needs recomputing for this edit —
// EldenRing-SaveForge's own CSPlayerGameDataHash (the only slot-internal
// hash found; see docs/CHAR_UNLOCK.md) hashes Level/Stats/Class/Souls/
// SoulMemory/Equipment, never the event-flags region, so flipping a flag
// bit either direction cannot desync it. The PC container's per-region
// MD5 is a different matter and IS refreshed below, because the game
// validates it and the flags region sits inside it. What SetReleaseBatch still guards
// against, mirroring savefile.Apply's round-trip self-check: a bug
// in charflags' resolution accidentally aliasing two different flag IDs
// onto the same byte, or touching bytes outside the flags it was asked to
// change. Nothing is written to saveData unless every check passes; on
// any failure saveData is left byte-for-byte unmodified and an error is
// returned.
func SetReleaseBatch(saveData []byte, charIndex int, targets []FlagTarget) (int, error) {
	flags, err := eventFlags(saveData, charIndex)
	if err != nil {
		return 0, err
	}

	work := append([]byte(nil), flags...)
	type touch struct {
		byteIdx  uint32
		label    string
		flagID   uint32
		released bool
	}
	var touches []touch
	for _, t := range targets {
		if t.FlagID == 0 {
			continue
		}
		flagID := uint32(t.FlagID)
		already, err := charflags.Get(work, flagID)
		if err != nil {
			return 0, fmt.Errorf("charunlock: %s: %w", t.diagLabel(), err)
		}
		if already == t.Released {
			continue
		}
		byteIdx, _ := charflags.Position(flagID)
		if err := charflags.Set(work, flagID, t.Released); err != nil {
			return 0, fmt.Errorf("charunlock: %s: %w", t.diagLabel(), err)
		}
		touches = append(touches, touch{byteIdx: byteIdx, label: t.diagLabel(), flagID: flagID, released: t.Released})
	}
	if len(touches) == 0 {
		return 0, nil
	}

	// Round-trip self-check 1: no byte outside the set of bytes this call
	// intended to touch actually changed.
	touchedBytes := make(map[uint32]bool, len(touches))
	for _, t := range touches {
		touchedBytes[t.byteIdx] = true
	}
	for i := range flags {
		if touchedBytes[uint32(i)] {
			continue
		}
		if work[i] != flags[i] {
			return 0, fmt.Errorf("charunlock: unexpected byte change at flags offset 0x%X outside the %d intended writes — aborted, saveData not modified", i, len(touches))
		}
	}
	// Round-trip self-check 2: every intended flag now reads back at its
	// own target value.
	for _, t := range touches {
		set, err := charflags.Get(work, t.flagID)
		if err != nil || set != t.released {
			return 0, fmt.Errorf("charunlock: round-trip check failed for %s (flag %d) — aborted, saveData not modified", t.label, t.flagID)
		}
	}

	copy(flags, work)
	// The slot-internal hash above needs no refresh, but the PC container
	// wraps every region in an MD5(body) the game does validate, and flags
	// live inside that region. No-op on PlayStation.
	if err := charslot.RefreshDigest(saveData, charIndex); err != nil {
		return 0, fmt.Errorf("charunlock: %w", err)
	}
	return len(touches), nil
}

// SetRelease sets every gated row in rows to the same released state for
// the given character charslot. A thin SetReleaseBatch wrapper for the common
// single-direction case (the CLI, and "Unlock all remaining").
func SetRelease[T Gate](saveData []byte, charIndex int, rows []T, released bool) (int, error) {
	targets := make([]FlagTarget, len(rows))
	for i, r := range rows {
		rowID, flagID := r.CharacterGate()
		targets[i] = FlagTarget{FlagID: flagID, Released: released, Label: fmt.Sprintf("row %d", rowID)}
	}
	return SetReleaseBatch(saveData, charIndex, targets)
}

// SetBellBearingsBatch sets each bell-bearing flag ID's released state for
// the given character slot — BellBearing's flag-ID counterpart to
// SetReleaseBatch/SetRelease, sharing the exact same write path (this is
// a thin FlagTarget-building wrapper, no new write logic).
func SetBellBearingsBatch(saveData []byte, charIndex int, targets map[uint32]bool) (int, error) {
	ft := make([]FlagTarget, 0, len(targets))
	for id, released := range targets {
		ft = append(ft, FlagTarget{FlagID: int64(id), Released: released, Label: fmt.Sprintf("bell bearing flag %d", id)})
	}
	return SetReleaseBatch(saveData, charIndex, ft)
}

// Unlock sets (releases) the UnlockFlag bit for every gated row in rows
// that isn't already released. A thin convenience wrapper over
// SetRelease(..., true) kept for the CLI's original default behavior.
func Unlock[T Gate](saveData []byte, charIndex int, rows []T) (int, error) {
	return SetRelease(saveData, charIndex, rows, true)
}

// ApplyBatchToFile reads inPath, applies edits (character slot index ->
// that character's mixed-direction FlagTargets) to the same in-memory
// buffer in turn -- one character's flags never overlap another's, so
// applying several characters' edits sequentially before one write is
// safe -- and writes the full result to outPath, never inPath (mirrors
// savefile.Apply's "never overwrite the input" convention). Returns
// the total number of rows actually changed across every character.
func ApplyBatchToFile(inPath, outPath string, edits map[int][]FlagTarget) (int, error) {
	if inPath == "" || outPath == "" {
		return 0, fmt.Errorf("charunlock: inPath and outPath are both required")
	}
	if inPath == outPath {
		return 0, fmt.Errorf("charunlock: outPath must differ from inPath (never overwrite the input save)")
	}
	data, err := os.ReadFile(inPath)
	if err != nil {
		return 0, fmt.Errorf("charunlock: read %s: %w", inPath, err)
	}
	total := 0
	for charIndex, targets := range edits {
		n, err := SetReleaseBatch(data, charIndex, targets)
		if err != nil {
			return 0, err
		}
		total += n
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return 0, fmt.Errorf("charunlock: write %s: %w", outPath, err)
	}
	return total, nil
}
