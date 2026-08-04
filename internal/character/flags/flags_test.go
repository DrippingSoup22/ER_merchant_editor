package flags

import "testing"

// Ground truth captured from EldenRing-SaveForge's own GetEventFlag/
// SetEventFlag (github.com/oisis/EldenRing-SaveForge, GPLv3) via a
// throwaway, uncommitted oracle program calling those functions directly
// (never copying code) — see docs/SAVEFORGE_REFERENCE.md. IDs are real
// ShopLineupParam eventFlag_forRelease values pulled from
// save_files/vanilla_fresh_character.dat (sampled across the full range,
// plus 60150 and 9440 which fall through both this package's exception
// table and the naive fallback formula, and 100/300 which are in the
// exception table).
func TestResolvePositionMatchesOracle(t *testing.T) {
	cases := []struct {
		id   uint32
		byte uint32
		bit  uint8
	}{
		{9101, 0x471, 2},
		{9155, 0x478, 4},
		{60150, 0x4F4, 1},
		{65821, 0x7B9, 2},
		{65841, 0x7BC, 6},
		{65861, 0x7BE, 2},
		{65881, 0x7C1, 6},
		{65901, 0x7C3, 2},
		{65929, 0x7C7, 6},
		{11109759, 0x153A73, 0},
		{11109796, 0x153A78, 3},
		{1034509450, 0x5F425, 5},
		{1040529255, 0x92F32, 0},
		{1050560800, 0xE9165, 7},
		{1254560800, 0x1AA7A2, 7},
		{2045429330, 0x11B5D, 5},
		{2045429331, 0x11B5D, 4},
		{2051459206, 0x2F2FB, 1},
		{100, 0xC, 3},
		{300, 0x25, 3},
		{9440, 0x49C, 7},
	}
	for _, c := range cases {
		byteIdx, bitIdx := resolvePosition(c.id)
		if byteIdx != c.byte || bitIdx != c.bit {
			t.Errorf("resolvePosition(%d) = (0x%X, %d), want (0x%X, %d)", c.id, byteIdx, bitIdx, c.byte, c.bit)
		}
		if byteIdx >= FlagsByteCount {
			t.Errorf("resolvePosition(%d) byte 0x%X exceeds FlagsByteCount 0x%X", c.id, byteIdx, FlagsByteCount)
		}
	}
}

func TestGetSetRoundTrip(t *testing.T) {
	flags := make([]byte, FlagsByteCount)
	ids := []uint32{100, 300, 60150, 9101, 65929, 1254560800}
	for _, id := range ids {
		set, err := Get(flags, id)
		if err != nil {
			t.Fatalf("Get(%d) before Set: %v", id, err)
		}
		if set {
			t.Fatalf("flag %d unexpectedly set before any write", id)
		}
	}
	for _, id := range ids {
		if err := Set(flags, id, true); err != nil {
			t.Fatalf("Set(%d, true): %v", id, err)
		}
	}
	for _, id := range ids {
		set, err := Get(flags, id)
		if err != nil {
			t.Fatalf("Get(%d) after Set true: %v", id, err)
		}
		if !set {
			t.Fatalf("flag %d not set after Set(true)", id)
		}
	}
	// Clearing one flag must not disturb the others sharing no byte with it.
	if err := Set(flags, 100, false); err != nil {
		t.Fatalf("Set(100, false): %v", err)
	}
	if set, _ := Get(flags, 100); set {
		t.Fatal("flag 100 still set after Set(false)")
	}
	if set, _ := Get(flags, 300); !set {
		t.Fatal("unrelated flag 300 was cleared by Set(100, false)")
	}
}

func TestOutOfBounds(t *testing.T) {
	flags := make([]byte, 4) // deliberately too short
	if _, err := Get(flags, 100); err == nil {
		t.Fatal("expected out-of-bounds error from Get")
	}
	if err := Set(flags, 100, true); err == nil {
		t.Fatal("expected out-of-bounds error from Set")
	}
}
