package charunlock

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"er_merchant_editor/app/catalog"
	"er_merchant_editor/app/charflags"
	"er_merchant_editor/app/charslot"
)

func TestUnlockClearsLockedRowsOnly(t *testing.T) {
	data := append([]byte(nil), loadFixtureBytes(t)...)
	rows := loadRows(t)

	const charIndex = 7 // level-9 slot: most locked rows to work with
	before, err := LockedRows(data, charIndex, rows)
	if err != nil {
		t.Fatalf("LockedRows before: %v", err)
	}
	if len(before) < 3 {
		t.Fatalf("need at least 3 locked rows to test, got %d", len(before))
	}
	target := before[:2]

	origCopy := append([]byte(nil), data...)

	n, err := Unlock(data, charIndex, target)
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if n != 2 {
		t.Fatalf("Unlock reported %d changes, want 2", n)
	}

	for _, r := range target {
		unlocked, err := IsUnlocked(data, charIndex, r)
		if err != nil {
			t.Fatalf("IsUnlocked(%d): %v", r.RowID, err)
		}
		if !unlocked {
			t.Errorf("row %d still locked after Unlock", r.RowID)
		}
	}

	// Every other previously-locked row must be unaffected.
	stillLocked, err := LockedRows(data, charIndex, rows)
	if err != nil {
		t.Fatalf("LockedRows after: %v", err)
	}
	stillLockedSet := make(map[int64]bool, len(stillLocked))
	for _, r := range stillLocked {
		stillLockedSet[r.RowID] = true
	}
	targetSet := map[int64]bool{target[0].RowID: true, target[1].RowID: true}
	for _, r := range before {
		if targetSet[r.RowID] {
			if stillLockedSet[r.RowID] {
				t.Errorf("row %d should have been unlocked", r.RowID)
			}
			continue
		}
		if !stillLockedSet[r.RowID] {
			t.Errorf("row %d unexpectedly unlocked as a side effect", r.RowID)
		}
	}
	if len(stillLocked) != len(before)-2 {
		t.Errorf("locked count = %d, want %d", len(stillLocked), len(before)-2)
	}

	// Bytes outside the flags region, and outside the two touched flag
	// bytes within it, must be byte-identical to the original file.
	origSlots := charslot.Slots(origCopy)
	off, err := charslot.EventFlagsOffset(origSlots[charIndex])
	if err != nil {
		t.Fatalf("EventFlagsOffset: %v", err)
	}
	slotStart := charslot.HeaderSize + charIndex*charslot.SlotSize
	diffOutsideFlags := 0
	for i := range data {
		if data[i] == origCopy[i] {
			continue
		}
		rel := i - slotStart - off
		if rel < 0 || rel >= charflags.FlagsByteCount {
			diffOutsideFlags++
		}
	}
	if diffOutsideFlags != 0 {
		t.Errorf("%d bytes changed outside the target character's event-flags region", diffOutsideFlags)
	}
}

func TestUnlockNoOpWhenAlreadyUnlocked(t *testing.T) {
	data := append([]byte(nil), loadFixtureBytes(t)...)
	rows := loadRows(t)

	const charIndex = 2 // one of the higher-progress slots
	var unlockedRow *catalog.Row
	for _, r := range rows {
		if r.UnlockFlag == 0 {
			continue
		}
		u, err := IsUnlocked(data, charIndex, r)
		if err != nil {
			t.Fatalf("IsUnlocked: %v", err)
		}
		if u {
			unlockedRow = r
			break
		}
	}
	if unlockedRow == nil {
		t.Skip("no already-unlocked gated row found for this slot")
	}

	before := append([]byte(nil), data...)
	n, err := Unlock(data, charIndex, []*catalog.Row{unlockedRow})
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if n != 0 {
		t.Errorf("Unlock reported %d changes for an already-unlocked row, want 0", n)
	}
	if !bytes.Equal(before, data) {
		t.Error("Unlock modified saveData even though nothing needed changing")
	}
}

// TestSetReleaseLocksAndUnlocks round-trips a row through both
// directions: unlock it, confirm, re-lock it, confirm -- the new
// SaveForge-style toggle the GUI's checkboxes rely on. Reuses the same
// byte-diff isolation check as TestUnlockClearsLockedRowsOnly.
func TestSetReleaseLocksAndUnlocks(t *testing.T) {
	data := append([]byte(nil), loadFixtureBytes(t)...)
	rows := loadRows(t)

	const charIndex = 7
	locked, err := LockedRows(data, charIndex, rows)
	if err != nil {
		t.Fatalf("LockedRows: %v", err)
	}
	if len(locked) == 0 {
		t.Fatal("need at least 1 locked row to test")
	}
	target := []*catalog.Row{locked[0]}

	origSlots := charslot.Slots(append([]byte(nil), data...))
	off, err := charslot.EventFlagsOffset(origSlots[charIndex])
	if err != nil {
		t.Fatalf("EventFlagsOffset: %v", err)
	}
	slotStart := charslot.HeaderSize + charIndex*charslot.SlotSize
	assertOnlyFlagsByteChanged := func(before []byte) {
		t.Helper()
		diff := 0
		for i := range data {
			if data[i] == before[i] {
				continue
			}
			rel := i - slotStart - off
			if rel < 0 || rel >= charflags.FlagsByteCount {
				diff++
			}
		}
		if diff != 0 {
			t.Errorf("%d bytes changed outside the event-flags region", diff)
		}
	}

	// Unlock.
	before := append([]byte(nil), data...)
	n, err := SetRelease(data, charIndex, target, true)
	if err != nil {
		t.Fatalf("SetRelease(true): %v", err)
	}
	if n != 1 {
		t.Fatalf("SetRelease(true) reported %d changes, want 1", n)
	}
	assertOnlyFlagsByteChanged(before)
	if unlocked, err := IsUnlocked(data, charIndex, target[0]); err != nil || !unlocked {
		t.Fatalf("IsUnlocked after SetRelease(true) = %v, %v, want true, nil", unlocked, err)
	}

	// Re-lock.
	before = append([]byte(nil), data...)
	n, err = SetRelease(data, charIndex, target, false)
	if err != nil {
		t.Fatalf("SetRelease(false): %v", err)
	}
	if n != 1 {
		t.Fatalf("SetRelease(false) reported %d changes, want 1", n)
	}
	assertOnlyFlagsByteChanged(before)
	if unlocked, err := IsUnlocked(data, charIndex, target[0]); err != nil || unlocked {
		t.Fatalf("IsUnlocked after SetRelease(false) = %v, %v, want false, nil", unlocked, err)
	}

	// A repeated SetRelease(false) is a no-op (already at target).
	before = append([]byte(nil), data...)
	n, err = SetRelease(data, charIndex, target, false)
	if err != nil {
		t.Fatalf("SetRelease(false) repeat: %v", err)
	}
	if n != 0 {
		t.Errorf("SetRelease(false) repeat reported %d changes, want 0", n)
	}
	if !bytes.Equal(before, data) {
		t.Error("no-op SetRelease modified saveData")
	}
}

// TestLockStatesMatchesLockedRows cross-checks LockStates (all gated rows)
// against LockedRows (locked subset only): the rows LockStates marks
// false must be exactly the rows LockedRows returns, and every row
// LockStates marks true must round-trip through IsUnlocked as true too.
func TestLockStatesMatchesLockedRows(t *testing.T) {
	data := loadFixtureBytes(t)
	rows := loadRows(t)

	const charIndex = 7
	locked, err := LockedRows(data, charIndex, rows)
	if err != nil {
		t.Fatalf("LockedRows: %v", err)
	}
	lockedSet := make(map[int64]bool, len(locked))
	for _, r := range locked {
		lockedSet[r.RowID] = true
	}

	states, err := LockStates(data, charIndex, rows)
	if err != nil {
		t.Fatalf("LockStates: %v", err)
	}

	gatedCount := 0
	for _, r := range rows {
		if r.UnlockFlag != 0 {
			gatedCount++
		}
	}
	if len(states) == 0 {
		t.Fatal("LockStates returned nothing for a fixture with gated rows")
	}

	for _, r := range rows {
		if r.UnlockFlag == 0 {
			if _, ok := states[r.RowID]; ok {
				t.Errorf("row %d has no gate but appears in LockStates", r.RowID)
			}
			continue
		}
		unlocked, ok := states[r.RowID]
		if !ok {
			continue // unresolvable flag, consistent with LockedRows skipping it too
		}
		wantLocked := lockedSet[r.RowID]
		if unlocked == wantLocked {
			t.Errorf("row %d: LockStates unlocked=%v, LockedRows says locked=%v (contradiction)", r.RowID, unlocked, wantLocked)
		}
		u, err := IsUnlocked(data, charIndex, r)
		if err != nil {
			t.Fatalf("IsUnlocked(%d): %v", r.RowID, err)
		}
		if u != unlocked {
			t.Errorf("row %d: LockStates=%v, IsUnlocked=%v", r.RowID, unlocked, u)
		}
	}
}

// TestSetReleaseBatchMixedDirections stages one row to unlock and another
// to re-lock for the same character in a single call -- the GUI's staged
// checkbox-commit case, which plain SetRelease (one direction for the
// whole batch) can't express.
func TestSetReleaseBatchMixedDirections(t *testing.T) {
	data := append([]byte(nil), loadFixtureBytes(t)...)
	rows := loadRows(t)

	const charIndex = 7
	locked, err := LockedRows(data, charIndex, rows)
	if err != nil {
		t.Fatalf("LockedRows: %v", err)
	}
	var unlockedRow *catalog.Row
	for _, r := range rows {
		if r.UnlockFlag == 0 {
			continue
		}
		u, err := IsUnlocked(data, charIndex, r)
		if err != nil {
			t.Fatalf("IsUnlocked: %v", err)
		}
		if u {
			unlockedRow = r
			break
		}
	}
	if len(locked) == 0 || unlockedRow == nil {
		t.Fatal("need at least 1 locked and 1 unlocked gated row to test mixed directions")
	}
	toUnlock := locked[0]

	n, err := SetReleaseBatch(data, charIndex, []FlagTarget{
		{FlagID: toUnlock.UnlockFlag, Released: true},
		{FlagID: unlockedRow.UnlockFlag, Released: false},
	})
	if err != nil {
		t.Fatalf("SetReleaseBatch: %v", err)
	}
	if n != 2 {
		t.Fatalf("SetReleaseBatch reported %d changes, want 2", n)
	}

	if u, err := IsUnlocked(data, charIndex, toUnlock); err != nil || !u {
		t.Errorf("toUnlock: IsUnlocked = %v, %v, want true, nil", u, err)
	}
	if u, err := IsUnlocked(data, charIndex, unlockedRow); err != nil || u {
		t.Errorf("unlockedRow: IsUnlocked = %v, %v, want false, nil", u, err)
	}
}

// TestApplyBatchToFileMultipleCharacters writes edits for two different
// characters in one call/one file -- the GUI's "Save All" case. Operates
// entirely under t.TempDir(), never touching save_files/ or
// working_copies/.
func TestApplyBatchToFileMultipleCharacters(t *testing.T) {
	data := loadFixtureBytes(t)
	rows := loadRows(t)

	const charA, charB = 7, 2
	var rowA, rowB *catalog.Row
	for _, r := range rows {
		if r.UnlockFlag == 0 {
			continue
		}
		if rowA == nil {
			if u, _ := IsUnlocked(data, charA, r); !u {
				rowA = r
			}
		}
		if rowB == nil {
			if u, _ := IsUnlocked(data, charB, r); !u {
				rowB = r
			}
		}
		if rowA != nil && rowB != nil {
			break
		}
	}
	if rowA == nil || rowB == nil {
		t.Fatal("need a locked gated row for both test characters")
	}

	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.dat")
	outPath := filepath.Join(dir, "out.dat")
	if err := os.WriteFile(inPath, data, 0o644); err != nil {
		t.Fatalf("write temp input: %v", err)
	}

	n, err := ApplyBatchToFile(inPath, outPath, map[int][]FlagTarget{
		charA: {{FlagID: rowA.UnlockFlag, Released: true}},
		charB: {{FlagID: rowB.UnlockFlag, Released: true}},
	})
	if err != nil {
		t.Fatalf("ApplyBatchToFile: %v", err)
	}
	if n != 2 {
		t.Fatalf("ApplyBatchToFile reported %d changes, want 2", n)
	}

	outData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if u, err := IsUnlocked(outData, charA, rowA); err != nil || !u {
		t.Errorf("charA rowA: IsUnlocked = %v, %v, want true, nil", u, err)
	}
	if u, err := IsUnlocked(outData, charB, rowB); err != nil || !u {
		t.Errorf("charB rowB: IsUnlocked = %v, %v, want true, nil", u, err)
	}
}

// TestSetBellBearingsBatchRoundTrips exercises the bell-bearing write
// path (no backing catalog.Row at all) through the same round-trip
// discipline as TestSetReleaseLocksAndUnlocks: unlock, confirm, re-lock,
// confirm, no-op repeat leaves bytes untouched, byte-diff isolated to the
// touched flags' bytes.
func TestSetBellBearingsBatchRoundTrips(t *testing.T) {
	data := append([]byte(nil), loadFixtureBytes(t)...)
	if len(BellBearings) < 1 {
		t.Fatal("BellBearings table is empty")
	}
	flagID := BellBearings[0].FlagID

	const charIndex = 7
	origSlots := charslot.Slots(append([]byte(nil), data...))
	off, err := charslot.EventFlagsOffset(origSlots[charIndex])
	if err != nil {
		t.Fatalf("EventFlagsOffset: %v", err)
	}
	slotStart := charslot.HeaderSize + charIndex*charslot.SlotSize
	assertOnlyFlagsByteChanged := func(before []byte) {
		t.Helper()
		diff := 0
		for i := range data {
			if data[i] == before[i] {
				continue
			}
			rel := i - slotStart - off
			if rel < 0 || rel >= charflags.FlagsByteCount {
				diff++
			}
		}
		if diff != 0 {
			t.Errorf("%d bytes changed outside the event-flags region", diff)
		}
	}

	// Start from a known state: force it locked first via SetBellBearingsBatch
	// itself, ignoring the result, so the test doesn't depend on the
	// fixture's pre-existing value for this flag.
	if _, err := SetBellBearingsBatch(data, charIndex, map[uint32]bool{flagID: false}); err != nil {
		t.Fatalf("SetBellBearingsBatch(initial false): %v", err)
	}

	// Unlock.
	before := append([]byte(nil), data...)
	n, err := SetBellBearingsBatch(data, charIndex, map[uint32]bool{flagID: true})
	if err != nil {
		t.Fatalf("SetBellBearingsBatch(true): %v", err)
	}
	if n != 1 {
		t.Fatalf("SetBellBearingsBatch(true) reported %d changes, want 1", n)
	}
	assertOnlyFlagsByteChanged(before)
	states, err := FlagStates(data, charIndex, []uint32{flagID})
	if err != nil || !states[flagID] {
		t.Fatalf("FlagStates after SetBellBearingsBatch(true) = %v, %v, want true, nil", states[flagID], err)
	}

	// No-op repeat.
	before = append([]byte(nil), data...)
	n, err = SetBellBearingsBatch(data, charIndex, map[uint32]bool{flagID: true})
	if err != nil {
		t.Fatalf("SetBellBearingsBatch(true, repeat): %v", err)
	}
	if n != 0 {
		t.Errorf("SetBellBearingsBatch(true, repeat) reported %d changes, want 0", n)
	}
	if !bytes.Equal(before, data) {
		t.Error("no-op SetBellBearingsBatch call modified saveData")
	}

	// Re-lock.
	before = append([]byte(nil), data...)
	n, err = SetBellBearingsBatch(data, charIndex, map[uint32]bool{flagID: false})
	if err != nil {
		t.Fatalf("SetBellBearingsBatch(false): %v", err)
	}
	if n != 1 {
		t.Fatalf("SetBellBearingsBatch(false) reported %d changes, want 1", n)
	}
	assertOnlyFlagsByteChanged(before)
	states, err = FlagStates(data, charIndex, []uint32{flagID})
	if err != nil || states[flagID] {
		t.Fatalf("FlagStates after SetBellBearingsBatch(false) = %v, %v, want false, nil", states[flagID], err)
	}
}

// TestSetReleaseBatchMixesRowsAndBellBearingFlagsInOneCall is the direct
// regression test for the FlagTarget generalization actually working end
// to end (not just compiling): one row-backed target and one bare
// bell-bearing target in the same SetReleaseBatch call, confirming both
// land correctly and independently -- the exact scenario
// startCombinedSave now produces when both PendingFlagEdits and
// PendingBellBearingEdits are staged for one character.
func TestSetReleaseBatchMixesRowsAndBellBearingFlagsInOneCall(t *testing.T) {
	data := append([]byte(nil), loadFixtureBytes(t)...)
	rows := loadRows(t)
	if len(BellBearings) < 1 {
		t.Fatal("BellBearings table is empty")
	}
	bbFlagID := BellBearings[0].FlagID

	const charIndex = 7
	locked, err := LockedRows(data, charIndex, rows)
	if err != nil {
		t.Fatalf("LockedRows: %v", err)
	}
	if len(locked) == 0 {
		t.Fatal("need at least 1 locked row to test")
	}
	row := locked[0]

	// Start the bell bearing flag from a known-locked state.
	if _, err := SetBellBearingsBatch(data, charIndex, map[uint32]bool{bbFlagID: false}); err != nil {
		t.Fatalf("SetBellBearingsBatch(initial false): %v", err)
	}

	n, err := SetReleaseBatch(data, charIndex, []FlagTarget{
		{FlagID: row.UnlockFlag, Released: true, Label: fmt.Sprintf("row %d", row.RowID)},
		{FlagID: int64(bbFlagID), Released: true, Label: "bell bearing"},
	})
	if err != nil {
		t.Fatalf("SetReleaseBatch: %v", err)
	}
	if n != 2 {
		t.Fatalf("SetReleaseBatch reported %d changes, want 2", n)
	}

	if u, err := IsUnlocked(data, charIndex, row); err != nil || !u {
		t.Errorf("row: IsUnlocked = %v, %v, want true, nil", u, err)
	}
	if states, err := FlagStates(data, charIndex, []uint32{bbFlagID}); err != nil || !states[bbFlagID] {
		t.Errorf("bell bearing flag: FlagStates = %v, %v, want true, nil", states[bbFlagID], err)
	}
}

// --- SetReleaseBatch's two round-trip self-checks (write.go) ---
//
// SetReleaseBatch flips the requested flag bits in a working copy, then runs
// two safety checks before touching saveData:
//
//	(a) "unexpected byte change ... outside the N intended writes" — no byte
//	    OTHER than the recorded touched-flag bytes may differ from the original.
//	(b) "round-trip check failed" — every touched flag must read back at its
//	    own intended value.
//
// Only branch (b) is reachable through the public API; see the note above
// TestSetReleaseBatchSelfCheckBranchAUnreachable below for why branch (a)
// isn't, and what it would take (a production change) to force it.

// TestSetReleaseBatchAliasedTargetsTripRoundTripCheck forces self-check (b) to
// fire and asserts both required guarantees: an error is returned, and the
// function's real mutation target (saveData) is left byte-for-byte untouched.
//
// Trigger: two targets naming the SAME flag with opposing Released values. The
// batch sets the bit true (first target), then clears it back to false (second
// target) in the working copy; the first target's post-write read-back (false)
// then disagrees with its intended value (true) — exactly the corrupted /
// aliased-resolver condition self-check (b) exists to catch. (This stands in
// for two DISTINCT flag IDs resolving onto one bit, which the real charflags
// resolver never produces — see the branch-(a) note below.)
func TestSetReleaseBatchAliasedTargetsTripRoundTripCheck(t *testing.T) {
	data := append([]byte(nil), loadFixtureBytes(t)...)
	rows := loadRows(t)

	const charIndex = 7
	locked, err := LockedRows(data, charIndex, rows)
	if err != nil {
		t.Fatalf("LockedRows: %v", err)
	}
	if len(locked) == 0 {
		t.Fatal("need at least 1 locked (released=false) gated row to test")
	}
	flag := locked[0].UnlockFlag // resolvable, and currently released=false

	before := append([]byte(nil), data...)
	n, err := SetReleaseBatch(data, charIndex, []FlagTarget{
		{FlagID: flag, Released: true, Label: "alias-set"},
		{FlagID: flag, Released: false, Label: "alias-clear"},
	})
	if err == nil {
		t.Fatal("SetReleaseBatch accepted contradictory aliased targets; self-check (b) did not fire")
	}
	if !strings.Contains(err.Error(), "round-trip check failed") {
		t.Errorf("error %q is not the expected round-trip self-check (b) failure", err)
	}
	if n != 0 {
		t.Errorf("SetReleaseBatch returned n=%d on failure, want 0", n)
	}
	if !bytes.Equal(before, data) {
		t.Error("SetReleaseBatch modified saveData despite self-check (b) failing")
	}
}

// TestSetReleaseBatchSelfCheckBranchAUnreachable documents — and guards the
// invariant behind — the fact that self-check (a) cannot be forced through the
// public API without a production change.
//
// Branch (a) fires only if charflags.Set mutates a byte that
// charflags.Position did NOT report — i.e. the two disagree. Both resolve a
// flag ID through the identical resolvePosition, and SetReleaseBatch records
// Position(flagID) for every flag it Sets, so the working copy can only ever
// differ from the original at recorded ("touched") bytes; no saveData/targets
// input can make an *un*touched byte differ. Forcing branch (a) would require
// injecting a broken resolver (an interface/hook that does not exist) — a
// production change, deliberately not made (this phase is test-only). Reported
// to the caller rather than worked around.
//
// This test pins the exact invariant that keeps branch (a) dormant: for real
// flag IDs, Set touches precisely the one byte Position names, and no other.
// If a future refactor breaks that agreement, branch (a) becomes reachable and
// this test fails first.
func TestSetReleaseBatchSelfCheckBranchAUnreachable(t *testing.T) {
	rows := loadRows(t)
	var ids []uint32
	for _, r := range rows {
		if r.UnlockFlag == 0 {
			continue
		}
		ids = append(ids, uint32(r.UnlockFlag))
		if len(ids) >= 25 {
			break
		}
	}
	if len(ids) == 0 {
		t.Skip("no gated flag IDs available in fixture")
	}
	for _, id := range ids {
		buf := make([]byte, charflags.FlagsByteCount)
		byteIdx, _ := charflags.Position(id)
		if err := charflags.Set(buf, id, true); err != nil {
			t.Fatalf("Set(%d): %v", id, err)
		}
		for i, b := range buf {
			if b != 0 && uint32(i) != byteIdx {
				t.Errorf("flag %d: Set touched byte 0x%X but Position reported 0x%X", id, i, byteIdx)
			}
		}
		if buf[byteIdx] == 0 {
			t.Errorf("flag %d: Set did not modify the byte Position reported (0x%X)", id, byteIdx)
		}
	}
}
