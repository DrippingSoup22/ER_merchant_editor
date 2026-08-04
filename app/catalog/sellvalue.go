package catalog

// EquipParam* sellValue lookup (Weapon/Protector/Accessory/Goods/Gem,
// equipType 0-4 -- same convention ShopLineupParam's own equipType field
// uses). sellValue lives in a separate table, keyed by equip id not shop row
// id; it governs the price>=sellValue merchant-visibility rule (see
// docs/MERCHANT_DATA.md). Decoded LIVE from the currently loaded save (not a
// precomputed vanilla snapshot), since the loaded save may itself already be
// edited -- reuses the same generic BND4/param decode ShopRows uses, off the
// regulation.bin blob already in memory.

import (
	"er_merchant_editor/app/shopwrite"
)

// equipRef identifies one EquipParam* row: (equipType, equipId), the same
// pair ShopLineupParam.equipId/equipType (or a staged ItemChange's
// EquipType/BaseEquipID) already carry.
type equipRef struct {
	EquipType int64
	EquipID   int64
}

// SellValue returns the currently loaded save's EquipParam* sellValue for
// (equipType, equipID), decoding and caching all 5 tables on first use per
// load. ok is false if no save is loaded or the ref doesn't resolve to a
// real row.
func (c *Catalog) SellValue(equipType, equipID int64) (int64, bool) {
	if c.sellValueByEquipRef == nil {
		if err := c.loadSellValues(); err != nil {
			return 0, false
		}
	}
	v, ok := c.sellValueByEquipRef[equipRef{equipType, equipID}]
	return v, ok
}

func (c *Catalog) loadSellValues() error {
	path, err := c.requireSave()
	if err != nil {
		return err
	}
	reg, err := shopwrite.LoadRegulation(path)
	if err != nil {
		return err
	}
	m := make(map[equipRef]int64)
	for equipType := int64(0); equipType <= 4; equipType++ {
		entryName, ok := shopwrite.EquipParamEntryName(equipType)
		if !ok {
			continue
		}
		schema, err := shopwrite.LoadEquipParamSchema(equipType)
		if err != nil {
			return err
		}
		_, blob, _, rows, err := shopwrite.LoadParamRows(reg.BND4, entryName)
		if err != nil {
			return err
		}
		for _, r := range rows {
			fields, err := shopwrite.DecodeRowFields(blob, r.DataOffset, schema)
			if err != nil {
				return err
			}
			m[equipRef{equipType, int64(r.ID)}] = fields["sellValue"]
		}
	}
	c.sellValueByEquipRef = m
	return nil
}
