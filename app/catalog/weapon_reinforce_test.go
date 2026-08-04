package catalog

// CI-runnable (no fixture): weapon_reinforce.json is embedded static data
// (generated once by tools/weapon_reinforce_extract from our own fixture's
// regulation.bin, see docs/ITEM_IDS.md), not decoded per-save.

import "testing"

// TestMaxUpgradeLevelKnownWeapons cross-checks a handful of items whose real
// in-game max reinforcement level is well known: standard/Smithing-Stone
// weapons cap at +25, somber/Somber-Smithing-Stone weapons (bows, staves,
// seals, and most legendary/boss-drop melee weapons) cap at +10. Catches a
// wrong reinforceTypeId/ReinforceParamWeapon mapping immediately instead of
// only in a live game test.
func TestMaxUpgradeLevelKnownWeapons(t *testing.T) {
	items, _, err := loadItems()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]int64{}
	for _, it := range items {
		byName[it.Name] = it.ID
	}

	weaponMaxLvl, err := loadWeaponReinforce()
	if err != nil {
		t.Fatal(err)
	}
	c := &Catalog{itemByID: nil, weaponMaxLvl: weaponMaxLvl}

	cases := []struct {
		name string
		want int
	}{
		{"Zweihander", 25},      // standard melee
		{"Uchigatana", 25},      // standard melee
		{"Moonveil", 10},        // legendary melee, somber
		{"Rivers of Blood", 10}, // legendary melee, somber
		{"Wing of Astel", 10},   // legendary melee, somber
		{"Marika's Hammer", 10}, // legendary melee, somber
		{"Erdtree Bow", 10},     // bow, always somber
		{"Finger Seal", 25},     // sacred seal (confirmed: not all seals are somber)
	}
	for _, tc := range cases {
		id, ok := byName[tc.name]
		if !ok {
			t.Errorf("%s: not found in items.json", tc.name)
			continue
		}
		got, ok := c.MaxUpgradeLevel(id)
		if !ok {
			t.Errorf("%s (id %d): MaxUpgradeLevel not found", tc.name, id)
			continue
		}
		if got != tc.want {
			t.Errorf("%s (id %d): MaxUpgradeLevel = %d, want %d", tc.name, id, got, tc.want)
		}
	}
}

// TestMaxUpgradeLevelCoversEveryWeaponItem: every items.json entry in a
// weapon-table category (melee_armaments/shields/ranged_and_catalysts) must
// resolve a max level -- a gap here means tools/weapon_reinforce_extract
// missed a row.
func TestMaxUpgradeLevelCoversEveryWeaponItem(t *testing.T) {
	items, _, err := loadItems()
	if err != nil {
		t.Fatal(err)
	}
	weaponMaxLvl, err := loadWeaponReinforce()
	if err != nil {
		t.Fatal(err)
	}
	c := &Catalog{weaponMaxLvl: weaponMaxLvl}

	weaponCategories := map[string]bool{"melee_armaments": true, "shields": true, "ranged_and_catalysts": true}
	missing := 0
	for _, it := range items {
		if !weaponCategories[it.Category] {
			continue
		}
		if _, ok := c.MaxUpgradeLevel(it.ID); !ok {
			t.Errorf("%s (id %d, category %s): no weapon_reinforce.json entry", it.Name, it.ID, it.Category)
			missing++
		}
	}
	if missing == 0 {
		t.Logf("all weapon-table items.json entries have a MaxUpgradeLevel entry")
	}

	// Non-weapon items must never resolve (armor/talismans/goods/AoW aren't
	// in EquipParamWeapon at all).
	for _, it := range items {
		if weaponCategories[it.Category] {
			continue
		}
		if lvl, ok := c.MaxUpgradeLevel(it.ID); ok {
			t.Errorf("%s (id %d, category %s): unexpectedly has a MaxUpgradeLevel (%d)", it.Name, it.ID, it.Category, lvl)
		}
	}
}

// TestResolveItemIDWithLevelResolvesLeveledWeapons is the regression test
// for a real user-reported bug (2026-08-02): reloading a save with a
// weapon-level ("+N") edit showed a blank icon and the row's stale
// pre-edit label in the merchant grid, because items.json only indexes
// +0 base weapon ids -- a leveled equipId (base+N) never resolved at all.
func TestResolveItemIDWithLevelResolvesLeveledWeapons(t *testing.T) {
	items, byID, err := loadItems()
	if err != nil {
		t.Fatal(err)
	}
	weaponMaxLvl, err := loadWeaponReinforce()
	if err != nil {
		t.Fatal(err)
	}
	c := &Catalog{itemByID: byID, weaponMaxLvl: weaponMaxLvl}

	byName := map[string]*Item{}
	for _, it := range items {
		byName[it.Name] = it
	}

	zweihander := byName["Zweihander"] // standard, max +25
	moonveil := byName["Moonveil"]     // somber, max +10
	if zweihander == nil || moonveil == nil {
		t.Fatal("fixture items not found in items.json")
	}

	cases := []struct {
		name      string
		rawID     int64
		wantBase  int64
		wantLevel int64
		wantOK    bool
	}{
		{"Zweihander +0 (direct hit, no fallback needed)", zweihander.ID, zweihander.ID, 0, true},
		{"Zweihander +12", zweihander.ID + 12, zweihander.ID, 12, true},
		{"Zweihander +25 (max)", zweihander.ID + 25, zweihander.ID, 25, true},
		{"Zweihander +26 (past real max, must reject)", zweihander.ID + 26, 0, 0, false},
		{"Moonveil +10 (max)", moonveil.ID + 10, moonveil.ID, 10, true},
		{"Moonveil +11 (past real max, must reject)", moonveil.ID + 11, 0, 0, false},
		{"bogus id near a real base but not a real item", zweihander.ID + 9999, 0, 0, false},
	}
	for _, tc := range cases {
		gotBase, gotLevel, ok := c.resolveItemIDWithLevel(tc.rawID, 0)
		if ok != tc.wantOK {
			t.Errorf("%s: ok = %v, want %v", tc.name, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if gotBase != tc.wantBase || gotLevel != tc.wantLevel {
			t.Errorf("%s: got base=%d level=%d, want base=%d level=%d", tc.name, gotBase, gotLevel, tc.wantBase, tc.wantLevel)
		}
	}

	// Non-weapon categories must never take the leveled fallback (only
	// weapons have reinforcement at all).
	if _, _, ok := c.resolveItemIDWithLevel(zweihander.ID+12, 1); ok {
		t.Error("category 1 (Protector) unexpectedly resolved via the weapon-level fallback")
	}
}
