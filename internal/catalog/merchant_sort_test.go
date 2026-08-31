package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/savefile"
)

// TestMerchantSortKeyOrdersGroups checks the actual Twin Maiden Husks Bell
// Bearing Shop grouping: Shop 1 specialist sellers, Shop 2's named general
// sellers, Shop 3 Nomadic merchants, Shop 4's combined non-Nomadic wandering
// merchants, then Shop 5 DLC sellers. Dragon Communion remains a final
// real-world-order block (Church/Cathedral/Grand Altar).
func TestMerchantSortKeyOrdersGroups(t *testing.T) {
	names := []string{
		"Grand Altar of Dragon Communion",
		"Abandoned Merchant - Siofra River",
		"Count Ymir",
		"Imprisoned Merchant - Mohgwyn",
		"Hermit Merchant - Leyndell",
		"Isolated Merchant - Dragonbarrow",
		"Nomadic Merchant - Altus Plateau",
		"Sorceress Sellen",
		"Twin Maiden Husks",
		"Cathedral of Dragon Communion",
		"Patches",
		"Pidia Carian Servant",
		"Gatekeeper Gostoc",
		"Iji",
		"Thiollier",
		"Church of Dragon Communion",
	}
	wantOrder := []string{
		"Twin Maiden Husks",
		"Sorceress Sellen",
		"Iji",
		"Gatekeeper Gostoc",
		"Pidia Carian Servant",
		"Patches",
		"Nomadic Merchant - Altus Plateau",
		"Isolated Merchant - Dragonbarrow",
		"Hermit Merchant - Leyndell",
		"Abandoned Merchant - Siofra River",
		"Imprisoned Merchant - Mohgwyn",
		"Thiollier",
		"Count Ymir",
		"Church of Dragon Communion", // altar block, real-world order not alphabetical
		"Cathedral of Dragon Communion",
		"Grand Altar of Dragon Communion",
	}
	sort.Slice(names, func(i, j int) bool {
		gi, ni := MerchantSortKey(names[i])
		gj, nj := MerchantSortKey(names[j])
		if gi != gj {
			return gi < gj
		}
		return ni < nj
	})
	for i, name := range names {
		if name != wantOrder[i] {
			t.Fatalf("MerchantSortKey order[%d] = %q, want %q\ngot:  %v\nwant: %v", i, name, wantOrder[i], names, wantOrder)
		}
	}
}

// TestMerchantSortKeyOrdersBellBearingFamilies keeps generic merchant filters
// in Shop 3's bearing-number order, followed by Shop 4's internal family
// order (Isolated before Hermit in this sample).
func TestMerchantSortKeyOrdersBellBearingFamilies(t *testing.T) {
	names := []string{
		"Nomadic Merchant - South Caelid",
		"Nomadic Merchant - Altus Plateau",
		"Nomadic Merchant - North Limgrave",
		"Nomadic Merchant - Coastal Cave",
		"Isolated Merchant - Dragonbarrow",
		"Isolated Merchant - Weeping Peninsula",
		"Hermit Merchant - Ainsel River",
		"Hermit Merchant - Leyndell",
	}
	wantOrder := []string{
		"Nomadic Merchant - North Limgrave",
		"Nomadic Merchant - Coastal Cave",
		"Nomadic Merchant - Altus Plateau",
		"Nomadic Merchant - South Caelid",
		"Isolated Merchant - Weeping Peninsula",
		"Isolated Merchant - Dragonbarrow",
		"Hermit Merchant - Leyndell",
		"Hermit Merchant - Ainsel River",
	}
	sort.Slice(names, func(i, j int) bool {
		gi, ni := MerchantSortKey(names[i])
		gj, nj := MerchantSortKey(names[j])
		if gi != gj {
			return gi < gj
		}
		return ni < nj
	})
	for i, name := range names {
		if name != wantOrder[i] {
			t.Fatalf("MerchantSortKey bearing order[%d] = %q, want %q", i, name, wantOrder[i])
		}
	}
}

func TestRegulation117MerchantSlots(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]string{
		100568: "Nomadic Merchant - East Limgrave",
		100666: "Isolated Merchant - Weeping Peninsula",
		100667: "Isolated Merchant - Weeping Peninsula",
		100668: "Isolated Merchant - Weeping Peninsula",
		100669: "Isolated Merchant - Weeping Peninsula",
		100709: "Nomadic Merchant - North Liurnia",
		100710: "Nomadic Merchant - North Liurnia",
		100711: "Nomadic Merchant - North Liurnia",
		100712: "Nomadic Merchant - North Liurnia",
		100713: "Nomadic Merchant - North Liurnia",
		101896: "Twin Maiden Husks",
	}
	for rowID, merchant := range want {
		cc := c.canonicalFor(rowID)
		if cc.Kind != "merchant" || cc.Merchant != merchant || !isBrowsable(cc) {
			t.Errorf("row %d = %#v, want browsable merchant %q", rowID, cc, merchant)
		}
	}
	for _, rowID := range []int64{110084, 111084, 110085, 111085, 110284, 111284, 110285, 111285} {
		cc := c.canonicalFor(rowID)
		if cc.Kind != "special_exchange" || isBrowsable(cc) {
			t.Errorf("tailoring row %d = %#v, want non-browsable special exchange", rowID, cc)
		}
	}
}

// TestMerchantRowsKeepEditLayoutOrderAfterItemSwap keeps the editor's default
// layout stable while a user is assembling a merchant. The separate UI Game
// Preview applies the in-game sort only when explicitly requested.
func TestMerchantRowsKeepEditLayoutOrderAfterItemSwap(t *testing.T) {
	if _, err := os.Stat(fixtureSave); err != nil {
		t.Skipf("fixture save absent, skipping: %v", err)
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.LoadSave(fixtureSave); err != nil {
		t.Fatal(err)
	}
	rows, err := c.MerchantRows("Twin Maiden Husks")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("Twin Maiden Husks rows = %d, want at least 2", len(rows))
	}
	before := make([]int64, len(rows))
	for i, row := range rows {
		before[i] = row.RowID
	}

	out := filepath.Join(t.TempDir(), "tmh-slot-order.dat")
	target := rows[len(rows)-1]
	edits := []savefile.Edit{{RowID: target.RowID, Fields: map[string]json.Number{
		"equipId": "190", "equipType": "3", // Rune Arc
	}}}
	if _, err := c.ApplyEdits(edits, out); err != nil {
		t.Fatal(err)
	}
	after, err := c.MerchantRows("Twin Maiden Husks")
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range after {
		if row.RowID != before[i] {
			t.Fatalf("edit-layout order changed after save at index %d: row %d, want row %d", i, row.RowID, before[i])
		}
	}
	if got := after[len(after)-1].ItemName; got != "Rune Arc" {
		t.Fatalf("target item after reload = %q, want Rune Arc", got)
	}
	index := make(map[int64]int, len(after))
	for i, row := range after {
		index[row.RowID] = i
	}
	if index[target.RowID] != len(after)-1 {
		t.Fatalf("target row %d index = %d, want final edit-layout slot", target.RowID, index[target.RowID])
	}
}
