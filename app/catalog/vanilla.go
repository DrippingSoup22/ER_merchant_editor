package catalog

// "Reset to Vanilla": diff the currently loaded save's ShopLineupParam rows
// against an embedded snapshot of FromSoftware's original vanilla values
// (data/vanilla_shop_lineup.json, generated once from save_files/
// vanilla_fresh_character.dat by tools/vanilla_shop_lineup_extract -- see
// that tool's own doc comment for the ground-truth/regeneration story).
// Caveat: this baseline goes stale if FromSoftware ever patches these
// values; it would need re-extracting from a fresh vanilla save.

import (
	"encoding/json"
	"strconv"

	"er_merchant_editor/data"
)

// vanillaDiffFields is the complete editable surface of a row: the 4 core
// fields plus the 4 display-override fields (nameIconOverrideFields,
// enrich.go) -- the only fields Reset-to-Vanilla ever compares or restores.
var vanillaDiffFields = append([]string{"equipId", "equipType", "value", "sellQuantity"}, nameIconOverrideFields...)

// loadVanillaShopLineup mirrors loadMerchantCatalog's own pattern exactly:
// a string-row-id-keyed embedded JSON, here further keyed by raw field name.
func loadVanillaShopLineup() (map[int64]map[string]int64, error) {
	raw, err := data.FS.ReadFile("vanilla_shop_lineup.json")
	if err != nil {
		return nil, err
	}
	var m map[string]map[string]int64
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := make(map[int64]map[string]int64, len(m))
	for k, v := range m {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			continue
		}
		out[id] = v
	}
	return out, nil
}

// VanillaFieldDiff is one row's Reset-to-Vanilla plan: Fields holds the
// vanilla value for every differing field EXCEPT equipId/equipType (those
// two are only ever restored together, as ItemChanged/Vanilla* below --
// see app/editor/staging.go's rowEditFromVanillaDiff for why: the
// sellValue-recompute pipeline needs a real ItemChange, not raw
// FieldChanges, to correctly attribute the row's post-reset item).
type VanillaFieldDiff struct {
	RowID  int64
	Fields map[string]int64

	ItemChanged        bool
	VanillaEquipID     int64
	VanillaEquipType   int64
	VanillaBaseEquipID int64 // the item's raw +0 base equip id; == VanillaEquipID when VanillaLevel == 0
	VanillaLevel       int64
	VanillaItemName    string // "" if unresolvable -- the numeric fields are still correct to write
	VanillaIconPath    string
}

// DiffFromVanilla reports what row needs to revert to vanilla, or ok=false
// if row.RowID has no vanilla baseline entry or nothing differs.
func (c *Catalog) DiffFromVanilla(row *Row) (d *VanillaFieldDiff, ok bool) {
	vFields, known := c.vanillaByID[row.RowID]
	if !known {
		return nil, false
	}

	d = &VanillaFieldDiff{RowID: row.RowID, Fields: map[string]int64{}}
	for _, field := range vanillaDiffFields {
		if row.Fields[field] == vFields[field] {
			continue
		}
		switch field {
		case "equipId", "equipType":
			d.ItemChanged = true
		default:
			d.Fields[field] = vFields[field]
		}
	}
	if !d.ItemChanged && len(d.Fields) == 0 {
		return nil, false
	}

	if d.ItemChanged {
		d.VanillaEquipID = vFields["equipId"]
		d.VanillaEquipType = vFields["equipType"]
		d.VanillaBaseEquipID = d.VanillaEquipID // fallback if resolution below fails
		if unifiedID, level, ok := c.resolveItemIDWithLevel(d.VanillaEquipID, int(d.VanillaEquipType)); ok {
			d.VanillaLevel = level
			if item := c.itemByID[unifiedID]; item != nil {
				d.VanillaItemName = item.Name
				d.VanillaIconPath = item.IconPath
				if item.EquipID != nil {
					d.VanillaBaseEquipID = *item.EquipID
				}
			}
		}
	}
	return d, true
}

// VanillaDiffs returns DiffFromVanilla for every row of the current save
// that differs from vanilla, skipping material-locked rows entirely --
// ApplyEdits rejects the WHOLE batch if even one staged row is
// material-locked (mirrors app/editor/staging.go's applyDrop, which
// already skips locked rows the same way on drag-fill).
func (c *Catalog) VanillaDiffs() (map[int64]*VanillaFieldDiff, error) {
	rows, err := c.ShopRows()
	if err != nil {
		return nil, err
	}
	out := map[int64]*VanillaFieldDiff{}
	for _, row := range rows {
		if row.MaterialLocked {
			continue
		}
		if d, ok := c.DiffFromVanilla(row); ok {
			out[row.RowID] = d
		}
	}
	return out, nil
}

// MerchantForRow resolves a row's canonical merchant name ("" if
// unnamed/non-browsable). Needed because Reset-to-Vanilla stages rows
// across every merchant at once -- unlike every existing staging path,
// which only ever runs while ONE merchant is being browsed and stamps
// RowEdit.Merchant from the currently-viewed merchant.
func (c *Catalog) MerchantForRow(rowID int64) string {
	cc := c.canonicalFor(rowID)
	if !isBrowsable(cc) {
		return ""
	}
	return cc.Merchant
}
