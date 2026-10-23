package gio

// Tests for clampPickLevel (pure clamp) and orderedCategoryOptions (curated
// order + unknown-category fallback + label resolution).

import (
	"reflect"
	"testing"
)

// TestClampPickLevel covers the shared weapon-level control's range clamp.
func TestClampPickLevel(t *testing.T) {
	cases := []struct {
		in, want int64
	}{
		{-5, 0},
		{0, 0},
		{10, 10},
		{pickLevelMax, pickLevelMax},
		{pickLevelMax + 1, pickLevelMax},
		{100, pickLevelMax},
	}
	for _, c := range cases {
		if got := clampPickLevel(c.in); got != c.want {
			t.Errorf("clampPickLevel(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestOrderedCategoryOptions covers the curated categoryOrder ranking, the
// leading "" / "All Categories" entry, friendly-label resolution, and the
// fallback for a category not in categoryOrder (sorts alphabetically AFTER
// every known one, with the raw id as its own label so it can't silently
// disappear).
func TestOrderedCategoryOptions(t *testing.T) {
	// Deliberately out of curated order, with two unknown categories.
	cats := []string{"talismans", "melee_armaments", "zzz_new_category", "head", "aaa_other_new"}

	options, labels := orderedCategoryOptions(cats)

	wantOptions := []string{
		"",                // All
		"melee_armaments", // rank 8
		"head",            // rank 12
		"talismans",       // rank 16
		"aaa_other_new",   // unknown -> after known, alphabetical
		"zzz_new_category",
	}
	if !reflect.DeepEqual(options, wantOptions) {
		t.Errorf("options = %v, want %v", options, wantOptions)
	}

	wantLabels := []string{
		"All Categories",
		"Melee Weapons",
		"Head Armor",
		"Talismans",
		"aaa_other_new",    // no friendly label -> raw id fallback
		"zzz_new_category", // no friendly label -> raw id fallback
	}
	if !reflect.DeepEqual(labels, wantLabels) {
		t.Errorf("labels = %v, want %v", labels, wantLabels)
	}
}

// TestCategoryOrderMatchesGameMenu locks the single shared order used by the
// Catalog filter/grid and Merchant Game Preview. Keep this list in the same
// order as Elden Ring's menu categories.
func TestCategoryOrderMatchesGameMenu(t *testing.T) {
	want := []string{
		"tools", "ashes", "crafting_materials", "bolstering_materials", "key_items",
		"sorceries", "incantations", "ashes_of_war",
		"melee_armaments", "ranged_and_catalysts", "arrows_and_bolts", "shields",
		"head", "chest", "arms", "legs", "talismans", "info",
	}
	if !reflect.DeepEqual(categoryOrder, want) {
		t.Errorf("categoryOrder = %v, want %v", categoryOrder, want)
	}
}
