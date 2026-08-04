package catalog

// Item-info popup data (2026-08-01 user request): description/weight/
// weapon-armor-spell stats for items.json entries, generated alongside
// items.json itself by tools/itemdb_extract from SaveForge's already-
// computed db.ItemEntry fields -- see item_details.json's own doc comment
// in tools/itemdb_extract/details.go. Loaded once at New(), same pattern
// as loadWeaponReinforce/loadSortOrder.

import (
	"encoding/json"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/assets/data"
)

// WeaponDetail is a weapon-table item's base (+0, Standard affinity, see
// docs/ITEM_IDS.md) combat stats. Zero fields are omitted from the source
// JSON (e.g. a purely physical weapon has MagDamage/FireDamage/etc == 0),
// which decodes to their Go zero value here too -- not distinguishable from
// "field absent", which is fine, a 0 is a 0 either way for display purposes.
type WeaponDetail struct {
	PhysDamage, MagDamage, FireDamage, LitDamage, HolyDamage uint32
	ScaleStr, ScaleDex, ScaleInt, ScaleFai, ScaleArc         uint32
	ReqStr, ReqDex, ReqInt, ReqFai, ReqArc                   uint32
	// Guard cut rates (%), guard boost, and crit rate -- see the popup card.
	GuardPhys, GuardMag, GuardFire, GuardLit, GuardHoly, GuardBoost uint32
	Crit                                                            uint32
	Passives                                                        []WeaponPassive
}

// WeaponPassive is one named on-hit/resident status effect ("Blood Loss 50").
type WeaponPassive struct {
	Label string
	Value int32
}

// ArmorDetail is a protector/talisman item's negation/resistance stats.
type ArmorDetail struct {
	Physical, Strike, Slash, Pierce, Magic, Fire, Lightning, Holy, Poise float64
	Immunity, Robustness, Focus, Vitality                                uint32
}

// SpellDetail is a sorcery/incantation item's cast cost/requirements.
type SpellDetail struct {
	FPCost, Slots, ReqInt, ReqFai, ReqArc uint32
}

// ScalingDetail is a damage throwable's attribute scaling coefficients
// (raw correctX values from its virtual weapon); the popup grades them like
// a weapon. Only for description-card consumables, distinct from WeaponDetail.
type ScalingDetail struct {
	Str, Dex, Int, Fai, Arc uint32
}

// ItemDetail is one item_details.json entry -- everything the item-info
// popup shows beyond what Item already carries (name/category/icon).
// Weapon/Armor/Spell are mutually exclusive in practice (an item is at
// most one of the three equip kinds); Description/Location/Weight can
// accompany any of them, or stand alone for a plain consumable/key item.
type ItemDetail struct {
	Description string
	Location    string
	Weight      float64
	Weapon      *WeaponDetail
	Armor       *ArmorDetail
	Spell       *SpellDetail
	Scaling     *ScalingDetail
}

// itemDetailJSON mirrors tools/itemdb_extract/details.go's on-disk shape
// exactly (same field names/json tags) -- kept as a private decode target
// so ItemDetail itself can stay a clean, tag-free struct.
type itemDetailJSON struct {
	ID          uint32  `json:"id"`
	Description string  `json:"description,omitempty"`
	Location    string  `json:"location,omitempty"`
	Weight      float64 `json:"weight,omitempty"`
	Weapon      *struct {
		PhysDamage uint32 `json:"physDamage,omitempty"`
		MagDamage  uint32 `json:"magDamage,omitempty"`
		FireDamage uint32 `json:"fireDamage,omitempty"`
		LitDamage  uint32 `json:"litDamage,omitempty"`
		HolyDamage uint32 `json:"holyDamage,omitempty"`
		ScaleStr   uint32 `json:"scaleStr,omitempty"`
		ScaleDex   uint32 `json:"scaleDex,omitempty"`
		ScaleInt   uint32 `json:"scaleInt,omitempty"`
		ScaleFai   uint32 `json:"scaleFai,omitempty"`
		ScaleArc   uint32 `json:"scaleArc,omitempty"`
		ReqStr     uint32 `json:"reqStr,omitempty"`
		ReqDex     uint32 `json:"reqDex,omitempty"`
		ReqInt     uint32 `json:"reqInt,omitempty"`
		ReqFai     uint32 `json:"reqFai,omitempty"`
		ReqArc     uint32 `json:"reqArc,omitempty"`
		GuardPhys  uint32 `json:"guardPhys,omitempty"`
		GuardMag   uint32 `json:"guardMag,omitempty"`
		GuardFire  uint32 `json:"guardFire,omitempty"`
		GuardLit   uint32 `json:"guardLit,omitempty"`
		GuardHoly  uint32 `json:"guardHoly,omitempty"`
		GuardBoost uint32 `json:"guardBoost,omitempty"`
		Crit       uint32 `json:"crit,omitempty"`
		Passives   []struct {
			Label string `json:"label"`
			Value int32  `json:"value,omitempty"`
		} `json:"passives,omitempty"`
	} `json:"weapon,omitempty"`
	Armor *struct {
		Physical   float64 `json:"physical,omitempty"`
		Strike     float64 `json:"strike,omitempty"`
		Slash      float64 `json:"slash,omitempty"`
		Pierce     float64 `json:"pierce,omitempty"`
		Magic      float64 `json:"magic,omitempty"`
		Fire       float64 `json:"fire,omitempty"`
		Lightning  float64 `json:"lightning,omitempty"`
		Holy       float64 `json:"holy,omitempty"`
		Poise      float64 `json:"poise,omitempty"`
		Immunity   uint32  `json:"immunity,omitempty"`
		Robustness uint32  `json:"robustness,omitempty"`
		Focus      uint32  `json:"focus,omitempty"`
		Vitality   uint32  `json:"vitality,omitempty"`
	} `json:"armor,omitempty"`
	Spell *struct {
		FPCost uint32 `json:"fpCost,omitempty"`
		Slots  uint32 `json:"slots,omitempty"`
		ReqInt uint32 `json:"reqInt,omitempty"`
		ReqFai uint32 `json:"reqFai,omitempty"`
		ReqArc uint32 `json:"reqArc,omitempty"`
	} `json:"spell,omitempty"`
	Scaling *struct {
		Str uint32 `json:"str,omitempty"`
		Dex uint32 `json:"dex,omitempty"`
		Int uint32 `json:"int,omitempty"`
		Fai uint32 `json:"fai,omitempty"`
		Arc uint32 `json:"arc,omitempty"`
	} `json:"scaling,omitempty"`
}

// loadItemDetails decodes item_details.json into a lookup by item id.
func loadItemDetails() (map[int64]ItemDetail, error) {
	raw, err := data.FS.ReadFile("item_details.json")
	if err != nil {
		return nil, err
	}
	var docs []itemDetailJSON
	if err := json.Unmarshal(raw, &docs); err != nil {
		return nil, err
	}
	out := make(map[int64]ItemDetail, len(docs))
	for _, d := range docs {
		det := ItemDetail{Description: d.Description, Location: d.Location, Weight: d.Weight}
		if d.Weapon != nil {
			det.Weapon = &WeaponDetail{
				PhysDamage: d.Weapon.PhysDamage, MagDamage: d.Weapon.MagDamage,
				FireDamage: d.Weapon.FireDamage, LitDamage: d.Weapon.LitDamage, HolyDamage: d.Weapon.HolyDamage,
				ScaleStr: d.Weapon.ScaleStr, ScaleDex: d.Weapon.ScaleDex,
				ScaleInt: d.Weapon.ScaleInt, ScaleFai: d.Weapon.ScaleFai, ScaleArc: d.Weapon.ScaleArc,
				ReqStr: d.Weapon.ReqStr, ReqDex: d.Weapon.ReqDex, ReqInt: d.Weapon.ReqInt,
				ReqFai: d.Weapon.ReqFai, ReqArc: d.Weapon.ReqArc,
				GuardPhys: d.Weapon.GuardPhys, GuardMag: d.Weapon.GuardMag, GuardFire: d.Weapon.GuardFire,
				GuardLit: d.Weapon.GuardLit, GuardHoly: d.Weapon.GuardHoly, GuardBoost: d.Weapon.GuardBoost,
				Crit: d.Weapon.Crit,
			}
			for _, p := range d.Weapon.Passives {
				det.Weapon.Passives = append(det.Weapon.Passives, WeaponPassive{Label: p.Label, Value: p.Value})
			}
		}
		if d.Armor != nil {
			det.Armor = &ArmorDetail{
				Physical: d.Armor.Physical, Strike: d.Armor.Strike, Slash: d.Armor.Slash, Pierce: d.Armor.Pierce,
				Magic: d.Armor.Magic, Fire: d.Armor.Fire, Lightning: d.Armor.Lightning, Holy: d.Armor.Holy,
				Poise: d.Armor.Poise, Immunity: d.Armor.Immunity, Robustness: d.Armor.Robustness,
				Focus: d.Armor.Focus, Vitality: d.Armor.Vitality,
			}
		}
		if d.Spell != nil {
			det.Spell = &SpellDetail{
				FPCost: d.Spell.FPCost, Slots: d.Spell.Slots,
				ReqInt: d.Spell.ReqInt, ReqFai: d.Spell.ReqFai, ReqArc: d.Spell.ReqArc,
			}
		}
		if d.Scaling != nil {
			det.Scaling = &ScalingDetail{
				Str: d.Scaling.Str, Dex: d.Scaling.Dex, Int: d.Scaling.Int,
				Fai: d.Scaling.Fai, Arc: d.Scaling.Arc,
			}
		}
		out[int64(d.ID)] = det
	}
	return out, nil
}

// ItemDetails returns id's popup detail data, ok=false if item_details.json
// has no entry for it (e.g. an item with neither a description nor a stat
// block -- see buildItemDetails' skip rule).
func (c *Catalog) ItemDetails(id int64) (ItemDetail, bool) {
	d, ok := c.itemDetails[id]
	return d, ok
}
