package character

import (
	"bytes"
	"crypto/md5"
	"os"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_shared/charslot"
	"github.com/DrippingSoup22/ER_shared/regulation"
)

// The private saves: a PS4 save with eight characters and a PC save.
const (
	ps4Fixture = "vanilla_fresh_character.dat"
	pcFixture  = "pc_fixture.sl2"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "save_files", name))
	if err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	return data
}

// merchantRows is every row a merchant sells: what the GUI passes here. The
// game's internal loadout rows are left out; their gate flags are not in the
// game's flag table.
func merchantRows(t *testing.T) []*catalog.Row {
	t.Helper()
	readFixture(t, ps4Fixture)
	c, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.LoadSave(filepath.Join("..", "..", "save_files", ps4Fixture)); err != nil {
		t.Fatal(err)
	}
	merchants, err := c.ListMerchants()
	if err != nil {
		t.Fatal(err)
	}
	var rows []*catalog.Row
	for _, m := range merchants {
		merchantRows, err := c.MerchantRows(m.Name)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, merchantRows...)
	}
	return rows
}

func TestListCharacters(t *testing.T) {
	characters := ListCharacters(readFixture(t, ps4Fixture))
	if len(characters) != 8 || characters[0].Name != "DrippingSoup" || characters[0].Level != 146 {
		t.Fatalf("characters = %+v, want 8 starting with DrippingSoup level 146", characters)
	}
}

// LockedRows and LockStates are two views of one bitfield.
func TestLockQueriesAgree(t *testing.T) {
	data := readFixture(t, ps4Fixture)
	rows := merchantRows(t)
	const character = 7
	locked, err := LockedRows(data, character, rows)
	if err != nil {
		t.Fatal(err)
	}
	states, err := LockStates(data, character, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(locked) == 0 || len(states) == len(locked) {
		t.Fatalf("%d locked of %d gated rows: the fixture should hold both kinds", len(locked), len(states))
	}
	for _, row := range locked {
		if released, gated := states[row.RowID]; !gated || released {
			t.Errorf("row %d: locked, but LockStates says released %v, gated %v", row.RowID, released, gated)
		}
	}
	stillLocked := 0
	for _, released := range states {
		if !released {
			stillLocked++
		}
	}
	if stillLocked != len(locked) {
		t.Errorf("LockStates has %d locked rows, LockedRows %d", stillLocked, len(locked))
	}
}

// A mixed batch changes only the bits it names - and, on PC, the slot's
// digest, which must then match - is a no-op repeated, and its reverse
// restores the original bytes.
func TestSetReleaseBatchChangesOnlyItsFlags(t *testing.T) {
	rows := merchantRows(t)
	for _, name := range []string{ps4Fixture, pcFixture} {
		original := readFixture(t, name)
		data := bytes.Clone(original)
		character := ListCharacters(data)[0].Index
		states, err := LockStates(data, character, rows)
		if err != nil {
			t.Fatal(err)
		}
		// Two locked rows and one unlocked, each behind its own flag (several
		// rows can share one), plus a bell bearing.
		var batch []FlagTarget
		used := map[int64]bool{}
		for _, row := range rows {
			if released, gated := states[row.RowID]; gated && !used[row.UnlockFlag] && len(batch) < 3 && (len(batch) < 2) != released {
				batch = append(batch, FlagTarget{FlagID: row.UnlockFlag, Released: !released})
				used[row.UnlockFlag] = true
			}
		}
		bearing := BellBearings[0]
		bearingState, _ := FlagStates(data, character, []uint32{bearing.FlagID})
		batch = append(batch, FlagTarget{FlagID: int64(bearing.FlagID), Released: !bearingState[bearing.FlagID]})

		changed, err := setReleaseBatch(data, character, batch)
		if err != nil || changed != len(batch) {
			t.Fatalf("%s: setReleaseBatch = %d, %v; want %d", name, changed, err, len(batch))
		}
		originalSlots, _ := charslot.Slots(original)
		originalFlags, _ := charslot.EventFlags(originalSlots[character])
		slots, _ := charslot.Slots(data)
		flags, _ := charslot.EventFlags(slots[character])
		named := map[int]bool{}
		for _, target := range batch {
			byteIndex, _, _ := charslot.FlagPosition(uint32(target.FlagID))
			named[byteIndex] = true
			if on, _ := charslot.GetFlag(flags, uint32(target.FlagID)); on != target.Released {
				t.Errorf("%s: flag %d reads %v", name, target.FlagID, on)
			}
		}
		for index := range flags {
			if flags[index] != originalFlags[index] && !named[index] {
				t.Fatalf("%s: flag byte 0x%X changed outside the batch", name, index)
			}
		}
		// With the flag bytes put back, only a PC slot's digest may differ.
		edited := bytes.Clone(flags)
		copy(flags, originalFlags)
		platform, _ := regulation.DetectPlatform(data)
		digest := regulation.SlotOffset(platform, character) - regulation.DigestSize
		if platform == regulation.PC {
			copy(data[digest:], original[digest:digest+regulation.DigestSize])
		}
		if !bytes.Equal(data, original) {
			t.Fatalf("%s: bytes outside the flags changed", name)
		}
		copy(flags, edited)
		if platform == regulation.PC {
			if err := charslot.RefreshDigest(data, character); err != nil {
				t.Fatal(err)
			}
			if sum := md5.Sum(slots[character]); !bytes.Equal(data[digest:digest+regulation.DigestSize], sum[:]) {
				t.Fatal("unreachable: RefreshDigest wrote a wrong digest")
			}
		}

		if again, err := setReleaseBatch(data, character, batch); again != 0 || err != nil {
			t.Errorf("%s: repeating the batch changed %d flags, %v", name, again, err)
		}
		for index := range batch {
			batch[index].Released = !batch[index].Released
		}
		if _, err := setReleaseBatch(data, character, batch); err != nil || !bytes.Equal(data, original) {
			t.Errorf("%s: the reversed batch did not restore the original (%v)", name, err)
		}
	}
}

// Two targets naming one flag in opposite directions cannot both hold: the
// read-back check refuses the batch and leaves the save untouched.
func TestSetReleaseBatchRefusesContradictoryTargets(t *testing.T) {
	data := readFixture(t, ps4Fixture)
	original := bytes.Clone(data)
	flag := int64(BellBearings[0].FlagID)
	_, err := setReleaseBatch(data, 0, []FlagTarget{{FlagID: flag, Released: true}, {FlagID: flag, Released: false}})
	if err == nil || !bytes.Equal(data, original) {
		t.Fatalf("err = %v, save changed %v", err, !bytes.Equal(data, original))
	}
}

func TestApplyBatchToFileWritesEveryCharacter(t *testing.T) {
	data := readFixture(t, ps4Fixture)
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.dat"), filepath.Join(dir, "out.dat")
	if err := os.WriteFile(in, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyBatchToFile(in, in, nil); err == nil {
		t.Fatal("the input was accepted as the output")
	}
	flag := BellBearings[0].FlagID
	before, _ := FlagStates(data, 7, []uint32{flag})
	edits := map[int][]FlagTarget{
		2: {{FlagID: int64(flag), Released: true}},
		7: {{FlagID: int64(flag), Released: !before[flag]}},
	}
	if _, err := ApplyBatchToFile(in, out, edits); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for character, targets := range edits {
		if states, _ := FlagStates(written, character, []uint32{flag}); states[flag] != targets[0].Released {
			t.Errorf("character %d: flag %d is %v", character, flag, states[flag])
		}
	}
}
