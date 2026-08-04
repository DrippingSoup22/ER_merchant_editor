package editor

// Tests for the pending-edits merchant grouping: pendingMerchantKey's
// "Unknown merchant" fallback and groupPendingByMerchant's name-sorted,
// stable-within-group bucketing.

import (
	"reflect"
	"testing"
)

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
