package character

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	charflags "github.com/DrippingSoup22/ER_merchant_editor/internal/character/flags"
)

const fixture = "vanilla_fresh_character.dat"

func loadFixtureBytes(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "save_files", fixture)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	return data
}

// loadRows returns every row belonging to a real, browsable merchant --
// i.e. exactly what the GUI would ever pass to this package. It
// deliberately excludes catalog.ShopRows()'s handful of internal,
// merchant=nil rows (e.g. row_id 1600101-1600123/1600401-1600420, the
// game's internal starting-class-loadout table): those share sentinel
// eventFlag_forRelease values (99019775/99019780) that don't resolve to
// any real position in the flags bitfield even via SaveForge's own
// resolver -- confirmed via the oracle, not a bug in this package -- and
// are never reachable through Catalog.MerchantRows.
func loadRows(t *testing.T) []*catalog.Row {
	t.Helper()
	path := filepath.Join("..", "..", "save_files", fixture)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	c, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	if err := c.LoadSave(path); err != nil {
		t.Fatalf("LoadSave: %v", err)
	}
	merchants, err := c.ListMerchants()
	if err != nil {
		t.Fatalf("ListMerchants: %v", err)
	}
	var rows []*catalog.Row
	for _, m := range merchants {
		rs, err := c.MerchantRows(m.Name)
		if err != nil {
			t.Fatalf("MerchantRows(%q): %v", m.Name, err)
		}
		rows = append(rows, rs...)
	}
	return rows
}

func TestListCharacters(t *testing.T) {
	data := loadFixtureBytes(t)
	chars := ListCharacters(data)
	if len(chars) != 8 {
		t.Fatalf("ListCharacters: got %d characters, want 8", len(chars))
	}
	if chars[0].Name != "DrippingSoup" || chars[0].Level != 146 {
		t.Errorf("chars[0] = %+v, want DrippingSoup/146", chars[0])
	}
}

func TestLockedRowsConsistentWithIsUnlocked(t *testing.T) {
	data := loadFixtureBytes(t)
	rows := loadRows(t)

	gated := 0
	for _, r := range rows {
		if r.UnlockFlag != 0 {
			gated++
		}
	}
	if gated == 0 {
		t.Fatal("fixture has no gated rows to test against")
	}

	locked, err := LockedRows(data, 0, rows)
	if err != nil {
		t.Fatalf("LockedRows: %v", err)
	}
	lockedSet := make(map[int64]bool, len(locked))
	for _, r := range locked {
		lockedSet[r.RowID] = true
	}

	for _, r := range rows {
		unlocked, err := IsUnlocked(data, 0, r)
		if err != nil {
			t.Fatalf("IsUnlocked(row %d): %v", r.RowID, err)
		}
		wantLocked := r.UnlockFlag != 0 && !unlocked
		if wantLocked != lockedSet[r.RowID] {
			t.Errorf("row %d: LockedRows says locked=%v, IsUnlocked says locked=%v", r.RowID, lockedSet[r.RowID], wantLocked)
		}
		if r.UnlockFlag == 0 && !unlocked {
			t.Errorf("row %d: ungated row reported as not unlocked", r.RowID)
		}
	}
}

// TestFlagStatesMatchesGetDirectly cross-checks FlagStates (LockStates'
// row-agnostic counterpart, used for Twin Maiden Husks bell-bearing
// toggles) against charflags.Get on the same slot's raw bytes directly,
// for a couple of real flag IDs already known-good from
// internal/character/flags/flags_test.go's oracle-cross-checked cases.
func TestFlagStatesMatchesGetDirectly(t *testing.T) {
	data := loadFixtureBytes(t)
	ids := []uint32{11109759, 11109796}

	for charIndex := 0; charIndex < 2; charIndex++ {
		states, err := FlagStates(data, charIndex, ids)
		if err != nil {
			t.Fatalf("FlagStates(char %d): %v", charIndex, err)
		}
		flags, err := eventFlags(data, charIndex)
		if err != nil {
			t.Fatalf("eventFlags(char %d): %v", charIndex, err)
		}
		for _, id := range ids {
			want, err := charflags.Get(flags, id)
			if err != nil {
				t.Fatalf("charflags.Get(char %d, id %d): %v", charIndex, id, err)
			}
			if states[id] != want {
				t.Errorf("char %d, flag %d: FlagStates = %v, charflags.Get = %v", charIndex, id, states[id], want)
			}
		}
	}
}

// TestSetFlagChangesLockStatus flips one previously-locked row's flag on a
// copy of the save bytes and checks IsUnlocked reflects it, without
// disturbing an unrelated gated row's status. Read-only from the real
// fixture's perspective -- operates on an in-memory copy, writes nothing
// to disk.
func TestSetFlagChangesLockStatus(t *testing.T) {
	data := append([]byte(nil), loadFixtureBytes(t)...)
	rows := loadRows(t)

	var target, other *catalog.Row
	for _, r := range rows {
		if r.UnlockFlag == 0 {
			continue
		}
		unlocked, err := IsUnlocked(data, 0, r)
		if err != nil {
			t.Fatalf("IsUnlocked: %v", err)
		}
		if unlocked {
			continue
		}
		if target == nil {
			target = r
		} else if other == nil && r.UnlockFlag != target.UnlockFlag {
			other = r
		}
		if target != nil && other != nil {
			break
		}
	}
	if target == nil {
		t.Skip("no locked gated row found in fixture to flip")
	}

	flags, err := eventFlags(data, 0)
	if err != nil {
		t.Fatalf("eventFlags: %v", err)
	}
	if err := charflags.Set(flags, uint32(target.UnlockFlag), true); err != nil {
		t.Fatalf("Set: %v", err)
	}

	unlocked, err := IsUnlocked(data, 0, target)
	if err != nil {
		t.Fatalf("IsUnlocked after Set: %v", err)
	}
	if !unlocked {
		t.Errorf("row %d still locked after setting its flag", target.RowID)
	}

	if other != nil {
		otherUnlocked, err := IsUnlocked(data, 0, other)
		if err != nil {
			t.Fatalf("IsUnlocked(other): %v", err)
		}
		if otherUnlocked {
			t.Errorf("unrelated row %d became unlocked as a side effect", other.RowID)
		}
	}
}
