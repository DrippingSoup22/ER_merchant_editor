package gio

// Tests for the pending-edits merchant grouping: pendingMerchantKey's
// "Unknown merchant" fallback and groupPendingByMerchant's name-sorted,
// stable-within-group bucketing.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
)

func TestFooterStatusTracksCurrentState(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState(cat)
	if got, want := s.footerStatusMessage(), "Open a save file to begin"; got != want {
		t.Errorf("unloaded status = %q, want %q", got, want)
	}

	s.inlineErr = "bad save"
	if got := s.footerStatusMessage(); !strings.Contains(got, "check the message above") {
		t.Errorf("load-error status = %q, want guidance to the inline error", got)
	}

	s.busy, s.busyMsg = true, "Loading save..."
	if got, want := s.footerStatusMessage(), "Loading save..."; got != want {
		t.Errorf("busy status = %q, want %q", got, want)
	}
}

func TestFooterStatusUpdatesWithPendingCount(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.loadedName = "fixture.dat"
	if got := s.footerStatusMessage(); !strings.Contains(got, "original stays unchanged") {
		t.Errorf("loaded status = %q, want save-copy safety guidance", got)
	}

	s.PendingEdits[1] = &RowEdit{}
	if got := s.footerStatusMessage(); !strings.HasPrefix(got, "1 change staged") {
		t.Errorf("one-change status = %q", got)
	}
	s.PendingEdits[2] = &RowEdit{}
	if got := s.footerStatusMessage(); !strings.HasPrefix(got, "2 changes staged") {
		t.Errorf("two-change status = %q", got)
	}

	s.pendingOpen = true
	if got := s.footerStatusMessage(); !strings.HasPrefix(got, "Review 2 staged changes") {
		t.Errorf("pending-review status = %q", got)
	}

	s.pendingOpen = false
	s.StartPicking([]int64{10, 11})
	if got := s.footerStatusMessage(); !strings.Contains(got, "2 items remaining") {
		t.Errorf("picker status = %q", got)
	}

	s.PickingForRows = nil
	s.itemInfoOpen = true
	if got := s.footerStatusMessage(); got != "" {
		t.Errorf("item-info status = %q, want no unrelated footer overlay", got)
	}
}

func TestFooterNoticeAndImmediateGuidancePriority(t *testing.T) {
	s := NewState(loadedTestCatalog(t))
	s.setFooterNotice("Opened Settings — customize appearance and editing defaults")
	if got := s.footerStatusMessage(); got != s.footerNotice {
		t.Fatalf("event notice = %q, want %q", got, s.footerNotice)
	}

	s.StartPicking([]int64{10})
	if got := s.footerStatusMessage(); !strings.Contains(got, "Choose a replacement") {
		t.Errorf("picker status = %q, want immediate picker instructions", got)
	}

	s.PickingForRows = nil
	s.busy, s.busyMsg = true, "Saving copy..."
	if got, want := s.footerStatusMessage(), "Saving copy..."; got != want {
		t.Errorf("busy status = %q, want %q", got, want)
	}
}

func TestViewOpenedMessagesExplainNextAction(t *testing.T) {
	tests := map[string]string{
		"Characters":  "choose a character",
		"Shop Editor": "select stock",
		"Settings":    "customize appearance",
	}
	for view, nextAction := range tests {
		if got := viewOpenedMessage(view); !strings.Contains(got, nextAction) {
			t.Errorf("viewOpenedMessage(%q) = %q, want next-action guidance containing %q", view, got, nextAction)
		}
	}
}

// TestPendingMerchantKey covers the single place the "" -> "Unknown merchant"
// fallback is decided.
func TestPendingMerchantKey(t *testing.T) {
	if got := pendingMerchantKey(&RowEdit{Merchant: ""}); got != "Unknown merchant" {
		t.Errorf("empty merchant key = %q, want %q", got, "Unknown merchant")
	}
	if got := pendingMerchantKey(&RowEdit{Merchant: "Merchant Kale"}); got != "Merchant Kale" {
		t.Errorf("named merchant key = %q, want %q", got, "Merchant Kale")
	}
}

// TestGroupPendingByMerchant covers name-sorted grouping, the "Unknown
// merchant" fallback bucket, stable ascending row order within a group, and
// skipping ids whose edit is missing from the map.
func TestGroupPendingByMerchant(t *testing.T) {
	edits := map[int64]*RowEdit{
		1: {Merchant: "Patches"},
		2: {Merchant: ""}, // -> "Unknown merchant"
		3: {Merchant: "Merchant Kale"},
		4: {Merchant: "Patches"},
		5: {Merchant: ""}, // -> "Unknown merchant"
		// id 6 is present in ids below but absent here: must be skipped.
	}
	ids := []int64{1, 2, 3, 4, 5, 6}

	groups := groupPendingByMerchant(edits, ids)

	want := []pendingMerchantGroup{
		{merchant: "Merchant Kale", ids: []int64{3}},
		{merchant: "Patches", ids: []int64{1, 4}},
		{merchant: "Unknown merchant", ids: []int64{2, 5}},
	}
	if len(groups) != len(want) {
		t.Fatalf("groups = %+v, want %d groups", groups, len(want))
	}
	for i := range want {
		if groups[i].merchant != want[i].merchant {
			t.Errorf("group %d name = %q, want %q (name-sorted)", i, groups[i].merchant, want[i].merchant)
		}
		if !reflect.DeepEqual(groups[i].ids, want[i].ids) {
			t.Errorf("group %q ids = %v, want %v (ascending, stable)", want[i].merchant, groups[i].ids, want[i].ids)
		}
	}
}

// TestPendingMerchantNames is groupPendingByMerchant's names-only projection,
// used for the filter combo -- same name-sorted order.
func TestPendingMerchantNames(t *testing.T) {
	edits := map[int64]*RowEdit{
		1: {Merchant: "Patches"},
		2: {Merchant: ""},
		3: {Merchant: "Merchant Kale"},
	}
	got := pendingMerchantNames(edits, []int64{1, 2, 3})
	want := []string{"Merchant Kale", "Patches", "Unknown merchant"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pendingMerchantNames = %v, want %v", got, want)
	}
}
