package gio

// Pure state-machine tests for the Characters view (no window/frame loop,
// no native dialogs -- startSetRelease's zenity call is exercised manually
// via cmd/charunlock, see docs/CHAR_UNLOCK.md).

import (
	"fmt"
	"image"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/character"
)

// headlessGtx builds a usable layout.Context with no real window (Context's
// zero value already "never returns events, maps units to pixels with a
// scale of 1.0" per its doc comment) -- enough to drive layout code and
// catch a nil-deref/index panic without any GUI automation tooling, which
// this dev environment doesn't have.
func headlessGtx() layout.Context {
	return layout.Context{
		Ops:         &op.Ops{},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1200, 800)},
	}
}

func loadedTestCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	path := filepath.Join("..", "..", "save_files", "vanilla_fresh_character.dat")
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.LoadSave(path); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	return cat
}

func TestEnsureCharListPopulatesAndResetsSelection(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.SelectedChar = 3 // pretend a previous save had this selected
	s.UnlockMerchant = "Patches"

	s.ensureCharList()

	if len(s.CharList) != 8 {
		t.Fatalf("CharList len = %d, want 8", len(s.CharList))
	}
	if s.SelectedChar != -1 {
		t.Errorf("SelectedChar = %d after a fresh load, want -1 (reset)", s.SelectedChar)
	}
	if s.UnlockMerchant != "" {
		t.Errorf("UnlockMerchant = %q after a fresh load, want \"\" (reset)", s.UnlockMerchant)
	}

	// A second call with the same save path must be a no-op (no re-read).
	s.selectCharacter(7)
	s.ensureCharList()
	if s.SelectedChar != 7 {
		t.Errorf("ensureCharList clobbered SelectedChar on an unchanged path: got %d, want 7", s.SelectedChar)
	}
}

func TestSelectCharacterToggles(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()

	s.selectCharacter(7)
	if s.SelectedChar != 7 {
		t.Fatalf("SelectedChar = %d, want 7", s.SelectedChar)
	}
	s.UnlockMerchant = "Patches" // simulate a merchant picked under char 7

	s.selectCharacter(2) // switching characters must drop the merchant sub-selection
	if s.SelectedChar != 2 {
		t.Fatalf("SelectedChar = %d, want 2", s.SelectedChar)
	}
	if s.UnlockMerchant != "" {
		t.Errorf("UnlockMerchant = %q after switching characters, want \"\"", s.UnlockMerchant)
	}

	s.selectCharacter(2) // re-clicking the same character deselects it
	if s.SelectedChar != -1 {
		t.Errorf("SelectedChar = %d after re-clicking, want -1 (toggle off)", s.SelectedChar)
	}
}

func TestCharacterWideUnlockDoesNotRequireMerchantSelection(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()

	if s.UnlockMerchant != "" {
		t.Fatalf("test requires no merchant selection, got %q", s.UnlockMerchant)
	}
	lockedEverywhere := s.allMerchantsLockedCount()
	if lockedEverywhere == 0 {
		t.Fatal("fixture has no remaining character-wide unlocks")
	}
	merchantEnabled, allEnabled := s.bulkUnlockAvailability(s.flagsColumnLockedCount(), lockedEverywhere)
	if merchantEnabled {
		t.Error("merchant-scoped unlock enabled without a merchant selection")
	}
	if !allEnabled {
		t.Error("character-wide unlock disabled despite a selected character and remaining flags")
	}
}

func TestEnsureMerchantGatedMatchesCharunlock(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()
	s.selectCharacter(7) // level-9 slot: known to have plenty of locked stock

	s.ensureMerchantGated()
	if len(s.merchantGatedTotal) == 0 {
		t.Fatal("expected at least one merchant with gated stock for the level-9 character")
	}

	// Cross-check EVERY entry directly against character. Total/unlocked
	// are counted per groupFlagRows GROUP (matching the checkbox count
	// layoutFlagsColumn renders), not per raw gated row. Checking all of them
	// (rather than one random map entry) keeps this deterministic -- an
	// earlier single-entry `break` flaked whenever map iteration happened to
	// land on Twin Maiden Husks, whose bell bearings the naive expectation
	// below omits.
	for name, total := range s.merchantGatedTotal {
		rows, err := s.Catalog.MerchantRows(name)
		if err != nil {
			t.Fatalf("MerchantRows(%q): %v", name, err)
		}
		states, err := character.LockStates(s.charSaveData, s.SelectedChar, rows)
		if err != nil {
			t.Fatalf("LockStates(%q): %v", name, err)
		}
		gatedRows := make([]*catalog.Row, 0, len(states))
		for _, r := range rows {
			if _, ok := states[r.RowID]; ok {
				gatedRows = append(gatedRows, r)
			}
		}
		groups := groupFlagRows(gatedRows)
		wantTotal, wantUnlocked := len(groups), 0
		for _, g := range groups {
			if states[g.Rows[0].RowID] {
				wantUnlocked++
			}
		}
		// Twin Maiden Husks folds her NPC bell bearings into the same
		// merchant count (ensureMerchantGated) -- they are not shop rows and
		// never appear in groupFlagRows, so add them to the expectation here.
		if name == twinMaidenHusksMerchantName {
			bb := character.BellBearingsForUI()
			ids := make([]uint32, len(bb))
			for i, b := range bb {
				ids[i] = b.FlagID
			}
			bbStates, err := character.FlagStates(s.charSaveData, s.SelectedChar, ids)
			if err != nil {
				t.Fatalf("FlagStates(bell bearings): %v", err)
			}
			wantTotal += len(bb)
			for _, b := range bb {
				if bbStates[b.FlagID] {
					wantUnlocked++
				}
			}
		}
		if total != wantTotal {
			t.Errorf("merchantGatedTotal[%q] = %d, want %d (flag groups)", name, total, wantTotal)
		}
		if got := s.merchantGatedUnlocked[name]; got != wantUnlocked {
			t.Errorf("merchantGatedUnlocked[%q] = %d, want %d", name, got, wantUnlocked)
		}
	}

	// The cache must not recompute (and must not require a re-selection)
	// when called again with nothing changed.
	before := s.gatedCacheChar
	s.ensureMerchantGated()
	if s.gatedCacheChar != before {
		t.Error("ensureMerchantGated recomputed despite nothing changing")
	}
}

// TestEniaReadOnlyLockDisplay covers Enia's safety-critical special case:
// her flags double as boss-progress state, so she must stay out of every
// unlock UI/write path, but the Shop Editor must read those same flags to
// display whether the selected character has already unlocked each item.
func TestEniaReadOnlyLockDisplay(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()

	if _, ok := s.merchantGatedTotal[eniaMerchantName]; ok {
		t.Error("Enia must not appear in merchantGatedTotal")
	}
	for _, name := range s.sortedGatedMerchants() {
		if name == eniaMerchantName {
			t.Fatal("Enia must not appear in sortedGatedMerchants")
		}
	}

	eniaRows, err := s.Catalog.MerchantRows(eniaMerchantName)
	if err != nil {
		t.Fatal(err)
	}
	var gatedRow *catalog.Row
	for _, r := range eniaRows {
		if r.UnlockFlag != 0 {
			gatedRow = r
			break
		}
	}
	if gatedRow == nil {
		t.Skip("no gated Enia row found in the fixture")
	}
	states, err := character.LockStates(s.charSaveData, s.SelectedChar, eniaRows)
	if err != nil {
		t.Fatal(err)
	}
	want, tracked := states[gatedRow.RowID]
	if !tracked {
		t.Fatalf("gated Enia row %d was not returned by LockStates", gatedRow.RowID)
	}
	if got, known := s.effectiveRowUnlocked(gatedRow.RowID); !known || got != want {
		t.Errorf("effectiveRowUnlocked(Enia row %d) = (%v, %v), want (%v, true)", gatedRow.RowID, got, known, want)
	}
	if !s.readOnlyGateRows[gatedRow.RowID] {
		t.Error("gated Enia row must be marked read-only")
	}

	// Defensive: selectFlagMerchant("Enia") must be a no-op even if reached
	// directly, not just unreachable through the list.
	s.selectFlagMerchant(eniaMerchantName)
	if s.UnlockMerchant == eniaMerchantName {
		t.Error("selectFlagMerchant must refuse to select Enia")
	}

	// A defensive direct call must remain unable to create a write, even
	// though effectiveRowUnlocked now has read-only knowledge of the flag.
	s.lastMerchant = eniaMerchantName
	s.MerchantRows = eniaRows
	s.setRowUnlockForSelectedChar(gatedRow, !want)
	if len(s.PendingFlagEdits[s.SelectedChar]) != 0 {
		t.Error("Enia lock display must never stage a flag write")
	}
}

// TestGroupFlagRowsGroupsSharedFlag checks the grouping used to collapse
// multiple rows gated by the identical UnlockFlag (e.g. a bell bearing
// purchase releasing a batch of items at once) into one checkbox.
func TestGroupFlagRowsGroupsSharedFlag(t *testing.T) {
	rows := []*catalog.Row{
		{RowID: 1, ItemName: "Boiled Crab", UnlockFlag: 500},
		{RowID: 2, ItemName: "Boiled Prawn", UnlockFlag: 500},
		{RowID: 3, ItemName: "Something Else", UnlockFlag: 900},
	}
	groups := groupFlagRows(rows)
	if len(groups) != 2 {
		t.Fatalf("groupFlagRows returned %d groups, want 2", len(groups))
	}
	if groups[0].FlagID != 500 || len(groups[0].Rows) != 2 {
		t.Errorf("group 0 = flag %d with %d rows, want flag 500 with 2 rows", groups[0].FlagID, len(groups[0].Rows))
	}
	if groups[1].FlagID != 900 || len(groups[1].Rows) != 1 {
		t.Errorf("group 1 = flag %d with %d rows, want flag 900 with 1 row", groups[1].FlagID, len(groups[1].Rows))
	}
}

// TestStageFlagGroupTogglesAllRowsInGroup checks that toggling one
// group's checkbox (the click-draining loop in layoutCharactersPanel)
// stages every row sharing that flag, not just one.
func TestStageFlagGroupTogglesAllRowsInGroup(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState(cat)
	s.SelectedChar = 0
	rows := []*catalog.Row{
		{RowID: 11, ItemName: "A", UnlockFlag: 500},
		{RowID: 12, ItemName: "B", UnlockFlag: 500},
	}
	s.FlagRows = rows
	s.FlagState = map[int64]bool{11: false, 12: false}

	groups := groupFlagRows(rows)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	chk := s.flagCheck(groups[0].FlagID)
	chk.Value = true // simulate the click having flipped the widget
	for _, r := range groups[0].Rows {
		s.stageFlag(r.RowID, chk.Value)
	}

	pending := s.PendingFlagEdits[0]
	if pending[11] != true || pending[12] != true {
		t.Errorf("PendingFlagEdits[0] = %v, want both row 11 and 12 staged true", pending)
	}
}

// TestEffectiveRowUnlockedPrefersPending checks the three answers
// effectiveRowUnlocked must give: unknown with no character selected,
// the committed (on-disk) value with nothing staged, and the staged
// value once one exists -- the same "pending wins" rule stageFlag itself
// uses.
func TestEffectiveRowUnlockedPrefersPending(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	if _, known := s.effectiveRowUnlocked(12345); known {
		t.Error("expected known=false with no character selected")
	}

	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()

	var rowID int64 = -1
	var committed bool
	for id, v := range s.charFlagState {
		rowID, committed = id, v
		break
	}
	if rowID < 0 {
		t.Skip("no gated row found for this character")
	}

	if got, known := s.effectiveRowUnlocked(rowID); !known || got != committed {
		t.Errorf("effectiveRowUnlocked = (%v,%v), want (%v,true) matching committed", got, known, committed)
	}

	s.PendingFlagEdits[s.SelectedChar] = map[int64]bool{rowID: !committed}
	if got, known := s.effectiveRowUnlocked(rowID); !known || got != !committed {
		t.Errorf("effectiveRowUnlocked with staged edit = (%v,%v), want (%v,true)", got, known, !committed)
	}
}

// TestDisplayMerchantUnlockedReactsToPendingEdits checks that staging an
// unlock (unsaved) for a row immediately shifts that row's merchant's
// displayed unlocked count, and un-staging it (back to the committed
// value) drops the count back to baseline -- this is what makes the
// merchant list recolor live instead of only after Save.
func TestDisplayMerchantUnlockedReactsToPendingEdits(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()

	var target string
	var lockedRowID int64 = -1
	for name, total := range s.merchantGatedTotal {
		if s.merchantGatedUnlocked[name] >= total {
			continue // fully unlocked already -- no locked row to stage
		}
		for rowID, unlocked := range s.charFlagState {
			if !unlocked && s.charFlagMerchant[rowID] == name {
				target, lockedRowID = name, rowID
				break
			}
		}
		if target != "" {
			break
		}
	}
	if target == "" {
		t.Skip("no merchant with a locked gated row found for this character")
	}

	before := s.displayMerchantUnlocked()[target]
	s.stageFlag(lockedRowID, true)
	if after := s.displayMerchantUnlocked()[target]; after != before+1 {
		t.Errorf("displayMerchantUnlocked()[%q] = %d after staging an unlock, want %d", target, after, before+1)
	}

	s.stageFlag(lockedRowID, false) // back to committed value -- unstages
	if got := s.displayMerchantUnlocked()[target]; got != before {
		t.Errorf("displayMerchantUnlocked()[%q] after unstaging = %d, want %d", target, got, before)
	}
}

func TestSelectFlagMerchantPopulatesRows(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()

	// Deterministic target (first by name) instead of a random map entry, so
	// a failure always reproduces.
	names := make([]string, 0, len(s.merchantGatedTotal))
	for name := range s.merchantGatedTotal {
		names = append(names, name)
	}
	if len(names) == 0 {
		t.Fatal("no gated merchant found to test against")
	}
	sort.Strings(names)
	target := names[0]

	s.selectFlagMerchant(target)
	if s.UnlockMerchant != target {
		t.Fatalf("UnlockMerchant = %q, want %q", s.UnlockMerchant, target)
	}
	// FlagRows carries only the shop-row groups; Twin Maiden Husks' bell
	// bearings live in a separate grid section, so subtract them from the
	// merchant total (which folds them in) before comparing.
	wantRows := s.merchantGatedTotal[target]
	if target == twinMaidenHusksMerchantName {
		wantRows -= len(character.BellBearingsForUI())
	}
	if got := len(groupFlagRows(s.FlagRows)); got != wantRows {
		t.Errorf("len(groupFlagRows(FlagRows)) = %d, want %d (merchantGatedTotal)", got, wantRows)
	}
	if len(s.FlagState) != len(s.FlagRows) {
		t.Errorf("FlagState len = %d, want %d", len(s.FlagState), len(s.FlagRows))
	}
	for _, r := range s.FlagRows {
		if r.UnlockFlag == 0 {
			t.Errorf("row %d in FlagRows has no gate at all", r.RowID)
		}
		unlocked, ok := s.FlagState[r.RowID]
		if !ok {
			t.Errorf("row %d missing from FlagState", r.RowID)
			continue
		}
		// The checkbox widget (keyed by flag id -- a flag may gate several
		// rows, see groupFlagRows) must be seeded to match the real state.
		if got := s.flagCheck(r.UnlockFlag).Value; got != unlocked {
			t.Errorf("flagCheck(flag %d).Value = %v, want %v (FlagState)", r.UnlockFlag, got, unlocked)
		}
	}

	// Re-selecting the same merchant toggles it off.
	s.selectFlagMerchant(target)
	if s.UnlockMerchant != "" {
		t.Errorf("UnlockMerchant = %q after re-selecting, want \"\" (toggle off)", s.UnlockMerchant)
	}
	if s.FlagRows != nil || s.FlagState != nil {
		t.Error("FlagRows/FlagState not cleared after deselecting the merchant")
	}
}

// TestStageFlagTracksPendingAndUnstagesAtCommittedValue checks stageFlag's
// core rule (mirroring item-edit staging): staging a row to a value
// different from its committed (on-disk) state records a pending entry;
// staging it back to the committed value removes the entry again.
func TestStageFlagTracksPendingAndUnstagesAtCommittedValue(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()

	var target string
	for name := range s.merchantGatedTotal {
		target = name
		break
	}
	if target == "" {
		t.Fatal("no gated merchant found to test against")
	}
	s.selectFlagMerchant(target)

	var row *catalog.Row
	for _, r := range s.FlagRows {
		if !s.FlagState[r.RowID] { // a locked (unlocked=false) row
			row = r
			break
		}
	}
	if row == nil {
		t.Skip("no locked gated row found for this merchant/character")
	}

	if s.pendingFlagCount(7) != 0 {
		t.Fatalf("pendingFlagCount = %d before any toggle, want 0", s.pendingFlagCount(7))
	}

	// Stage it to unlocked (different from committed false).
	s.stageFlag(row.RowID, true)
	if s.pendingFlagCount(7) != 1 {
		t.Fatalf("pendingFlagCount after staging = %d, want 1", s.pendingFlagCount(7))
	}
	if got := s.PendingFlagEdits[7][row.RowID]; got != true {
		t.Errorf("PendingFlagEdits[7][%d] = %v, want true", row.RowID, got)
	}

	// Stage it back to the committed value (false) -- must unstage.
	s.stageFlag(row.RowID, false)
	if s.pendingFlagCount(7) != 0 {
		t.Errorf("pendingFlagCount after un-staging = %d, want 0", s.pendingFlagCount(7))
	}
	if _, ok := s.PendingFlagEdits[7][row.RowID]; ok {
		t.Error("PendingFlagEdits[7] still has an entry after staging back to the committed value")
	}
}

// TestSelectFlagMerchantSeedsFromPendingOverCommitted checks that
// re-visiting a merchant with a staged-but-unsaved toggle shows the
// staged value, not the on-disk one -- otherwise navigating away and back
// would silently discard the user's pending choice.
func TestSelectFlagMerchantSeedsFromPendingOverCommitted(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()

	var target string
	for name := range s.merchantGatedTotal {
		target = name
		break
	}
	if target == "" {
		t.Fatal("no gated merchant found to test against")
	}
	s.selectFlagMerchant(target)

	var row *catalog.Row
	for _, r := range s.FlagRows {
		if !s.FlagState[r.RowID] {
			row = r
			break
		}
	}
	if row == nil {
		t.Skip("no locked gated row found for this merchant/character")
	}
	s.stageFlag(row.RowID, true) // stage to unlocked, committed state is false

	// Navigate away and back.
	s.selectFlagMerchant(target) // off
	s.selectFlagMerchant(target) // on again

	if got := s.flagCheck(row.UnlockFlag).Value; got != true {
		t.Errorf("flagCheck(flag %d).Value after re-visiting = %v, want true (the staged value, not committed false)", row.UnlockFlag, got)
	}
}

// TestBellBearingSectionOnlyForTMH checks that bellBearingState is only
// ever populated when Twin Maiden Husks is the expanded merchant --
// selecting any other merchant must leave it nil, since the bell-bearing
// section only ever renders for her (see docs/CHAR_UNLOCK.md's dated
// entry).
func TestBellBearingSectionOnlyForTMH(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()

	s.selectFlagMerchant(twinMaidenHusksMerchantName)
	if s.bellBearingState == nil {
		t.Fatal("bellBearingState is nil after selecting Twin Maiden Husks")
	}
	if len(s.bellBearingState) != len(character.BellBearingsForUI()) {
		t.Errorf("bellBearingState has %d entries, want %d (BellBearingsForUI)", len(s.bellBearingState), len(character.BellBearingsForUI()))
	}

	var other string
	for name := range s.merchantGatedTotal {
		if name != twinMaidenHusksMerchantName {
			other = name
			break
		}
	}
	if other == "" {
		t.Skip("no other gated merchant found for this character to test against")
	}
	s.selectFlagMerchant(other)
	if s.bellBearingState != nil {
		t.Errorf("bellBearingState = %v after selecting %q, want nil", s.bellBearingState, other)
	}
}

// TestStageBellBearingUnstagesAtCommittedValue mirrors
// TestStageFlagTracksPendingAndUnstagesAtCommittedValue for
// stageBellBearing: staging to a value different from the committed
// (on-disk) state records a pending entry; staging back to the committed
// value removes it.
func TestStageBellBearingUnstagesAtCommittedValue(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()
	s.selectFlagMerchant(twinMaidenHusksMerchantName)

	bb := character.BellBearingsForUI()
	if len(bb) == 0 {
		t.Fatal("BellBearingsForUI() is empty")
	}
	flagID := bb[0].FlagID
	committed := s.bellBearingState[flagID]

	if s.pendingBellBearingCount(7) != 0 {
		t.Fatalf("pendingBellBearingCount = %d before any toggle, want 0", s.pendingBellBearingCount(7))
	}

	s.stageBellBearing(flagID, !committed)
	if s.pendingBellBearingCount(7) != 1 {
		t.Fatalf("pendingBellBearingCount after staging = %d, want 1", s.pendingBellBearingCount(7))
	}
	if got := s.PendingBellBearingEdits[7][flagID]; got != !committed {
		t.Errorf("PendingBellBearingEdits[7][%d] = %v, want %v", flagID, got, !committed)
	}

	s.stageBellBearing(flagID, committed)
	if s.pendingBellBearingCount(7) != 0 {
		t.Errorf("pendingBellBearingCount after un-staging = %d, want 0", s.pendingBellBearingCount(7))
	}
	if _, ok := s.PendingBellBearingEdits[7][flagID]; ok {
		t.Error("PendingBellBearingEdits[7] still has an entry after staging back to the committed value")
	}
}

// TestRemovePendingFlagsForCharClearsBothMaps checks that one character's
// "Remove all" (RemovePendingFlagsForChar) clears both PendingFlagEdits
// and PendingBellBearingEdits for that character, not just row-gate
// edits.
func TestRemovePendingFlagsForCharClearsBothMaps(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.PendingFlagEdits[7] = map[int64]bool{100500: true}
	s.PendingBellBearingEdits[7] = map[uint32]bool{11109712: true}

	s.RemovePendingFlagsForChar(7)

	if len(s.PendingFlagEdits[7]) != 0 {
		t.Errorf("PendingFlagEdits[7] = %v after RemovePendingFlagsForChar, want empty", s.PendingFlagEdits[7])
	}
	if len(s.PendingBellBearingEdits[7]) != 0 {
		t.Errorf("PendingBellBearingEdits[7] = %v after RemovePendingFlagsForChar, want empty", s.PendingBellBearingEdits[7])
	}
}

// TestLayoutTMHFlagsSectionCoversEveryEntry is the direct regression test
// for the Flex-truncation bug (docs/CHAR_UNLOCK.md's dated follow-up):
// every one of Twin Maiden Husks' gated-row groups must land in exactly
// one of tmhBearings/otherItems (never both, never neither), and every
// character.BellBearingsForUI() entry must appear exactly once in
// npcBearings -- nothing dropped, nothing double-counted.
func TestLayoutTMHFlagsSectionCoversEveryEntry(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()
	s.selectFlagMerchant(twinMaidenHusksMerchantName)

	wantGroups := groupFlagRows(s.FlagRows)
	tmhBearings, otherItems, npcBearings := tmhFlagSections(s.FlagRows)

	if got, want := len(tmhBearings)+len(otherItems), len(wantGroups); got != want {
		t.Fatalf("len(tmhBearings)+len(otherItems) = %d, want %d (total gated-row groups)", got, want)
	}
	seen := make(map[int64]int, len(wantGroups))
	for _, it := range tmhBearings {
		seen[it.group.FlagID]++
		if !it.known {
			t.Errorf("tmhBearings contains flag %d marked known=false", it.group.FlagID)
		}
	}
	for _, it := range otherItems {
		seen[it.group.FlagID]++
		if it.known {
			t.Errorf("otherItems contains flag %d marked known=true", it.group.FlagID)
		}
	}
	for _, g := range wantGroups {
		if seen[g.FlagID] != 1 {
			t.Errorf("flag %d appears %d times across tmhBearings+otherItems, want exactly 1", g.FlagID, seen[g.FlagID])
		}
	}

	if got, want := len(npcBearings), len(character.BellBearingsForUI()); got != want {
		t.Fatalf("len(npcBearings) = %d, want %d (character.BellBearingsForUI(), unsplit)", got, want)
	}
	bbSeen := make(map[uint32]int, len(npcBearings))
	for _, b := range npcBearings {
		bbSeen[b.FlagID]++
	}
	for _, b := range character.BellBearingsForUI() {
		if bbSeen[b.FlagID] != 1 {
			t.Errorf("BellBearingsForUI() flag %d appears %d times in npcBearings, want exactly 1", b.FlagID, bbSeen[b.FlagID])
		}
	}
}

// TestBellBearingSortKeyGroupsFamiliesByNumber checks bellBearingSortKey
// strips a trailing " [N]" so same-family entries (e.g. every "Nomadic
// Merchant's Bell Bearing [N]") sort together in ascending N order, and
// leaves singular names (no bracket) untouched.
func TestBellBearingSortKeyGroupsFamiliesByNumber(t *testing.T) {
	cases := []struct {
		name     string
		wantBase string
		wantNum  int
	}{
		{"Nomadic Merchant's Bell Bearing [7]", "Nomadic Merchant's Bell Bearing", 7},
		{"Nomadic Merchant's Bell Bearing [10]", "Nomadic Merchant's Bell Bearing", 10},
		{"Isolated Merchant's Bell Bearing [1]", "Isolated Merchant's Bell Bearing", 1},
		{"Patches' Bell Bearing", "Patches' Bell Bearing", 0},
		{"Abandoned Merchant's Bell Bearing", "Abandoned Merchant's Bell Bearing", 0},
	}
	for _, c := range cases {
		base, num := bellBearingSortKey(c.name)
		if base != c.wantBase || num != c.wantNum {
			t.Errorf("bellBearingSortKey(%q) = (%q, %d), want (%q, %d)", c.name, base, num, c.wantBase, c.wantNum)
		}
	}

	bb := []character.BellBearing{
		{FlagID: 1, Name: "Nomadic Merchant's Bell Bearing [10]"},
		{FlagID: 2, Name: "Abandoned Merchant's Bell Bearing"},
		{FlagID: 3, Name: "Nomadic Merchant's Bell Bearing [2]"},
		{FlagID: 4, Name: "Nomadic Merchant's Bell Bearing [1]"},
	}
	sortBellBearingsByFamily(bb)
	// Shop 3 Nomadic bearings appear before Shop 4's Abandoned bearing.
	wantOrder := []uint32{4, 3, 1, 2} // Nomadic [1] < [2] < [10] numerically, then Abandoned
	for i, b := range bb {
		if b.FlagID != wantOrder[i] {
			t.Errorf("sortBellBearingsByFamily order[%d] = flag %d, want flag %d", i, b.FlagID, wantOrder[i])
		}
	}
}

// TestBellBearingGroupRankClustersShopFamilies checks the real TMH shop
// order: Shop 2, then Shop 3, then Shop 4's internally ordered merchant
// families, then Shop 5 DLC.
func TestBellBearingGroupRankClustersShopFamilies(t *testing.T) {
	bb := []character.BellBearing{
		{FlagID: 1, Name: "Moore's Bell Bearing", Category: "dlc"},
		{FlagID: 2, Name: "Abandoned Merchant's Bell Bearing", Category: "merchant"},
		{FlagID: 3, Name: "Nomadic Merchant's Bell Bearing [2]", Category: "merchant"},
		{FlagID: 4, Name: "Patches' Bell Bearing", Category: "npc"},
		{FlagID: 5, Name: "Isolated Merchant's Bell Bearing [1]", Category: "merchant"},
		{FlagID: 6, Name: "Hermit Merchant's Bell Bearing [1]", Category: "merchant"},
		{FlagID: 7, Name: "Nomadic Merchant's Bell Bearing [1]", Category: "merchant"},
		{FlagID: 8, Name: "Imprisoned Merchant's Bell Bearing", Category: "merchant"},
	}
	sortBellBearingsByFamily(bb)
	wantOrder := []uint32{4, 7, 3, 5, 6, 2, 8, 1}
	for i, b := range bb {
		if b.FlagID != wantOrder[i] {
			t.Fatalf("sortBellBearingsByFamily order[%d] = flag %d, want flag %d\ngot: %v", i, b.FlagID, wantOrder[i], bb)
		}
	}
}

// TestNamedBellBearingShopSequence fixes the two named-merchant submenu
// memberships and their underlying bearing order. The game item-sorts each
// shop's stock, so this is the stable sequence available for the checkboxes.
func TestNamedBellBearingShopSequence(t *testing.T) {
	want := map[int][]string{
		// Miriel and Gowry each have separate Sorcery/Incantation entries in
		// the game menu, but one save flag. Their one checkbox takes the
		// position of the first of those two entries.
		1: {"Sellen's Bell Bearing", "Seluvis's Bell Bearing", "Thops's Bell Bearing", "Corhyn's Bell Bearing", "Miriel's Bell Bearing", "D's Bell Bearing", "Gowry's Bell Bearing", "Rogier's Bell Bearing", "Bernahl's Bell Bearing", "Iji's Bell Bearing"},
		2: {"Gostoc's Bell Bearing", "Pidia's Bell Bearing", "Patches' Bell Bearing", "Blackguard's Bell Bearing"},
	}
	got := map[int][]string{1: {}, 2: {}}
	bearings := character.BellBearingsForUI()
	sortBellBearingsByFamily(bearings)
	for _, b := range bearings {
		if rank := bellBearingGroupRank(b); rank == 1 || rank == 2 {
			got[rank] = append(got[rank], b.Name)
		}
	}
	for shop, names := range want {
		if len(got[shop]) != len(names) {
			t.Fatalf("Shop %d named bearing count = %d, want %d (%v)", shop, len(got[shop]), len(names), got[shop])
		}
		for i, name := range names {
			if got[shop][i] != name {
				t.Errorf("Shop %d bearing[%d] = %q, want %q", shop, i, got[shop][i], name)
			}
		}
	}
}

// TestNPCBellBearingColumnsKeepNomadicFamilyVertical keeps the deliberately
// balanced Characters layout stable. The right side must be one uninterrupted
// Nomadic [1] through [10] run; Kalé joins the named/DLC side so the two
// columns have equal height instead of leaving the final rows orphaned.
func TestNPCBellBearingColumnsKeepNomadicFamilyVertical(t *testing.T) {
	bearings := character.BellBearingsForUI()
	sortBellBearingsByFamily(bearings)
	left, right := splitTMHBearingColumns(bearings)
	if len(left) != len(right) {
		t.Fatalf("balanced NPC Bell Bearing columns = %d/%d, want equal", len(left), len(right))
	}
	if len(left)+len(right) != len(bearings) {
		t.Fatalf("column entries = %d, want all %d Bell Bearing entries", len(left)+len(right), len(bearings))
	}
	for i := 1; i <= 10; i++ {
		want := fmt.Sprintf("Nomadic Merchant's Bell Bearing [%d]", i)
		if i-1 >= len(right) || right[i-1].Name != want {
			got := "<missing>"
			if i-1 < len(right) {
				got = right[i-1].Name
			}
			t.Errorf("right column Nomadic[%d] = %q, want %q", i, got, want)
		}
	}
	for _, b := range left {
		if strings.HasPrefix(b.Name, "Nomadic Merchant's") {
			t.Errorf("left column contains Nomadic bearing %q", b.Name)
		}
	}
}

// TestBellBearingSortIgnoresPartialMerchantMappings keeps a family ordered by
// its bearing number even where only some entries have a known shop mapping.
func TestBellBearingSortIgnoresPartialMerchantMappings(t *testing.T) {
	bb := []character.BellBearing{
		{FlagID: 7, Name: "Nomadic Merchant's Bell Bearing [7]", Merchant: "Nomadic Merchant - Altus Plateau", Category: "merchant"},
		{FlagID: 1, Name: "Nomadic Merchant's Bell Bearing [1]", Category: "merchant"},
		{FlagID: 3, Name: "Nomadic Merchant's Bell Bearing [3]", Category: "merchant"},
	}
	sortBellBearingsByFamily(bb)
	want := []uint32{1, 3, 7}
	for i, id := range want {
		if bb[i].FlagID != id {
			t.Fatalf("sortBellBearingsByFamily order[%d] = flag %d, want %d", i, bb[i].FlagID, id)
		}
	}
}

// TestAddTMHSectionPairedGroupsTwoPerRow checks addTMHSectionPaired's
// block count directly: a header + spacer (2 blocks) plus one block per
// PAIR of rows (ceil(n/2)), not one block per row like addTMHSection --
// covers both the even and odd-n (blank trailing cell) cases.
func TestAddTMHSectionPairedGroupsTwoPerRow(t *testing.T) {
	dummy := func(i int) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }
	}
	th := material.NewTheme()

	cases := []struct{ n, wantRowBlocks int }{
		{0, 0}, // section omitted entirely
		{1, 1},
		{2, 1},
		{3, 2},
		{4, 2},
		{7, 4},
	}
	for _, c := range cases {
		var blocks []layout.Widget
		addTMHSectionPaired(th, &blocks, "Test Section", c.n, dummy)
		if c.n == 0 {
			if len(blocks) != 0 {
				t.Errorf("n=0: got %d blocks, want 0 (section omitted)", len(blocks))
			}
			continue
		}
		// header + spacer + wantRowBlocks
		if want := 2 + c.wantRowBlocks; len(blocks) != want {
			t.Errorf("n=%d: got %d blocks, want %d (header+spacer+%d row-blocks)", c.n, len(blocks), want, c.wantRowBlocks)
		}
	}
}

// TestStartCombinedSaveRespectsPreconditions checks the no-op guard: the
// shared save button (v5) does nothing when nothing is staged, of either
// kind (item edits or character-flag edits).
func TestStartCombinedSaveRespectsPreconditions(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()

	if s.combinedPendingCount() != 0 {
		t.Fatal("expected nothing staged initially")
	}
	before := s.busy
	s.startCombinedSave()
	if s.busy != before {
		t.Error("startCombinedSave with nothing staged changed busy state")
	}
}

// TestCombinedPendingCountSumsBothKinds checks that the shared footer's
// count (state.go's combinedPendingCount) reflects staged item edits and
// staged character-flag edits together, from either or both.
func TestCombinedPendingCountSumsBothKinds(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	if s.combinedPendingCount() != 0 {
		t.Fatalf("combinedPendingCount() = %d before staging anything, want 0", s.combinedPendingCount())
	}

	s.PendingEdits[100500] = &RowEdit{FieldChanges: map[string]FieldChange{"value": {From: 100, To: 200}}}
	if got := s.combinedPendingCount(); got != 1 {
		t.Errorf("combinedPendingCount() = %d after 1 item edit, want 1", got)
	}

	s.PendingFlagEdits[0] = map[int64]bool{111: true, 222: false}
	if got := s.combinedPendingCount(); got != 3 {
		t.Errorf("combinedPendingCount() = %d after 1 item edit + 2 flag edits, want 3", got)
	}
}

// TestPendingFlagCountCollapsesSharedUnlockFlag verifies that the save batch
// retains every affected ShopLineup row while the UI reports the number of
// checkboxes the player actually changed. This prevents bulk merchant unlocks
// from showing a Pending count inflated by the number of stock slots behind a
// single bell-bearing/quest flag.
func TestPendingFlagCountCollapsesSharedUnlockFlag(t *testing.T) {
	s := newTestState(nil)
	s.PendingEdits = map[int64]*RowEdit{900: {}}
	s.PendingFlagEdits = map[int]map[int64]bool{3: {101: true, 102: true, 103: true}}
	s.PendingBellBearingEdits = map[int]map[uint32]bool{3: {7001: true}}
	s.charFlagFlag = map[int64]int64{101: 500, 102: 500, 103: 501}

	if got := s.pendingFlagCount(3); got != 2 {
		t.Fatalf("pendingFlagCount() = %d, want 2 checkbox changes", got)
	}
	if got := s.totalPendingFlagCount(); got != 2 {
		t.Fatalf("totalPendingFlagCount() = %d, want 2 checkbox changes", got)
	}
	if got := s.combinedPendingCount(); got != 4 {
		t.Fatalf("combinedPendingCount() = %d, want 4 (1 item + 2 flags + 1 bearing)", got)
	}
}

// TestLayoutCharactersPanelDoesNotPanic drives layoutCharactersPanel
// headlessly (no real window -- this environment has no GUI automation
// tooling to click through it) across every state this view can be in:
// no save loaded, a save loaded with nothing picked, a character with no
// gated stock, a character with a merchant's flags shown, Debug mode, and
// mid-write "busy". Only checks for a panic/nil-deref/index-out-of-range
// -- not a substitute for an actual visual pass, which the user should
// still do once.
func TestLayoutCharactersPanelDoesNotPanic(t *testing.T) {
	th := material.NewTheme()

	run := func(name string, build func(s *State)) {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			s := NewState(loadedTestCatalog(t))
			s.view = viewCharacters
			build(s)
			s.layoutCharactersPanel(headlessGtx(), th)
		})
	}

	run("no-save-loaded", func(s *State) {
		s.Catalog, _ = catalog.New() // fresh, unloaded catalog
	})
	run("loaded-nothing-picked", func(s *State) {})
	run("char-with-no-gated-stock", func(s *State) {
		s.ensureCharList()
		// Every real slot in the fixture has some gated stock (see
		// docs/CHAR_UNLOCK.md), so force the "none gated" branch directly
		// rather than relying on fixture data staying that way.
		s.SelectedChar = 2
		s.gatedCacheChar = 2
		s.gatedCachePath = s.charDataPath
		s.merchantGatedTotal = map[string]int{}
		s.merchantGatedUnlocked = map[string]int{}
	})
	run("char-with-merchant-flags-shown", func(s *State) {
		s.ensureCharList()
		s.selectCharacter(7)
		s.ensureMerchantGated()
		for name := range s.merchantGatedTotal {
			s.selectFlagMerchant(name)
			break
		}
	})
	run("risky-items-shown-merchant-flags", func(s *State) {
		s.Settings.ShowRiskyItems = true
		s.ensureCharList()
		s.selectCharacter(7)
		s.ensureMerchantGated()
		for name := range s.merchantGatedTotal {
			s.selectFlagMerchant(name)
			break
		}
	})
	run("busy", func(s *State) {
		s.ensureCharList()
		s.selectCharacter(7)
		s.ensureMerchantGated()
		for name := range s.merchantGatedTotal {
			s.selectFlagMerchant(name)
			break
		}
		s.busy = true
	})
	run("staged-pending-edits", func(s *State) {
		s.ensureCharList()
		s.selectCharacter(7)
		s.ensureMerchantGated()
		for name := range s.merchantGatedTotal {
			s.selectFlagMerchant(name)
			break
		}
		for _, r := range s.FlagRows {
			s.stageFlag(r.RowID, !s.FlagState[r.RowID])
			break
		}
	})
	run("twin-maiden-husks-bell-bearings-shown", func(s *State) {
		s.ensureCharList()
		s.selectCharacter(7)
		s.ensureMerchantGated()
		s.selectFlagMerchant(twinMaidenHusksMerchantName)
		for _, b := range character.BellBearingsForUI() {
			s.stageBellBearing(b.FlagID, !s.bellBearingState[b.FlagID])
			break
		}
	})
}

// TestOpenAndFooterBarsStayOneLineTall guards against exactly the bug the
// user reported from a screenshot: panelSurface forces Min=Max, which is
// only correct inside a Flexed(1) region -- using it for the Rigid top/
// bottom bars made the Open button balloon to fill nearly the whole
// window. barSurface (window.go) must keep these bars a single line tall
// while still spanning the full window width. The footer bar (v5: shared
// across every view, see window.go's Layout) replaces what used to be
// the Characters view's own separate Save Character/Save All bar here.
// TestPathEditorFillsCappedWidth guards against exactly the bug the user
// reported from a screenshot right after the 2/5-width cap shipped: the
// path editor rendered "super small" because capping Max.X without also
// forcing Min.X == Max.X left a Rigid Flex child free to shrink to its
// placeholder text's natural width instead of actually filling the cap.
func TestPathEditorFillsCappedWidth(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	th := material.NewTheme()
	gtx := headlessGtx()
	barWidth := gtx.Constraints.Max.X

	dims := s.layoutPathEditor(gtx, th, barWidth)

	want := barWidth * pathEditorWidthNum / pathEditorWidthDen
	if dims.Size.X != want {
		t.Errorf("path editor width = %dpx, want exactly %dpx (2/5 of bar width %dpx) -- shrank to its placeholder text instead of filling the capped width", dims.Size.X, want, barWidth)
	}
}

func TestOpenAndFooterBarsStayOneLineTall(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	th := material.NewTheme()
	const winW, winH = 1200, 800

	gtx := layout.Context{
		Ops:         &op.Ops{},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(winW, winH)},
	}
	openDims := barSurface(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.layoutOpenBar(gtx, th)
	})
	footerDims := barSurface(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.layoutFooterPendingControls(gtx, th)
	})

	const maxBarHeight = 100 // generous upper bound for one line + padding
	for name, dims := range map[string]layout.Dimensions{"open bar": openDims, "footer bar": footerDims} {
		if dims.Size.Y > maxBarHeight {
			t.Errorf("%s height = %dpx, want <= %dpx (single line) -- looks like the panelSurface Min=Max bug is back", name, dims.Size.Y, maxBarHeight)
		}
		if dims.Size.Y >= winH/2 {
			t.Errorf("%s height = %dpx, more than half the %dpx window -- ballooned", name, dims.Size.Y, winH)
		}
		wantMinWidth := winW * 9 / 10 // must span (almost) the full window width
		if dims.Size.X < wantMinWidth {
			t.Errorf("%s width = %dpx, want >= %dpx (should span the app's full length)", name, dims.Size.X, wantMinWidth)
		}
	}
}

// TestHeaderTabsAreRightAligned guards the header's leading flexSpacer:
// before flexSpacer existed, a bare `layout.Dimensions{}` return ignored
// the width Flex forced on it, silently pushing the view tabs to the far
// left instead of the top-right corner the design (and docs) call for.
func TestHeaderTabsAreRightAligned(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	th := material.NewTheme()
	const winW = 1200

	gtx := layout.Context{
		Ops:         &op.Ops{},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(winW, 100)},
	}
	dims := s.layoutHeader(gtx, th)

	// The row's own reported width must span the full window (it has a
	// leading Flexed(1) spacer), and the tabs must end at (near) the
	// right edge, not sit at the left.
	wantMinWidth := winW * 9 / 10
	if dims.Size.X < wantMinWidth {
		t.Fatalf("header width = %dpx, want >= %dpx (full window width)", dims.Size.X, wantMinWidth)
	}
}

// TestFullLayoutHeadlessAllViews drives the whole app's top-level Layout
// (header + panel dispatch, see window.go) for each of the 3 views,
// headlessly. Guards the header-button/view-dispatch wiring (tabCharsBtn/
// tabEditorBtn/tabSettingsBtn, viewCharacters as the default view) against
// a future regression, on top of TestLayoutCharactersPanelDoesNotPanic's
// narrower coverage of the Characters panel itself.
func TestFullLayoutHeadlessAllViews(t *testing.T) {
	th := material.NewTheme()
	s := NewState(loadedTestCatalog(t))
	if s.view != viewCharacters {
		t.Errorf("NewState view = %d, want viewCharacters (%d) -- must be the landing view", s.view, viewCharacters)
	}
	for _, v := range []int{viewEditor, viewSettings, viewCharacters} {
		t.Run(viewName(v), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			s.view = v
			s.Layout(headlessGtx(), th)
		})
	}
}

func viewName(v int) string {
	switch v {
	case viewEditor:
		return "editor"
	case viewSettings:
		return "settings"
	case viewCharacters:
		return "characters"
	default:
		return "unknown"
	}
}

func TestToggleEveryMerchantUnlocksRestoresExactlyItsOwnChanges(t *testing.T) {
	// Mark the cache warm so this pure state-machine test does not need a
	// decoded save. The method must use the already-built safe, character-wide
	// cache rather than the one currently open merchant.
	s := newTestState(nil)
	s.SelectedChar = 1
	s.charDataPath = "fixture"
	s.gatedCachePath = "fixture"
	s.gatedCacheChar = 1
	s.charFlagState = map[int64]bool{101: false, 102: true}
	s.tmhBellCommitted = map[uint32]bool{}
	s.PendingFlagEdits = map[int]map[int64]bool{1: {102: false}}
	s.PendingBellBearingEdits = map[int]map[uint32]bool{}
	s.toggleEveryMerchantUnlocks()

	if got := s.PendingFlagEdits[1][101]; !got {
		t.Errorf("locked row 101 staged = %v, want true", got)
	}
	if _, kept := s.PendingFlagEdits[1][102]; kept {
		t.Error("row 102 was previously staged false but is already unlocked; Check all merchants must clear that stale uncheck")
	}
	firstBearing := character.BellBearingsForUI()[0].FlagID
	if got := s.PendingBellBearingEdits[1][firstBearing]; !got {
		t.Errorf("bell bearing %d staged = %v, want true", firstBearing, got)
	}

	// A second press reverses only this button's contribution: row 101 is
	// restored to no staged edit and the earlier manual false on row 102 is
	// brought back exactly as it was.
	s.toggleEveryMerchantUnlocks()
	if _, kept := s.PendingFlagEdits[1][101]; kept {
		t.Error("row 101 is still staged after Undo all-merchant unlock")
	}
	if got, kept := s.PendingFlagEdits[1][102]; !kept || got {
		t.Errorf("row 102 after undo = (%v, staged=%v), want (false, true)", got, kept)
	}
	if _, kept := s.PendingBellBearingEdits[1][firstBearing]; kept {
		t.Errorf("bell bearing %d is still staged after Undo all-merchant unlock", firstBearing)
	}
}
