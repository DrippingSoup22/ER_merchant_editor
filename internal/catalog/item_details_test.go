package catalog

// CI-runnable (no fixture): item_details.json is embedded static data
// (generated alongside items.json by tools/itemdb_extract from SaveForge's
// own db.ItemEntry fields, see docs/ITEM_IDS.md), not decoded per-save.

import "testing"

// TestItemDetailsKnownItems cross-checks that a weapon/spell/armor item each
// resolve to their expected stat block kind -- catches a wrong field
// projection (e.g. Weapon/Spell/Armor swapped) immediately.
func TestItemDetailsKnownItems(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]int64{}
	for _, it := range c.itemList {
		byName[it.Name] = it.ID
	}

	weaponID, ok := byName["Zweihander"]
	if !ok {
		t.Fatal("Zweihander not found in items.json")
	}
	d, ok := c.ItemDetails(weaponID)
	if !ok {
		t.Fatal("no ItemDetails for Zweihander")
	}
	if d.Weapon == nil {
		t.Error("Zweihander: Weapon == nil, want a weapon stat block")
	} else if d.Weapon.ReqStr == 0 {
		t.Error("Zweihander: ReqStr == 0, want a real Strength requirement")
	}
	if d.Armor != nil || d.Spell != nil {
		t.Error("Zweihander: Armor/Spell should be nil for a weapon")
	}
	if d.Description == "" {
		t.Error("Zweihander: expected a non-empty description")
	}

	spellID, ok := byName["Comet Azur"]
	if !ok {
		t.Fatal("Comet Azur not found in items.json")
	}
	d, ok = c.ItemDetails(spellID)
	if !ok {
		t.Fatal("no ItemDetails for Comet Azur")
	}
	if d.Spell == nil {
		t.Error("Comet Azur: Spell == nil, want a spell stat block")
	} else if d.Spell.FPCost == 0 {
		t.Error("Comet Azur: FPCost == 0, want a real FP cost")
	}
	if d.Weapon != nil || d.Armor != nil {
		t.Error("Comet Azur: Weapon/Armor should be nil for a spell")
	}
}

func TestRegulation117AlteredArmorDetails(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id     int64
		weight float64
	}{
		{0x1051A6BC, 6.5},
		{0x1051CD68, 3.0},
	} {
		d, ok := c.ItemDetails(tc.id)
		if !ok || d.Armor == nil {
			t.Errorf("0x%08X: missing altered-armor details", tc.id)
			continue
		}
		if d.Weight != tc.weight {
			t.Errorf("0x%08X weight = %.1f, want %.1f", tc.id, d.Weight, tc.weight)
		}
	}
}

// Every catalog armor has a real EquipParamProtector row and must expose the
// complete regulation-derived popup block.
func TestEveryArmorHasRegulationStats(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, it := range c.itemList {
		if it.Category != "head" && it.Category != "chest" && it.Category != "arms" && it.Category != "legs" {
			continue
		}
		checked++
		d, ok := c.ItemDetails(it.ID)
		if !ok || d.Armor == nil {
			t.Errorf("%s (0x%08X): missing regulation-derived armor details", it.Name, it.ID)
		}
	}
	if checked != 741 {
		t.Fatalf("checked %d armor items, want 741", checked)
	}
}

func TestArmorRegulationConversionMatchesKnownItem(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := c.ItemDetails(0x1000C3B4) // Kaiden Armor
	if !ok || d.Armor == nil {
		t.Fatal("Kaiden Armor details missing")
	}
	if d.Armor.Physical != 11.9 || d.Armor.Strike != 8.8 || d.Armor.Holy != 8.0 ||
		d.Armor.Immunity != 25 || d.Armor.Robustness != 55 || d.Armor.Focus != 11 ||
		d.Armor.Vitality != 11 || d.Armor.Poise != 18 {
		t.Fatalf("Kaiden Armor conversion = %+v", *d.Armor)
	}
}
