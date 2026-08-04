package gio

import (
	"testing"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/application"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
)

// newTestState creates the smallest State that exercises code backed by the
// application session. Tests concerned only with isolated widget helpers may
// still use &State{} directly.
func newTestState(cat *catalog.Catalog) *State {
	return &State{Session: application.NewSession(cat)}
}

func applyPendingForTest(t *testing.T, state *State, outPath string) {
	t.Helper()
	plan, err := state.Session.PrepareSave()
	if err != nil {
		t.Fatalf("PrepareSave: %v", err)
	}
	state.combinedApplyWorker(plan, outPath)
}
