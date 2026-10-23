package character

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/DrippingSoup22/ER_shared/charslot"
	reg "github.com/DrippingSoup22/ER_shared/regulation"
)

// The game's boss-defeat flags. Enia's release flags are among them (her
// Radahn armour and the Remembrance of the Starscourge both use 9130), and
// setting or clearing one marks a boss defeated or alive: on real hardware it
// forced the boss's cutscene and a teleport. No other merchant's row and no
// bell bearing uses the range, so no flag in it is ever written, whatever asks.
const (
	firstBossDefeatFlag = 9100
	lastBossDefeatFlag  = 9199
)

// ErrBossDefeatFlag refuses a write to a boss-defeat flag.
var ErrBossDefeatFlag = errors.New("a boss-defeat flag is never written")

// FlagTarget is one flag's wanted state: a row's gate or a bell bearing.
// Label only names it in error messages.
type FlagTarget struct {
	FlagID   int64
	Released bool
	Label    string
}

func (t FlagTarget) diagLabel() string {
	if t.Label != "" {
		return t.Label
	}
	return fmt.Sprintf("flag %d", t.FlagID)
}

// setReleaseBatch sets each target's flag for one character, in place in
// saveData; flags already right are left alone. Event flags sit outside the
// slot's own hash (SaveForge's CSPlayerGameDataHash covers level, stats and
// equipment), so only a PC slot's MD5 needs refreshing. The edit is made on a
// copy and checked first - only the named bytes changed, every flag reads back
// - so a failure leaves saveData untouched.
func setReleaseBatch(saveData []byte, charIndex int, targets []FlagTarget) (int, error) {
	flags, err := eventFlags(saveData, charIndex)
	if err != nil {
		return 0, err
	}

	work := append([]byte(nil), flags...)
	type touch struct {
		byteIdx  int
		label    string
		flagID   uint32
		released bool
	}
	var touches []touch
	for _, t := range targets {
		if t.FlagID == 0 {
			continue
		}
		if t.FlagID >= firstBossDefeatFlag && t.FlagID <= lastBossDefeatFlag {
			return 0, fmt.Errorf("charunlock: %s: flag %d: %w", t.diagLabel(), t.FlagID, ErrBossDefeatFlag)
		}
		flagID := uint32(t.FlagID)
		already, err := charslot.GetFlag(work, flagID)
		if err != nil {
			return 0, fmt.Errorf("charunlock: %s: %w", t.diagLabel(), err)
		}
		if already == t.Released {
			continue
		}
		byteIdx, _, err := charslot.FlagPosition(flagID)
		if err != nil {
			return 0, fmt.Errorf("charunlock: %s: %w", t.diagLabel(), err)
		}
		if err := charslot.SetFlag(work, flagID, t.Released); err != nil {
			return 0, fmt.Errorf("charunlock: %s: %w", t.diagLabel(), err)
		}
		touches = append(touches, touch{byteIdx: byteIdx, label: t.diagLabel(), flagID: flagID, released: t.Released})
	}
	if len(touches) == 0 {
		return 0, nil
	}

	// Round-trip self-check 1: no byte outside the set of bytes this call
	// intended to touch actually changed.
	touchedBytes := make(map[int]bool, len(touches))
	for _, t := range touches {
		touchedBytes[t.byteIdx] = true
	}
	for i := range flags {
		if touchedBytes[i] {
			continue
		}
		if work[i] != flags[i] {
			return 0, fmt.Errorf("charunlock: unexpected byte change at flags offset 0x%X outside the %d intended writes — aborted, saveData not modified", i, len(touches))
		}
	}
	// Round-trip self-check 2: every intended flag now reads back at its
	// own target value.
	for _, t := range touches {
		set, err := charslot.GetFlag(work, t.flagID)
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

// RowTargets is one FlagTarget per row, all to the same released state.
func RowTargets[T Gate](rows []T, released bool) []FlagTarget {
	targets := make([]FlagTarget, len(rows))
	for i, r := range rows {
		rowID, flagID := r.CharacterGate()
		targets[i] = FlagTarget{FlagID: flagID, Released: released, Label: fmt.Sprintf("row %d", rowID)}
	}
	return targets
}

// ApplyBatchToFile applies each character's FlagTargets to one copy of
// inPath and writes it to outPath, which may replace a file the user chose but
// never inPath. Characters' flags never overlap, so the order does not matter.
// It returns how many flags changed in all.
func ApplyBatchToFile(inPath, outPath string, edits map[int][]FlagTarget) (int, error) {
	if inPath == "" || outPath == "" {
		return 0, fmt.Errorf("charunlock: inPath and outPath are both required")
	}
	data, err := os.ReadFile(inPath)
	if err != nil {
		return 0, fmt.Errorf("charunlock: read %s: %w", inPath, err)
	}
	total := 0
	for charIndex, targets := range edits {
		n, err := setReleaseBatch(data, charIndex, targets)
		if err != nil {
			return 0, err
		}
		total += n
	}
	err = reg.ReplaceAtomicValidated(inPath, outPath, data, func(candidate string) error {
		written, err := os.ReadFile(candidate)
		if err != nil {
			return err
		}
		if !bytes.Equal(written, data) {
			return fmt.Errorf("the written save reads back different bytes")
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("charunlock: write %s: %w", outPath, err)
	}
	return total, nil
}
