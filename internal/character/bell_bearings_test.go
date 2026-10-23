package character

import (
	"testing"

	"github.com/DrippingSoup22/ER_shared/charslot"
)

// Every bell bearing resolves to its own bit: two entries sharing one would
// toggle together, and a duplicated flag ID collides with itself.
func TestBellBearingFlagsResolveToDistinctBits(t *testing.T) {
	seen := make(map[[2]int]string, len(BellBearings))
	for _, b := range BellBearings {
		byteIndex, bit, err := charslot.FlagPosition(b.FlagID)
		if err != nil {
			t.Errorf("%s: %v", b.Name, err)
			continue
		}
		key := [2]int{byteIndex, int(bit)}
		if prior, ok := seen[key]; ok {
			t.Errorf("%s and %s share byte 0x%X bit %d", prior, b.Name, byteIndex, bit)
		}
		seen[key] = b.Name
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
