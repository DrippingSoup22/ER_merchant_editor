package main

// Item-info popup data (2026-08-01 user request: right-click an item to see
// its icon/description/category/scaling/etc). SaveForge's db.GetAllItems
// already computes everything needed (Description, Weight, Weapon/Armor/
// Spell stat structs) -- no new extraction pipeline, just a second
// projection of the same ItemEntry slice main() already has, written
// straight to data/item_details.json (not stdout, so the existing
// `go run . > ../../data/items.json` regenerate command is untouched and
// now also refreshes this file as a side effect).

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"

	"github.com/oisis/EldenRing-SaveForge/backend/db"
	"github.com/oisis/EldenRing-SaveForge/backend/db/data"
)

// weaponPassive is one on-hit / resident status effect the weapon carries
// (e.g. "Blood Loss 50"), sourced from WeaponStatsV1.PassiveEffects. Only
// resolved (Known) named effects are emitted -- unresolved SpEffect IDs
// ("Unknown on-hit effect") carry no label worth showing in the popup.
type weaponPassive struct {
	Label string `json:"label"`
	Value int32  `json:"value,omitempty"`
}

type weaponDetail struct {
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
	// Guard cut rates (%) + guard boost, and crit rate -- sourced from the
	// richer WeaponStatsV1ByID record (the legacy db.ItemEntry.Weapon shape
	// drops them). Crit is the in-game display value (base 100 pre-added).
	GuardPhys  uint32           `json:"guardPhys,omitempty"`
	GuardMag   uint32           `json:"guardMag,omitempty"`
	GuardFire  uint32           `json:"guardFire,omitempty"`
	GuardLit   uint32           `json:"guardLit,omitempty"`
	GuardHoly  uint32           `json:"guardHoly,omitempty"`
	GuardBoost uint32           `json:"guardBoost,omitempty"`
	Crit       uint32           `json:"crit,omitempty"`
	Passives   []weaponPassive  `json:"passives,omitempty"`
}

type armorDetail struct {
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
}

type spellDetail struct {
	FPCost uint32 `json:"fpCost,omitempty"`
	Slots  uint32 `json:"slots,omitempty"`
	ReqInt uint32 `json:"reqInt,omitempty"`
	ReqFai uint32 `json:"reqFai,omitempty"`
	ReqArc uint32 `json:"reqArc,omitempty"`
}

// scalingDetail is the attribute scaling of a damage-dealing throwable
// consumable (from consumable_scaling.json / MagicParam-adjacent virtual
// weapon). Zero stats are omitted. Distinct from weaponDetail: these items
// render as a description card with a Scaling panel, not a full weapon card.
type scalingDetail struct {
	Str uint32 `json:"str,omitempty"`
	Dex uint32 `json:"dex,omitempty"`
	Int uint32 `json:"int,omitempty"`
	Fai uint32 `json:"fai,omitempty"`
	Arc uint32 `json:"arc,omitempty"`
}

type itemDetail struct {
	ID          uint32         `json:"id"`
	Description string         `json:"description,omitempty"`
	Location    string         `json:"location,omitempty"`
	Weight      float64        `json:"weight,omitempty"`
	Weapon      *weaponDetail  `json:"weapon,omitempty"`
	Armor       *armorDetail   `json:"armor,omitempty"`
	Spell       *spellDetail   `json:"spell,omitempty"`
	Scaling     *scalingDetail `json:"scaling,omitempty"`
}

// effectText resolves an item's shown description/effect text from
// SaveForge's FMG-sourced data.ItemTexts (keyed by our app id), which covers
// far more items than db.GetAllItems' own enrichment reaches -- notably
// tools/consumables, spirit ashes, key items and info notes, which come back
// from GetAllItems with an empty Description. It prefers the concise
// Description field (the short in-game effect line, e.g. "Completely restores
// HP and heals all ailments") but falls back to the fuller Caption when
// Description is empty or merely repeats the item name (as it does for spirit
// ashes). Existing non-empty text is kept when ItemTexts has no better entry.
// captionCategories are item types whose concise Description field is a
// generic, identical-for-all placeholder ("Grants affinities and skills to an
// armament" for every Ash of War; "Note imparting knowledge in brief" for
// every info note; a summon-type label for spirit ashes) -- for these the
// real, item-specific text lives in the Caption (the AoW's actual skill/
// moveset, the note's actual contents), so we always take Caption.
var captionCategories = map[string]bool{
	"ashes":        true, // Spirit Ashes: Description is just the summon name
	"ashes_of_war": true, // Ash of War: Description is the generic grant blurb
	"info":         true, // Notes/letters: Description is a generic one-liner
}

// controlCodeRE matches leftover FMG control tokens like "<?keyicon@31?>"
// (a button-glyph placeholder the game renders as an icon) which are
// meaningless as plain text.
var controlCodeRE = regexp.MustCompile(`<\?[^>]*\?>`)

func effectText(id uint32, category, existing string) string {
	txt, ok := data.ItemTexts[id]
	if !ok {
		return cleanText(existing)
	}
	d := txt.Description
	if captionCategories[category] || d == "" || d == txt.DisplayName || d == txt.CanonicalName {
		if txt.Caption != "" {
			d = txt.Caption
		}
	}
	if d == "" {
		d = existing
	}
	return cleanText(d)
}

// cleanText strips FMG control tokens and any now-dangling "Examine using"
// tail they left behind, then trims surrounding whitespace.
func cleanText(s string) string {
	s = controlCodeRE.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\nExamine using ", "")
	return strings.TrimSpace(s)
}

// buildItemDetails projects db.GetAllItems' ItemEntry slice down to just
// the item-info-popup fields, skipping any entry with nothing to show
// (no description AND no stat block -- most of the ~700 data.KeyItems-only
// merges in main() fall in here, since SaveForge's own text/stats
// enrichment only covers db.GetAllItems' own output).
func buildItemDetails(items []db.ItemEntry) []itemDetail {
	spellStats := loadSpellStats()
	consumableScaling := loadConsumableScaling()
	out := make([]itemDetail, 0, len(items))
	for _, it := range items {
		d := itemDetail{
			ID:          it.ID,
			Description: effectText(it.ID, it.Category, it.Description),
			Location:    it.Location,
			Weight:      it.Weight,
		}
		if it.Weapon != nil {
			d.Weapon = &weaponDetail{
				PhysDamage: it.Weapon.PhysDamage, MagDamage: it.Weapon.MagDamage,
				FireDamage: it.Weapon.FireDamage, LitDamage: it.Weapon.LitDamage, HolyDamage: it.Weapon.HolyDamage,
				ScaleStr: it.Weapon.ScaleStr, ScaleDex: it.Weapon.ScaleDex,
				ScaleInt: it.Weapon.ScaleInt, ScaleFai: it.Weapon.ScaleFai,
				ReqStr: it.Weapon.ReqStr, ReqDex: it.Weapon.ReqDex, ReqInt: it.Weapon.ReqInt,
				ReqFai: it.Weapon.ReqFai, ReqArc: it.Weapon.ReqArc,
			}
			enrichWeaponV1(d.Weapon, it.ID)
		}
		if it.Armor != nil {
			d.Armor = &armorDetail{
				Physical: it.Armor.Physical, Strike: it.Armor.Strike, Slash: it.Armor.Slash, Pierce: it.Armor.Pierce,
				Magic: it.Armor.Magic, Fire: it.Armor.Fire, Lightning: it.Armor.Lightning, Holy: it.Armor.Holy,
				Poise: it.Armor.Poise, Immunity: it.Armor.Immunity, Robustness: it.Armor.Robustness,
				Focus: it.Armor.Focus, Vitality: it.Armor.Vitality,
			}
		}
		if it.Spell != nil {
			d.Spell = &spellDetail{
				FPCost: it.Spell.FPCost, Slots: it.Spell.Slots,
				ReqInt: it.Spell.ReqInt, ReqFai: it.Spell.ReqFai, ReqArc: it.Spell.ReqArc,
			}
		}
		// Prefer MagicParam ground truth over SaveForge's curated spell table:
		// it matches for every curated spell (verified 0 mismatches) and adds
		// the ~42 Shadow-of-the-Erdtree DLC spells the curated table omits.
		if ss, ok := spellStats[it.ID]; ok {
			d.Spell = ss
		}
		if sc, ok := consumableScaling[it.ID]; ok {
			d.Scaling = sc
		}
		if d.Description == "" && d.Weapon == nil && d.Armor == nil && d.Spell == nil && d.Scaling == nil {
			continue
		}
		out = append(out, d)
	}
	return out
}

// loadSpellStats reads data/spell_stats.json (generated by
// tools/spell_stats_extract from the fixture's MagicParam) into a lookup by
// item id. Missing file is fatal: a silent fall-back to SaveForge's partial
// spell table would quietly drop the DLC spells this exists to add.
func loadSpellStats() map[uint32]*spellDetail {
	raw, err := os.ReadFile("../../data/spell_stats.json")
	if err != nil {
		panic("read spell_stats.json (run tools/spell_stats_extract first): " + err.Error())
	}
	var doc struct {
		Spells []struct {
			ItemID uint32 `json:"itemId"`
			FP     uint32 `json:"fp"`
			Slots  uint32 `json:"slots"`
			ReqInt uint32 `json:"reqInt"`
			ReqFai uint32 `json:"reqFai"`
			ReqArc uint32 `json:"reqArc"`
		} `json:"spells"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		panic("parse spell_stats.json: " + err.Error())
	}
	out := make(map[uint32]*spellDetail, len(doc.Spells))
	for _, s := range doc.Spells {
		out[s.ItemID] = &spellDetail{
			FPCost: s.FP, Slots: s.Slots,
			ReqInt: s.ReqInt, ReqFai: s.ReqFai, ReqArc: s.ReqArc,
		}
	}
	return out
}

// loadConsumableScaling reads data/consumable_scaling.json (generated by
// tools/consumable_scaling_extract from the fixture's EquipParamGoods ->
// virtual EquipParamWeapon chain) into a lookup by item id. Missing file is
// fatal for the same reason as spell_stats: silently shipping without it
// would drop the throwable-scaling panels this exists to add.
func loadConsumableScaling() map[uint32]*scalingDetail {
	raw, err := os.ReadFile("../../data/consumable_scaling.json")
	if err != nil {
		panic("read consumable_scaling.json (run tools/consumable_scaling_extract first): " + err.Error())
	}
	var doc struct {
		Items []struct {
			ItemID uint32 `json:"itemId"`
			Str    uint32 `json:"str"`
			Dex    uint32 `json:"dex"`
			Int    uint32 `json:"int"`
			Fai    uint32 `json:"fai"`
			Arc    uint32 `json:"arc"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		panic("parse consumable_scaling.json: " + err.Error())
	}
	out := make(map[uint32]*scalingDetail, len(doc.Items))
	for _, s := range doc.Items {
		out[s.ItemID] = &scalingDetail{Str: s.Str, Dex: s.Dex, Int: s.Int, Fai: s.Fai, Arc: s.Arc}
	}
	return out
}

// clampU32 drops the int32->uint32 sign the way SaveForge's own legacy
// projection does (negative -> 0); V1 guard/crit values are non-negative in
// practice but the guard keeps the projection safe.
func clampU32(v int32) uint32 {
	if v < 0 {
		return 0
	}
	return uint32(v)
}

// enrichWeaponV1 folds the guard/crit/arc-scaling/passive fields that only
// live on the richer WeaponStatsV1ByID record onto w. The base
// damage/req/str-dex-int-fai scaling already came from db.ItemEntry.Weapon
// (identical source values); this adds only what that legacy shape drops.
func enrichWeaponV1(w *weaponDetail, id uint32) {
	v, ok := data.WeaponStatsV1ByID[id]
	if !ok {
		return
	}
	w.ScaleArc = clampU32(v.ScalingArcRaw)
	w.GuardPhys = clampU32(v.GuardPhysical)
	w.GuardMag = clampU32(v.GuardMagic)
	w.GuardFire = clampU32(v.GuardFire)
	w.GuardLit = clampU32(v.GuardLightning)
	w.GuardHoly = clampU32(v.GuardHoly)
	w.GuardBoost = clampU32(v.GuardBoost)
	w.Crit = clampU32(v.Critical)
	for _, p := range v.PassiveEffects {
		if !p.Known || p.Label == "" {
			continue // unresolved SpEffect IDs carry no showable name
		}
		w.Passives = append(w.Passives, weaponPassive{Label: p.Label, Value: p.Value})
	}
}

// detailAliases is item_details.json's counterpart to main.go's
// manualPatchItems: an id that items.json carries but SaveForge has NO
// detail source for at all (absent from both db.GetAllItems and
// data.ItemTexts) inherits its entry wholesale from a sibling id
// describing the SAME game object.
//
// 0x400000FA ("Flask of Wondrous Physick", filled) is the id real shop rows
// use (row 101878, Twin Maiden Husks) but only its empty-state sibling
// 0x400000FB has text upstream -- so the item-info popup showed "No
// additional details" for the one item a player can actually be sold
// (user-reported 2026-08-03). Both ids are the same flask in different fill
// states, so FB's own text/stats describe FA exactly; FB's Description is
// itself the game's real "...but is empty now" line (the flask IS empty
// until crystal tears are mixed in), not a placeholder.
var detailAliases = map[uint32]uint32{
	0x400000FA: 0x400000FB, // Flask of Wondrous Physick: filled <- empty
}

// applyDetailAliases appends an entry for every detailAliases id missing
// from details, cloned from its source id (only the ID field differs).
// A no-op for any alias whose id already has a real entry, so this can
// never mask upstream gaining proper data later.
func applyDetailAliases(details []itemDetail) []itemDetail {
	byID := make(map[uint32]int, len(details))
	for i, d := range details {
		byID[d.ID] = i
	}
	for alias, src := range detailAliases {
		if _, exists := byID[alias]; exists {
			continue
		}
		i, ok := byID[src]
		if !ok {
			continue // source itself has no entry; nothing to inherit
		}
		clone := details[i]
		clone.ID = alias
		details = append(details, clone)
	}
	return details
}

func writeItemDetails(items []db.ItemEntry) error {
	details := applyDetailAliases(buildItemDetails(items))
	raw, err := json.MarshalIndent(details, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("../../data/item_details.json", append(raw, '\n'), 0o644)
}
