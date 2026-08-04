package editor

// Pure error-formatting tests (no window/frame loop involved).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"er_merchant_editor/app/catalog"
	"er_merchant_editor/app/charunlock"
)

func TestFriendlyLoadErrorForMissingFile(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "does-not-exist.dat")
	loadErr := cat.LoadSave(path)
	if loadErr == nil {
		t.Fatal("expected LoadSave to fail for a nonexistent path")
	}

	got := friendlyLoadError(path, loadErr)
	want := "File not found: " + path
	if got != want {
		t.Errorf("friendlyLoadError = %q, want %q", got, want)
	}
}

func TestFriendlyLoadErrorPassesThroughOtherErrors(t *testing.T) {
	other := errors.New("some other failure")
	if got := friendlyLoadError("whatever", other); got != other.Error() {
		t.Errorf("friendlyLoadError = %q, want %q (passthrough)", got, other.Error())
	}
}

// TestStartLoadSaveSetsFriendlyErrorForMissingFile exercises the real
// async path a "Load" click drives (StartLoadSave -> loadWorker), not
// just friendlyLoadError in isolation -- covers the whole plumbing a
// typed nonexistent path goes through.
func TestStartLoadSaveSetsFriendlyErrorForMissingFile(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState(cat)
	path := filepath.Join(t.TempDir(), "does-not-exist.dat")

	s.StartLoadSave(path)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s.Busy() {
		time.Sleep(5 * time.Millisecond)
	}
	if s.Busy() {
		t.Fatal("StartLoadSave never finished (loadWorker deadlocked or timed out)")
	}

	want := "File not found: " + path
	if got := s.InlineErr(); got != want {
		t.Errorf("InlineErr() after loading a missing file = %q, want %q", got, want)
	}
}

// TestConsumeResetClearsPendingFlagEditsImmediately checks that both
// consumeReset branches (a full reload, and an item-edit apply) clear
// staged character-flag edits right away, not lazily via ensureCharList
// -- v5's shared "Pending (N)" footer is visible from every view, so a
// combined save started from the Shop Editor must not leave stale flag-
// edit state around just because the Characters view isn't rendering.
func TestConsumeResetClearsPendingFlagEditsImmediately(t *testing.T) {
	s := NewState(loadedTestCatalog(t))

	s.charDataPath = "some/path.dat"
	s.PendingFlagEdits = map[int]map[int64]bool{0: {111: true}}
	s.resetPending = true
	s.consumeReset()
	if len(s.PendingFlagEdits) != 0 {
		t.Errorf("PendingFlagEdits after a pending reload = %v, want empty", s.PendingFlagEdits)
	}
	if s.charDataPath != "" {
		t.Errorf("charDataPath after a pending reload = %q, want empty", s.charDataPath)
	}

	s.charDataPath = "some/other/path.dat"
	s.PendingFlagEdits = map[int]map[int64]bool{0: {222: true}}
	s.applyDone = true
	s.consumeReset()
	if len(s.PendingFlagEdits) != 0 {
		t.Errorf("PendingFlagEdits after an applied save = %v, want empty", s.PendingFlagEdits)
	}
	if s.charDataPath != "" {
		t.Errorf("charDataPath after an applied save = %q, want empty", s.charDataPath)
	}
}

// TestCombinedApplyWorkerMergesItemAndFlagEdits is the real risk this
// round's save-flow unification introduced: an item edit (regulation.bin,
// via shopwrite) and a character-flag edit (a completely different file
// region, via charunlock) staged together and saved through ONE dialog
// must both land in the SAME output file. Exercises
// combinedApplyWorker's "both" branch (tmp file + reload + ApplyEdits)
// directly against the real fixture, then verifies each half
// independently by reloading the written file.
func TestCombinedApplyWorkerMergesItemAndFlagEdits(t *testing.T) {
	s := NewState(loadedTestCatalog(t))

	merchants, err := s.Catalog.ListMerchants()
	if err != nil || len(merchants) == 0 {
		t.Fatal("no merchants in fixture")
	}
	var editRow *catalog.Row
	for _, m := range merchants {
		rows, err := s.Catalog.MerchantRows(m.Name)
		if err != nil {
			continue
		}
		for _, r := range rows {
			if !r.MaterialLocked && r.Price != nil {
				editRow = r
				break
			}
		}
		if editRow != nil {
			break
		}
	}
	if editRow == nil {
		t.Fatal("no editable priced row found in fixture")
	}
	newPrice := *editRow.Price + 1
	s.StageField(s.PendingEdits, editRow, "value", newPrice)

	s.ensureCharList()
	s.selectCharacter(7)
	s.ensureMerchantGated()
	var flagRowID int64 = -1
	for rowID, unlocked := range s.charFlagState {
		if !unlocked {
			flagRowID = rowID
			break
		}
	}
	if flagRowID < 0 {
		t.Skip("no locked gated row found for character 7")
	}
	s.PendingFlagEdits[7] = map[int64]bool{flagRowID: true}

	itemEdits := s.BuildEdits()
	rowsByID, err := s.Catalog.RowsByID()
	if err != nil {
		t.Fatal(err)
	}
	flagTargets := map[int][]charunlock.FlagTarget{
		7: {{FlagID: rowsByID[flagRowID].UnlockFlag, Released: true}},
	}

	outPath := filepath.Join(t.TempDir(), "combined.dat")
	s.combinedApplyWorker(itemEdits, flagTargets, nil, outPath)

	if msg := s.ApplyErr(); msg != "" {
		t.Fatalf("combinedApplyWorker inline error: %s", msg)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	if _, err := os.Stat(outPath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("tmp file left behind at %s.tmp", outPath)
	}

	cat2, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := cat2.LoadSave(outPath); err != nil {
		t.Fatalf("reloading combined output: %v", err)
	}
	rows2, err := cat2.RowsByID()
	if err != nil {
		t.Fatal(err)
	}
	got := rows2[editRow.RowID]
	if got == nil || got.Price == nil || *got.Price != newPrice {
		t.Errorf("item edit not present in combined output for row %d, want price %d", editRow.RowID, newPrice)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	states, err := charunlock.LockStates(data, 7, []*catalog.Row{rowsByID[flagRowID]})
	if err != nil {
		t.Fatal(err)
	}
	if !states[flagRowID] {
		t.Errorf("flag edit not present in combined output: row %d still locked for character 7", flagRowID)
	}
}

// TestCombinedApplyWorkerMergesBellBearingEdits mirrors
// TestCombinedApplyWorkerMergesItemAndFlagEdits for a staged Twin Maiden
// Husks bell-bearing acquisition toggle (PendingBellBearingEdits, no
// backing catalog.Row) -- stages it, builds flagTargets the same way
// startCombinedSave does (see state.go), writes via combinedApplyWorker,
// reloads the output file, and confirms the flag reads back set via
// charunlock.FlagStates.
func TestCombinedApplyWorkerMergesBellBearingEdits(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.ensureCharList()

	bb := charunlock.BellBearingsForUI()
	if len(bb) == 0 {
		t.Fatal("BellBearingsForUI() is empty")
	}
	flagID := bb[0].FlagID
	s.PendingBellBearingEdits[7] = map[uint32]bool{flagID: true}

	var flagTargets map[int][]charunlock.FlagTarget
	if s.totalPendingBellBearingCount() > 0 {
		flagTargets = make(map[int][]charunlock.FlagTarget, len(s.PendingBellBearingEdits))
		for charIndex, edits := range s.PendingBellBearingEdits {
			for id, released := range edits {
				flagTargets[charIndex] = append(flagTargets[charIndex], charunlock.FlagTarget{
					FlagID:   int64(id),
					Released: released,
					Label:    fmt.Sprintf("bell bearing flag %d", id),
				})
			}
		}
	}

	outPath := filepath.Join(t.TempDir(), "combined_bb.dat")
	s.combinedApplyWorker(nil, flagTargets, nil, outPath)

	if msg := s.ApplyErr(); msg != "" {
		t.Fatalf("combinedApplyWorker inline error: %s", msg)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("output file not written: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	states, err := charunlock.FlagStates(data, 7, []uint32{flagID})
	if err != nil {
		t.Fatal(err)
	}
	if !states[flagID] {
		t.Errorf("bell bearing edit not present in combined output: flag %d still unset for character 7", flagID)
	}
}

// TestCombinedApplyWorkerWritesSellValueGuard is the end-to-end version of
// the 2026-07-31 cost=0 fix (see docs/MERCHANT_DATA.md): stages a real
// price-0 edit on a row whose item has a real sellValue and is
// flag-gated, runs the full combinedApplyWorker pipeline, then re-opens
// the written file and independently confirms: the row's price is 0, the
// row's own unlock gate is left completely untouched (the 2026-07-30
// auto-clear guard was removed 2026-08-02 -- the gate belongs to the
// slot, not the item in it), and the item's OWN EquipParam* sellValue got
// mirrored to match (0, confirmed 2026-07-31 behaviorally identical to
// the earlier -1 sentinel -- this fix).
func TestCombinedApplyWorkerWritesSellValueGuard(t *testing.T) {
	s := NewState(loadedTestCatalog(t))

	merchants, err := s.Catalog.ListMerchants()
	if err != nil || len(merchants) == 0 {
		t.Fatal("no merchants in fixture")
	}
	var editRow *catalog.Row
	var equipType, equipID, sellValue int64
	for _, m := range merchants {
		rows, rerr := s.Catalog.MerchantRows(m.Name)
		if rerr != nil {
			continue
		}
		for _, r := range rows {
			if r.MaterialLocked || r.Price == nil || *r.Price == 0 || r.UnlockFlag == 0 || r.ItemID == nil {
				continue
			}
			et, eid := r.Fields["equipType"], r.Fields["equipId"]
			sv, ok := s.Catalog.SellValue(et, eid)
			if !ok || sv <= 0 {
				continue // need a REAL positive sellValue to prove it gets cleared
			}
			editRow, equipType, equipID, sellValue = r, et, eid, sv
			break
		}
		if editRow != nil {
			break
		}
	}
	if editRow == nil {
		t.Skip("no gated, priced, real-sellValue row found in fixture")
	}

	s.StageField(s.PendingEdits, editRow, "value", 0)

	itemEdits := s.BuildEdits()
	equipParamEdits, err := s.BuildEquipParamEdits()
	if err != nil {
		t.Fatal(err)
	}
	if len(equipParamEdits) == 0 {
		t.Fatalf("BuildEquipParamEdits produced nothing for a real-sellValue (%d) item", sellValue)
	}

	outPath := filepath.Join(t.TempDir(), "sellvalue_guard.dat")
	s.combinedApplyWorker(itemEdits, nil, equipParamEdits, outPath)

	if msg := s.ApplyErr(); msg != "" {
		t.Fatalf("combinedApplyWorker inline error: %s", msg)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := os.Stat(fmt.Sprintf("%s.tmp%d", outPath, i)); !os.IsNotExist(err) {
			t.Errorf("tmp file left behind at %s.tmp%d", outPath, i)
		}
	}

	cat2, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := cat2.LoadSave(outPath); err != nil {
		t.Fatalf("reloading combined output: %v", err)
	}
	rows2, err := cat2.RowsByID()
	if err != nil {
		t.Fatal(err)
	}
	got := rows2[editRow.RowID]
	if got == nil || got.Price == nil || *got.Price != 0 {
		t.Fatalf("row %d price not written as 0 in combined output", editRow.RowID)
	}
	if got.UnlockFlag != editRow.UnlockFlag {
		t.Errorf("row %d unlock gate must be left untouched by a price edit, want %d got %d", editRow.RowID, editRow.UnlockFlag, got.UnlockFlag)
	}
	gotSellValue, ok := cat2.SellValue(equipType, equipID)
	if !ok || gotSellValue != 0 {
		t.Errorf("item (equipType=%d, equipID=%d) sellValue = %v (ok=%v), want 0 (mirrors the row's price)", equipType, equipID, gotSellValue, ok)
	}
}

// TestCombinedApplyWorkerSellValueRisesWhenSafeCappedBySiblings covers the
// 2026-08-01 global-minimum redesign (see docs/MERCHANT_DATA.md):
// sellValue is recomputed fresh each write as the true minimum effective
// price across EVERY row in the save currently selling that item, not
// remembered/clamped from a prior session. That means it can both SHRINK
// (an edited row priced below every sibling) and RISE back up once safe
// (every remaining low-priced row for that item is gone) -- but a rise is
// always capped by whatever the cheapest UNTOUCHED sibling row still sells
// the same item for, since raising sellValue past that would make that
// sibling row invisible (price < sellValue). Runs combinedApplyWorker
// TWICE against two independent State/Catalog instances (simulating two
// separate app sessions), chaining session 2's input to session 1's real
// output file.
func TestCombinedApplyWorkerSellValueRisesWhenSafeCappedBySiblings(t *testing.T) {
	baseRowsByID, err := loadedTestCatalog(t).RowsByID()
	if err != nil {
		t.Fatal(err)
	}

	type ref struct{ equipType, equipID int64 }
	byRef := map[ref][]*catalog.Row{}
	for _, r := range baseRowsByID {
		if r.MaterialLocked || r.Price == nil {
			continue
		}
		k := ref{r.Fields["equipType"], r.Fields["equipId"]}
		byRef[k] = append(byRef[k], r)
	}

	// Candidate items: sold by 2+ distinct rows, with at least one sibling
	// priced well above 1 -- gives room to prove both the shrink and the
	// safe-partial-rise halves of the new design. Sorted for a deterministic
	// try order (Go map iteration order is randomized per run).
	type candidate struct {
		row              *catalog.Row
		equipType, equID int64
		siblingMin       int64
	}
	var candidates []candidate
	for k, rows := range byRef {
		if len(rows) < 2 {
			continue
		}
		sibMin := int64(-1)
		for _, r := range rows[1:] {
			if sibMin == -1 || *r.Price < sibMin {
				sibMin = *r.Price
			}
		}
		if sibMin > 1 {
			candidates = append(candidates, candidate{rows[0], k.equipType, k.equipID, sibMin})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].equipType != candidates[j].equipType {
			return candidates[i].equipType < candidates[j].equipType
		}
		return candidates[i].equID < candidates[j].equID
	})
	if len(candidates) == 0 {
		t.Skip("no fixture item sold by 2+ rows with a sibling price > 1")
	}

	// Session 1: price one candidate row down to 1, well below every
	// sibling -- global minimum becomes 1, so sellValue must drop to 1.
	// Some real items' rows sit in a zstd block whose fresh recompression
	// legitimately exceeds the save's fixed ciphertext capacity (the
	// documented PS4 "no real re-compression" constraint --
	// app/shopwrite/apply.go, docs/MERCHANT_DATA.md) -- an orthogonal,
	// real safety guard unrelated to sellValue tracking, so on that
	// specific error try the next candidate instead of failing the test.
	var editRow *catalog.Row
	var equipType, equipID, siblingMin int64
	var cat1 *catalog.Catalog
	for i, c := range candidates {
		if i >= 10 {
			t.Fatal("gave up after 10 candidates all hit the PS4 capacity guard")
		}
		trial := NewState(loadedTestCatalog(t))
		trialRows, err := trial.Catalog.RowsByID()
		if err != nil {
			t.Fatal(err)
		}
		trialRow := trialRows[c.row.RowID]
		trial.StageField(trial.PendingEdits, trialRow, "value", 1)
		itemEdits1 := trial.BuildEdits()
		equipParamEdits1, err := trial.BuildEquipParamEdits()
		if err != nil {
			t.Fatal(err)
		}
		if len(equipParamEdits1) == 0 {
			t.Fatalf("candidate row %d: BuildEquipParamEdits produced nothing", c.row.RowID)
		}
		out1 := filepath.Join(t.TempDir(), fmt.Sprintf("session1-%d.dat", i))
		trial.combinedApplyWorker(itemEdits1, nil, equipParamEdits1, out1)
		if msg := trial.ApplyErr(); msg != "" {
			if strings.Contains(msg, "exceeds capacity") {
				continue
			}
			t.Fatalf("candidate row %d: session 1 combinedApplyWorker inline error: %s", c.row.RowID, msg)
		}

		c1, err := catalog.New()
		if err != nil {
			t.Fatal(err)
		}
		if err := c1.LoadSave(out1); err != nil {
			t.Fatal(err)
		}
		editRow, equipType, equipID, siblingMin, cat1 = c.row, c.equipType, c.equID, c.siblingMin, c1
		break
	}
	if editRow == nil {
		t.Skip("no candidate item could be written within the PS4 capacity guard")
	}
	sv1, ok := cat1.SellValue(equipType, equipID)
	if !ok || sv1 != 1 {
		t.Fatalf("after session 1: sellValue = %v (ok=%v), want 1", sv1, ok)
	}

	// Session 2: a FRESH State/Catalog loading session 1's own output
	// raises editRow's price far above every sibling. sellValue may rise
	// back up now that it's safe to, but only as far as siblingMin -- the
	// untouched sibling rows still selling this item at siblingMin would
	// go invisible (price < sellValue) if it rose any higher.
	s2 := NewState(cat1)
	rows2, err := s2.Catalog.RowsByID()
	if err != nil {
		t.Fatal(err)
	}
	row2 := rows2[editRow.RowID]
	if row2 == nil {
		t.Fatal("edited row missing after session 1's save")
	}
	highPrice := siblingMin*10 + 1000
	s2.StageField(s2.PendingEdits, row2, "value", highPrice)
	itemEdits2 := s2.BuildEdits()
	equipParamEdits2, err := s2.BuildEquipParamEdits()
	if err != nil {
		t.Fatal(err)
	}
	if len(equipParamEdits2) == 0 {
		t.Fatal("session 2: BuildEquipParamEdits should raise sellValue back toward siblingMin, produced nothing")
	}

	out2 := filepath.Join(t.TempDir(), "session2.dat")
	s2.combinedApplyWorker(itemEdits2, nil, equipParamEdits2, out2)
	if msg := s2.ApplyErr(); msg != "" {
		t.Fatalf("session 2 combinedApplyWorker inline error: %s", msg)
	}

	cat2, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := cat2.LoadSave(out2); err != nil {
		t.Fatal(err)
	}
	rows2b, err := cat2.RowsByID()
	if err != nil {
		t.Fatal(err)
	}
	if got := rows2b[editRow.RowID]; got == nil || got.Price == nil || *got.Price != highPrice {
		t.Fatalf("row %d price not written as %d in session 2's output", editRow.RowID, highPrice)
	}
	sv2, ok := cat2.SellValue(equipType, equipID)
	if !ok || sv2 != siblingMin {
		t.Errorf("after session 2 (price raised far above every sibling): sellValue = %v (ok=%v), want %d (capped by the cheapest untouched sibling row, not %d)", sv2, ok, siblingMin, highPrice)
	}
}
