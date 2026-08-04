package catalog

// CI-runnable (no fixture): the equipId<->item-id conversion must round-trip
// over every entry in the embedded items.json.

import (
	"strings"
	"testing"
)

func TestEquipRefRoundTrip(t *testing.T) {
	items, byID, err := loadItems()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("no items loaded")
	}

	resolvable, unresolvable := 0, 0
	for _, it := range items {
		et, eid, ok := EquipRefForItemID(it.ID)
		if !ok {
			// savescan considers this unresolvable: resolveItemID must agree.
			if _, ok := resolveItemID(0, -1, byID); ok {
				t.Fatalf("item %d: unexpected resolveItemID success on unresolvable ref", it.ID)
			}
			unresolvable++
			continue
		}
		got, ok := resolveItemID(eid, et, byID)
		if !ok {
			t.Errorf("item %d (%s): resolveItemID(%d, %d) failed to resolve back", it.ID, it.Name, eid, et)
			continue
		}
		if got != it.ID {
			t.Errorf("item %d (%s): round-trip = %d (equipType=%d equipId=%d)", it.ID, it.Name, got, et, eid)
		}
		resolvable++
	}
	t.Logf("round-tripped %d items (%d unresolvable)", resolvable, unresolvable)
}

// TestMerchantMenuSortDirection pins the in-game direction observed in the
// Twin Maiden Husks menu: within the same sort group a smaller sortId is shown
// first. These three consecutive Goods sort IDs make a compact, unambiguous
// regression fixture.
func TestMerchantMenuSortDirection(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	const (
		blessing = int64(1075742724) // Blessing of Marika, sortId 540
		shards   = int64(1073743114) // Starlight Shards, sortId 530
		runeArc  = int64(1073742014) // Rune Arc, sortId 520
	)
	if !c.ItemComesBeforeInMerchantMenu(runeArc, shards) ||
		!c.ItemComesBeforeInMerchantMenu(shards, blessing) {
		t.Fatal("merchant menu must order Rune Arc, Starlight Shards, Blessing of Marika")
	}
	if c.ItemComesBeforeInMerchantMenu(blessing, shards) ||
		c.ItemComesBeforeInMerchantMenu(shards, runeArc) {
		t.Fatal("merchant menu sort direction must be ascending sortId within a group")
	}
}

// TestHiddenItemsResolvableButNotListed: hidden items.json entries (internal
// same-name variants, e.g. Flask of Wondrous Physick 0x400000FB; and, since
// 2026-08-01, all 3 visible Flask items too -- infinite-heal exploit risk,
// see docs/ITEM_IDS.md) must stay resolvable by id (shop-row enrichment)
// while never appearing in the browsable list. A genuinely non-hidden item
// (Golden Rune [1], a plain always-visible tools/Golden Runes entry) is the
// control case confirming the browsable list isn't just empty.
func TestHiddenItemsResolvableButNotListed(t *testing.T) {
	items, byID, err := loadItems()
	if err != nil {
		t.Fatal(err)
	}
	const hiddenFlask, otherHiddenFlask, controlVisible = 0x400000FA, 0x400000FB, 0x40000B54
	if byID[hiddenFlask] == nil || byID[otherHiddenFlask] == nil || byID[controlVisible] == nil {
		t.Fatal("flask/control ids missing from byID")
	}
	listed := map[int64]bool{}
	for _, it := range items {
		listed[it.ID] = true
	}
	if listed[hiddenFlask] {
		t.Error("hidden Flask of Wondrous Physick appears in the browsable list")
	}
	if listed[otherHiddenFlask] {
		t.Error("hidden flask variant appears in the browsable list")
	}
	if !listed[controlVisible] {
		t.Error("control item (Golden Rune [1]) missing from the browsable list")
	}
}

// TestListItemsOrdersByGameSortOrder (2026-08-01, item catalog reorganization):
// ListItems must order items.json's "tools" category to match the game's own
// menu order (item_sort_order.json's real sortId, grouped by sub-category --
// see buildSubCategoryRank), not raw item-id order. Plain "Fire Grease" must
// come before every "Drawstring X Grease" variant (the user's own example:
// "drawstring greases must be all near to each other"), and every item in
// the category must still be present -- reordering must never drop items.
func TestListItemsOrdersByGameSortOrder(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	items := c.ListItems("tools", "", "", nil)
	if len(items) == 0 {
		t.Fatal("no tools items returned")
	}

	// Every plain "Fire Grease" must be positioned before every "Drawstring"
	// variant within the Grease sub-category -- confirms both the item-level
	// sortId ordering and the sub-category-stays-contiguous grouping.
	lastPlainGrease, firstDrawstring := -1, -1
	for i, it := range items {
		if it.SubCategory != "Grease" {
			continue
		}
		if it.Name == "Fire Grease" {
			lastPlainGrease = i
		}
		if it.Name == "Drawstring Fire Grease" && firstDrawstring < 0 {
			firstDrawstring = i
		}
	}
	if lastPlainGrease < 0 || firstDrawstring < 0 {
		t.Fatal("expected both Fire Grease and Drawstring Fire Grease in the tools category")
	}
	if lastPlainGrease >= firstDrawstring {
		t.Errorf("Fire Grease (index %d) must come before Drawstring Fire Grease (index %d)", lastPlainGrease, firstDrawstring)
	}

	// The full items.json tools set (via loadItems, unsorted) must be exactly
	// the same set ListItems returns, just reordered -- no item lost/added.
	all, _, err := loadItems()
	if err != nil {
		t.Fatal(err)
	}
	wantCount := 0
	for _, it := range all {
		if it.Category == "tools" {
			wantCount++
		}
	}
	if len(items) != wantCount {
		t.Errorf("ListItems returned %d tools items, want %d", len(items), wantCount)
	}
}

// TestListItemsGroupsBySubCategoryInFilterOrder locks in the 2026-08-03 fix:
// the unfiltered grid must read as the sub-category filter list in order,
// one CONTIGUOUS block per sub-category. Before the fix the primary sort key
// was the game's raw sortGroupId, which interleaved sub-categories that span
// several groups -- key items' "Quest Items" got split around "Great Runes",
// leaving only Memory of Grace under the first heading (user-reported).
//
// Checks several categories, not just the reported one, since the same
// span-multiple-groups shape exists elsewhere.
func TestListItemsGroupsBySubCategoryInFilterOrder(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range []string{"key_items", "tools", "melee_armaments", "armor"} {
		want := c.ListSubcategories(category)
		if len(want) == 0 {
			continue // category has no sub-categories (nothing to group)
		}

		// Walk the grid, recording each sub-category the moment it starts a
		// new block. A sub-category seen twice means a non-contiguous block.
		var got []string
		seen := map[string]bool{}
		last := ""
		for _, it := range c.ListItems(category, "", "", nil) {
			if it.SubCategory == last {
				continue
			}
			last = it.SubCategory
			if last == "" {
				continue // items with no sub-category aren't in the filter list
			}
			if seen[last] {
				t.Errorf("%s: sub-category %q appears in more than one block "+
					"(grid must group each sub-category contiguously)", category, last)
			}
			seen[last] = true
			got = append(got, last)
		}

		if len(got) != len(want) {
			t.Errorf("%s: grid has %d sub-category blocks, filter lists %d\n grid=%v\n filter=%v",
				category, len(got), len(want), got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: block %d is %q, filter dropdown has %q at that position\n grid=%v\n filter=%v",
					category, i, got[i], want[i], got, want)
				break
			}
		}
	}
}

// TestItemByIDIncludesHidden: ItemByID must resolve hidden items, which
// ListItems deliberately omits. Hidden items are excluded from the browsable
// grid but are still referenced by real shop rows (e.g. Twin Maiden Husks'
// Flask of Wondrous Physick), so anything reached from such a row -- the
// item-info popup above all -- has to resolve them. Regression test for the
// 2026-08-03 report that right-clicking one opened nothing.
func TestItemByIDIncludesHidden(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// loadItems' FIRST return already excludes hidden entries (that's the
	// browsable list ListItems serves from); its byID map is the one that
	// keeps them, so the hidden set is byID minus the listed ids.
	_, byID, err := loadItems()
	if err != nil {
		t.Fatal(err)
	}
	listed := map[int64]bool{}
	for _, it := range c.ListItems("", "", "", nil) {
		listed[it.ID] = true
	}
	hidden := 0
	for id, it := range byID {
		if listed[id] {
			continue
		}
		hidden++
		if c.ItemByID(id) == nil {
			t.Errorf("ItemByID(%d) = nil for hidden item %q, want it resolvable", id, it.Name)
		}
	}
	if hidden == 0 {
		t.Fatal("no hidden items found -- test can't prove anything (items.json shape changed?)")
	}
	if c.ItemByID(-1) != nil {
		t.Error("ItemByID(-1) should be nil for an unknown id")
	}
}

// TestAlteredArmorInheritsRisky guards a data invariant that regressed once
// (user-reported 2026-08-03): SaveForge flags Ragged Hat as cut content but
// not Ragged Hat (Altered), even though they're the same cut armor. The
// generator now derives the Altered half's flag from its base piece
// (tools/itemdb_extract's propagateRiskyToAltered); this asserts the shipped
// data actually carries it, so a regenerate can't silently drop it.
func TestAlteredArmorInheritsRisky(t *testing.T) {
	_, byID, err := loadItems()
	if err != nil {
		t.Fatal(err)
	}
	type key struct{ category, name string }
	base := make(map[key]*Item)
	for _, it := range byID {
		if !strings.HasSuffix(it.Name, " (Altered)") {
			base[key{it.Category, it.Name}] = it
		}
	}
	checked := 0
	for _, it := range byID {
		stem, ok := strings.CutSuffix(it.Name, " (Altered)")
		if !ok {
			continue
		}
		b, ok := base[key{it.Category, stem}]
		if !ok {
			t.Errorf("%q has no base piece in category %q", it.Name, it.Category)
			continue
		}
		checked++
		if b.Risky && !it.Risky {
			t.Errorf("%q is risky but %q is not -- same armor, same warning", b.Name, it.Name)
		}
	}
	if checked == 0 {
		t.Fatal("no (Altered) armor pieces found -- test is vacuous")
	}
}

// TestRiskyOverrides guards the two manually-audited exceptions to
// SaveForge's cut-content flags: Erdtree Prayerbook exists only in game data,
// while Deathbed Dress is a legitimate Leyndell pickup (unlike Deathbed
// Smalls, which remains risky).
func TestRiskyOverrides(t *testing.T) {
	_, byID, err := loadItems()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id    int64
		name  string
		risky bool
	}{
		{0x4000229C, "Erdtree Prayerbook", true},
		{0x101D7374, "Deathbed Dress", false},
		{0x101D743C, "Deathbed Smalls", true},
	} {
		it := byID[tc.id]
		if it == nil {
			t.Errorf("%s (0x%X) missing from items.json", tc.name, tc.id)
			continue
		}
		if it.Risky != tc.risky {
			t.Errorf("%s risky = %t, want %t", tc.name, it.Risky, tc.risky)
		}
	}
}
