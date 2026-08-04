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
