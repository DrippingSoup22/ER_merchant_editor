package gio

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/application"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/platform/dialogs"
)

// --- load lifecycle (thread-safe snapshot accessors) ---

// Busy reports whether a save load or an edit apply is in progress.
func (s *State) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy
}

// clearBusy resets busy after a flow that set it (e.g. startCombinedSave)
// aborts before reaching the worker that would otherwise clear it itself
// (combinedApplyWorker, loadWorker) -- dialog canceled/failed, or a
// pre-dialog validation error.
func (s *State) clearBusy() {
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
}

// BusyMsg describes the in-flight operation ("" when idle).
func (s *State) BusyMsg() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.busy {
		return ""
	}
	return s.busyMsg
}

// ApplyErr returns the pending-dropdown inline error ("" = none).
func (s *State) ApplyErr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyErr
}

// ClearApplyErr dismisses the pending-dropdown inline error.
func (s *State) ClearApplyErr() {
	s.mu.Lock()
	s.applyErr = ""
	s.mu.Unlock()
}

// LoadedName returns the base filename of the loaded save ("" = none).
func (s *State) LoadedName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadedName
}

// InlineErr returns the current save-switcher inline error text.
func (s *State) InlineErr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inlineErr
}

// StartLoadSave loads a save off the UI goroutine. The heavy
// decrypt/decompress and the first row decode both run in the worker, so the
// frame loop never blocks; it requests a redraw when done. A second call while
// a load is in flight is ignored.
func (s *State) StartLoadSave(path string) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return
	}
	s.busy = true
	s.busyMsg = "Loading save..."
	s.inlineErr = ""
	s.mu.Unlock()

	go s.loadWorker(path)
}

func (s *State) loadWorker(path string) {
	err := s.Session.Load(path)

	s.mu.Lock()
	s.busy = false
	if err != nil {
		s.inlineErr = friendlyLoadError(path, err)
	} else {
		s.inlineErr = ""
		s.loadedName = filepath.Base(path)
		// UI-goroutine-owned fields must not be touched from this worker;
		// flag the reset for the frame loop to apply instead.
		s.resetPending = true
	}
	s.mu.Unlock()
	s.invalidate()
}

// friendlyLoadError turns a missing-file load error into a plain "File not
// found" message instead of Go's raw *fs.PathError text; any other error
// (wrong format, corrupt data, etc.) passes through unchanged.
func friendlyLoadError(path string, err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "File not found: " + path
	}
	return err.Error()
}

// consumeReset applies the after-load UI reset on the UI goroutine (called at
// the top of every frame): a fresh save invalidates whatever was selected or
// staged against the old one. An apply-success instead clears only the staging
// and re-reads the current merchant's rows — the save's content changed but the
// user's place in the UI (merchant, selected row) is still meaningful.
func (s *State) consumeReset() {
	s.mu.Lock()
	pending := s.resetPending
	applied := s.applyDone
	s.resetPending = false
	s.applyDone = false
	s.mu.Unlock()

	if pending {
		s.clearRowSelection()
		s.lastMerchant = ""
		s.merchantsPath = ""
		s.clearSelection()
		s.resetStagingCommon()
		return
	}
	if applied {
		// The catalog's save path moved to the Save-As target; adopt it without
		// repopulating the merchant combo (same merchants, updated rows). The
		// apply worker pre-warmed the row decode, so this re-read is cheap.
		s.merchantsPath = s.Catalog.SavePath()
		if s.lastMerchant != "" {
			if rows, err := s.Catalog.MerchantRows(s.lastMerchant); err == nil {
				s.MerchantRows = rows
			}
		}
		s.resetStagingCommon()
	}
}

// resetStagingCommon clears the shop-editor staging state shared by both
// consumeReset outcomes above (a fresh load invalidates ALL of it; a
// completed apply clears only the staging, since MerchantRows/lastMerchant
// are handled separately by each caller) plus the Characters view's own
// staged state and the Reset-to-Vanilla button's availability cache --
// every consumeReset outcome ends here.
func (s *State) resetStagingCommon() {
	s.Session.ClearPending()
	s.PickingForRows = nil
	s.formRowIDs = nil // reseed the form editors from the rewritten values
	s.pendingOpen = false
	s.pendingMerchantFilter = ""
	s.resetCharacterViewState()
	s.refreshResetVanillaAvailability()
}

// resetCharacterViewState clears the Characters view's state after any
// successful save/load. Staged flag edits must read as gone immediately
// -- the shared "Pending (N)" footer is visible from every view, so
// it can't wait for ensureCharList to notice a path change, which only
// happens while the Characters view itself is being laid out. Clearing
// charDataPath also makes ensureCharList redo its own full reset
// (CharList, SelectedChar, etc.) the next time that view renders, so a
// combined save started from the Shop Editor doesn't leave the Characters
// view pointed at stale, pre-save character data.
func (s *State) resetCharacterViewState() {
	s.charDataPath = ""
	s.clearBulkUnlockUndo()
}

// --- Reset to Vanilla (see staging.go's ResetToVanilla; settings.go's
// button, window.go's confirm-dialog overlay) ---

// refreshResetVanillaAvailability recomputes whether the currently loaded
// save has ANY row differing from vanilla -- gates the Settings button
// itself (disabled when nothing differs from vanilla). Called from
// consumeReset right after a fresh load or a completed save/apply, both
// points where the catalog's on-disk-backed row values can actually change;
// cheap enough (a few hundred µs over ~1300 rows) to just eagerly recompute
// rather than trying to track dirtiness incrementally.
func (s *State) refreshResetVanillaAvailability() {
	if !s.Catalog.Loaded() {
		s.resetVanillaAvailable = false
		return
	}
	diffs, err := s.Catalog.VanillaDiffs()
	s.resetVanillaAvailable = err == nil && len(diffs) > 0
}

// openResetVanillaConfirm computes how many rows currently differ from
// vanilla (for the confirm dialog's body text) and opens the confirm
// dialog. A no-op with no save loaded -- the Settings button is disabled
// in that case, but this is the authoritative guard.
func (s *State) openResetVanillaConfirm() {
	if !s.Catalog.Loaded() {
		return
	}
	diffs, err := s.Catalog.VanillaDiffs()
	if err != nil {
		s.reportError("compute Reset to Vanilla diff", err)
		return
	}
	s.resetVanillaDiffCount = len(diffs)
	s.resetVanillaConfirmOpen = true
}

// doResetToVanilla closes the confirm dialog and stages the reset (see
// ResetToVanilla's own doc for exactly what that does and doesn't touch).
// Recomputing the diff here (rather than reusing openResetVanillaConfirm's
// count) keeps the two calls independent and correct even in the
// (currently impossible, single-goroutine-UI) case the catalog changed
// between the two clicks -- the cost is negligible either way.
func (s *State) doResetToVanilla() {
	s.resetVanillaConfirmOpen = false
	if _, err := s.ResetToVanilla(); err != nil {
		s.reportError("Reset to Vanilla", err)
	}
}

// --- save-as / apply flow ---

// combinedPendingCount is the shared footer's "Pending (N)" count: staged
// item edits plus every character's staged flag edits, of either kind (one
// save button, reachable from any view, saves both at once).
func (s *State) combinedPendingCount() int {
	return s.PendingCount() + s.totalPendingFlagCount() + s.totalPendingBellBearingCount()
}

// startCombinedSave resolves whatever's staged (item edits, character-flag
// edits, or both) up front on the UI goroutine, then shows ONE native
// Save-As dialog and hands the result to combinedApplyWorker -- one save
// button that always works, regardless of which kind of edit (or both) is
// staged.
func (s *State) startCombinedSave() {
	if s.Busy() || s.combinedPendingCount() == 0 {
		return
	}
	// Mark busy for the WHOLE flow, not just the write itself. The
	// itemEdits/equipParamEdits/flagTargets snapshot built below is a
	// point-in-time copy of PendingEdits/PendingFlagEdits/
	// PendingBellBearingEdits -- if busy only became true once the native
	// Save-As dialog returned a path (as before), every Busy()-gated
	// staging control (merchant grid, flag checkboxes, ...) stayed
	// interactive for the whole time the dialog sat open, so edits staged
	// during that window never entered the snapshot and were silently
	// discarded when combinedApplyWorker's write completed and
	// consumeReset cleared PendingEdits. Every early return below must
	// call s.clearBusy() (combinedApplyWorker clears it itself on
	// completion, success or failure).
	s.mu.Lock()
	s.busy = true
	s.busyMsg = "Preparing save..."
	s.applyErr = ""
	s.mu.Unlock()

	plan, err := s.Session.PrepareSave()
	if err != nil {
		s.clearBusy()
		s.reportError("prepare save", err)
		return
	}

	suggested := s.SuggestedOutPath()
	if !s.setDialogOpen(true) {
		s.clearBusy()
		return
	}
	go func() {
		defer s.setDialogOpen(false)
		outPath, err := s.dialogs.SaveAs(suggested)
		if errors.Is(err, dialogs.ErrCanceled) || outPath == "" {
			s.clearBusy()
			s.invalidate()
			return
		}
		if err != nil {
			s.clearBusy()
			s.reportError("save dialog", err)
			return
		}
		s.mu.Lock()
		s.busyMsg = "Saving..."
		s.mu.Unlock()
		s.combinedApplyWorker(plan, outPath)
	}()
}

// combinedApplyWorker writes itemEdits (ShopLineupParam, via
// Catalog.ApplyEdits/savefile), flagTargets (character-slot event
// flags, via character), and equipParamEdits (an item's own
// EquipParam* sellValue -- see BuildEquipParamEdits's doc comment for why,
// via savefile.ApplyWithSchema) to outPath, in whichever combination is
// non-empty. Each kind is an independently optional PIPELINE STAGE, run in
// a fixed order (flags -> sellValue, one stage per distinct EquipParam*
// table touched -> item edits) chained through numbered ".tmpN" files
// (never overwriting the currently-loaded input, same safety rule every
// individual write already enforces on its own): stage N's output becomes
// stage N+1's input, and only the LAST stage that actually runs writes
// directly to outPath. This generalizes what was a fixed 2-stage
// (flags-then-items) switch before sellValue edits existed; every
// individual write function is untouched.
//
// An EditError from ApplyEdits stays a routine inline dropdown error
// (nothing was written); any other stage's failure is unexpected/internal
// (e.g. a round-trip self-check) and goes through the same reportError
// modal path applyWorker's item-edit-only case already used.
func (s *State) combinedApplyWorker(plan *application.SavePlan, outPath string) {
	err := plan.Apply(outPath)

	var editErr *catalog.EditError
	s.mu.Lock()
	s.busy = false
	switch {
	case err == nil:
		s.applyErr = ""
		s.loadedName = filepath.Base(s.Catalog.SavePath())
		s.applyDone = true
	case errors.As(err, &editErr):
		// Routine validation failure — inline text, nothing was written.
		s.applyErr = editErr.Error()
	default:
		s.mu.Unlock()
		s.reportError("save file", err)
		s.invalidate()
		return
	}
	s.mu.Unlock()
	s.invalidate()
}

// --- native open dialog ---

// setDialogOpen toggles the dialog guard, returning false if a dialog is
// already open (so the caller should not open a second one).
func (s *State) setDialogOpen(open bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if open && s.dialogOpen {
		return false
	}
	s.dialogOpen = open
	return true
}

// setInlineErr records a save-switcher error and requests a redraw.
func (s *State) setInlineErr(msg string) {
	s.mu.Lock()
	s.inlineErr = msg
	s.mu.Unlock()
	s.invalidate()
}
