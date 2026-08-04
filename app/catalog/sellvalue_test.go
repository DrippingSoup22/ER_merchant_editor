package catalog

// SellValue decodes live from the loaded save's own EquipParam* tables (not
// embedded/static data, see sellvalue.go), so these need the fixture --
// gitignored, CI skips.

import "testing"

// TestSellValueKnownItems cross-checks a handful of items whose sellValue
// this investigation already confirmed by hand (see docs/MERCHANT_DATA.md's
// 2026-07-30 "cost=0" entry): key items/one-of-a-kind weapons get FromSoft's
// own "-1 = can never be sold back" sentinel, normal items get a real
// positive value.
func TestSellValueKnownItems(t *testing.T) {
	c := loadedCatalog(t)

	cases := []struct {
		name      string
		equipType int64
		equipID   int64
		want      int64
	}{
		{"Cracked Pot (Goods)", 3, 9500, -1},
		{"Stonesword Key (Goods)", 3, 8000, -1},
		{"Serpent-Hunter (Weapon)", 0, 17030000, -1},
		{"Rune Arc (Goods)", 3, 190, 200},
		{"Smithing Stone [1] (Goods)", 3, 10100, 100},
		{"Dagger (Weapon)", 0, 1000000, 100},
	}
	for _, c2 := range cases {
		got, ok := c.SellValue(c2.equipType, c2.equipID)
		if !ok {
			t.Errorf("%s: SellValue(%d, %d) not found", c2.name, c2.equipType, c2.equipID)
			continue
		}
		if got != c2.want {
			t.Errorf("%s: SellValue(%d, %d) = %d, want %d", c2.name, c2.equipType, c2.equipID, got, c2.want)
		}
	}
}

// TestSellValueUnknownRefIsNotOK covers a nonexistent (equipType, equipID)
// pair -- must report ok=false, not a zero-value false positive.
func TestSellValueUnknownRefIsNotOK(t *testing.T) {
	c := loadedCatalog(t)
	if _, ok := c.SellValue(0, 999999999); ok {
		t.Error("SellValue for a nonexistent equipId reported ok=true")
	}
}

// TestSellValueCacheInvalidatedByLoadSave: the lazily-decoded cache must not
// leak across a LoadSave call (even to the same file) -- guards against a
// stale-cache bug if a later change reorders when the cache gets populated.
func TestSellValueCacheInvalidatedByLoadSave(t *testing.T) {
	c := loadedCatalog(t)
	if _, ok := c.SellValue(3, 190); !ok {
		t.Fatal("setup: Rune Arc sellValue not found")
	}
	if c.sellValueByEquipRef == nil {
		t.Fatal("setup: cache should be populated after a SellValue call")
	}
	if err := c.LoadSave(fixtureSave); err != nil {
		t.Fatal(err)
	}
	if c.sellValueByEquipRef != nil {
		t.Error("LoadSave must invalidate the sellValue cache")
	}
}
