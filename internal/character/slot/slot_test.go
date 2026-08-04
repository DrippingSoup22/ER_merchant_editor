package slot

import (
	"os"
	"path/filepath"
	"testing"
)

// Ground truth captured from EldenRing-SaveForge's own slot parser
// (github.com/oisis/EldenRing-SaveForge, GPLv3) via a throwaway,
// uncommitted oracle program (core.LoadSave) — see
// docs/SAVEFORGE_REFERENCE.md. Offsets are relative to the start of each
// slot (i.e. already past HeaderSize).
type wantSlot struct {
	version          uint32
	eventFlagsOffset int
	name             string
	level            uint32
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "save_files", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture %s not present: %v", name, err)
	}
	return data
}

func TestSlotsMatchOracle(t *testing.T) {
	cases := []struct {
		fixture string
		slots   map[int]wantSlot
	}{
		{
			fixture: "vanilla_fresh_character.dat",
			slots: map[int]wantSlot{
				0: {230, 0x39E25, "DrippingSoup", 146},
				1: {251, 0x3996F, "DrippingSoup", 150},
				2: {210, 0x395F9, "DrippingSoup", 165},
				3: {220, 0x391EA, "DrippingSoup", 167},
				4: {251, 0x3A492, "DrippingSoup", 150},
				5: {251, 0x37FF7, "DrippingSoup_F", 121},
				6: {251, 0x38181, "Granny", 137},
				7: {251, 0x36C35, "DrippingSoup", 9},
			},
		},
		{
			fixture: "BetterPSN.dat",
			slots: map[int]wantSlot{
				0: {230, 0x39E25, "DrippingSoup", 146},
				1: {251, 0x3996F, "DrippingSoup", 150},
				2: {210, 0x395F9, "DrippingSoup", 165},
				3: {220, 0x391EA, "DrippingSoup", 167},
				4: {251, 0x3A492, "DrippingSoup", 150},
				5: {251, 0x37FF7, "DrippingSoup_F", 121},
				6: {251, 0x385E1, "Granny", 137},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			data := loadFixture(t, tc.fixture)
			slots := Slots(data)
			if len(slots) != NumSlots {
				t.Fatalf("Slots() returned %d slots, want %d", len(slots), NumSlots)
			}
			seenReal := 0
			for i, slot := range slots {
				want, wantReal := tc.slots[i]
				if !wantReal {
					if !IsEmpty(slot) {
						t.Errorf("slot %d: expected empty, got version=%d", i, Version(slot))
					}
					continue
				}
				seenReal++
				if got := Version(slot); got != want.version {
					t.Errorf("slot %d: Version=%d, want %d", i, got, want.version)
				}
				if IsEmpty(slot) {
					t.Errorf("slot %d: IsEmpty=true for a real character", i)
				}
				offset, err := EventFlagsOffset(slot)
				if err != nil {
					t.Errorf("slot %d: EventFlagsOffset: %v", i, err)
				} else if offset != want.eventFlagsOffset {
					t.Errorf("slot %d: EventFlagsOffset=0x%X, want 0x%X", i, offset, want.eventFlagsOffset)
				}
				id, err := ReadIdentity(slot)
				if err != nil {
					t.Errorf("slot %d: ReadIdentity: %v", i, err)
					continue
				}
				if id.Name != want.name {
					t.Errorf("slot %d: Name=%q, want %q", i, id.Name, want.name)
				}
				if id.Level != want.level {
					t.Errorf("slot %d: Level=%d, want %d", i, id.Level, want.level)
				}
			}
			if seenReal != len(tc.slots) {
				t.Errorf("%s: checked %d real slots, want %d", tc.fixture, seenReal, len(tc.slots))
			}
		})
	}
}
