package character

import (
	"testing"

	charflags "github.com/DrippingSoup22/ER_merchant_editor/internal/character/flags"
)

// TestBellBearingFlagsResolveWithoutCollision promotes an ad hoc
// verification script (run once against the real resolver while
// designing this feature) into a real test: every BellBearings entry
// must resolve to an in-bounds byte position, and no two distinct flag
// IDs may resolve to the same byte+bit (which would mean toggling one
// bell bearing silently toggles another).
func TestBellBearingFlagsResolveWithoutCollision(t *testing.T) {
	seen := make(map[[2]uint32]uint32, len(BellBearings))
	for _, b := range BellBearings {
		byteIdx, bitIdx := charflags.Position(b.FlagID)
		if byteIdx >= charflags.FlagsByteCount {
			t.Errorf("%s (flag %d): byte 0x%X exceeds FlagsByteCount 0x%X", b.Name, b.FlagID, byteIdx, charflags.FlagsByteCount)
		}
		key := [2]uint32{byteIdx, uint32(bitIdx)}
		if prior, ok := seen[key]; ok {
			t.Errorf("collision: flag %d (%s) and flag %d both resolve to byte 0x%X bit %d", prior, b.Name, b.FlagID, byteIdx, bitIdx)
		}
		seen[key] = b.FlagID
	}
}

// TestBellBearingNoDuplicateFlagIDs is a cheap correctness net independent
// of the collision test above (which only checks resolved position
// collisions) -- catches an accidental copy-paste typo in the source
// table itself.
func TestBellBearingNoDuplicateFlagIDs(t *testing.T) {
	seen := make(map[uint32]string, len(BellBearings))
	for _, b := range BellBearings {
		if prior, ok := seen[b.FlagID]; ok {
			t.Errorf("duplicate FlagID %d: %q and %q", b.FlagID, prior, b.Name)
		}
		seen[b.FlagID] = b.Name
	}
}

// TestBellBearingsForUIExcludesCovered asserts the UI filter is exactly
// the Covered==false subset -- no more, no less.
func TestBellBearingsForUIExcludesCovered(t *testing.T) {
	wantCovered := 0
	for _, b := range BellBearings {
		if b.Covered {
			wantCovered++
		}
	}
	ui := BellBearingsForUI()
	if got, want := len(ui), len(BellBearings)-wantCovered; got != want {
		t.Fatalf("BellBearingsForUI() returned %d entries, want %d", got, want)
	}
	for _, b := range ui {
		if b.Covered {
			t.Errorf("BellBearingsForUI() unexpectedly returned a Covered==true entry: %s", b.Name)
		}
	}
}

// TestThiollierHasNoSeparateTMHBearing documents the actual DLC data:
// Thiollier has no Twin Maiden Husks bearing/flag, so the Characters view
// must not invent a checkbox. His progressed wares are carried by Moore's
// Shop 5 bearing rather than a separate flag.
func TestThiollierHasNoSeparateTMHBearing(t *testing.T) {
	mooreFound := false
	for _, b := range BellBearingsForUI() {
		if b.Name == "Thiollier's Bell Bearing" || b.Merchant == "Thiollier" {
			t.Fatalf("unexpected separate Thiollier TMH bearing: %+v", b)
		}
		if b.Name == "Moore's Bell Bearing" && b.Merchant == "Moore" {
			mooreFound = true
		}
	}
	if !mooreFound {
		t.Fatal("Moore's Bell Bearing is missing from the TMH UI table")
	}
}
