package catalog

// Weapon reinforcement rate curves (2026-08-03 user request): the item-info
// popup shows a weapon's stats at its actual "+N" upgrade level, not always
// +0. item_details.json stores the base (+0) stats; data/weapon_reinforce_
// rates.json (from tools/weapon_reinforce_rates_extract) stores, per weapon's
// own reinforceTypeId, the per-level multiplier curve the game applies. Using
// the weapon's OWN reinforceTypeId is the correct "standard scaling" for a
// base merchant weapon (no Ash-of-War affinity infusion is involved).

import (
	"encoding/json"
	"math"
	"strconv"

	"er_merchant_editor/data"
)

// reinforceColumns is the fixed order of the 15 rate columns in each per-level
// array of weapon_reinforce_rates.json (kept in sync with the extractor's
// COLUMNS). Attack (5) + scaling (5) + guard-cut (5).
const reinforceColumns = 15

// reinforceRates holds the decoded weapon_reinforce_rates.json: each
// reinforceTypeId's per-level rate curve, plus each weapon item's type.
type reinforceRates struct {
	types   map[int64][][]float64 // reinforceTypeId -> [level] -> [reinforceColumns rates]
	weapons map[int64]int64       // item id -> reinforceTypeId
}

// loadReinforceRates decodes weapon_reinforce_rates.json.
func loadReinforceRates() (*reinforceRates, error) {
	raw, err := data.FS.ReadFile("weapon_reinforce_rates.json")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Types   map[string][][]float64 `json:"types"`
		Weapons map[string]int64       `json:"weapons"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	rr := &reinforceRates{
		types:   make(map[int64][][]float64, len(doc.Types)),
		weapons: make(map[int64]int64, len(doc.Weapons)),
	}
	for k, curve := range doc.Types {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			return nil, err
		}
		rr.types[id] = curve
	}
	for k, rt := range doc.Weapons {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			return nil, err
		}
		rr.weapons[id] = rt
	}
	return rr, nil
}

// ReinforcedWeapon returns w's stats scaled to upgrade level `level`, using
// the weapon's own standard reinforcement curve. A level <= 0, an unknown
// weapon, or missing curve data returns w unchanged; a level past the curve's
// max is clamped to it. The base w is copied, never mutated.
func (c *Catalog) ReinforcedWeapon(itemID int64, level int, w WeaponDetail) WeaponDetail {
	if level <= 0 || c.reinforce == nil {
		return w
	}
	rt, ok := c.reinforce.weapons[itemID]
	if !ok {
		return w
	}
	curve, ok := c.reinforce.types[rt]
	if !ok || len(curve) == 0 {
		return w
	}
	if level >= len(curve) {
		level = len(curve) - 1
	}
	r := curve[level]
	if len(r) < reinforceColumns {
		return w
	}
	w.PhysDamage = scaleStat(w.PhysDamage, r[0])
	w.MagDamage = scaleStat(w.MagDamage, r[1])
	w.FireDamage = scaleStat(w.FireDamage, r[2])
	w.LitDamage = scaleStat(w.LitDamage, r[3])
	w.HolyDamage = scaleStat(w.HolyDamage, r[4])
	w.ScaleStr = scaleStat(w.ScaleStr, r[5])
	w.ScaleDex = scaleStat(w.ScaleDex, r[6])
	w.ScaleInt = scaleStat(w.ScaleInt, r[7])
	w.ScaleFai = scaleStat(w.ScaleFai, r[8])
	w.ScaleArc = scaleStat(w.ScaleArc, r[9])
	w.GuardPhys = scaleStat(w.GuardPhys, r[10])
	w.GuardMag = scaleStat(w.GuardMag, r[11])
	w.GuardFire = scaleStat(w.GuardFire, r[12])
	w.GuardLit = scaleStat(w.GuardLit, r[13])
	w.GuardHoly = scaleStat(w.GuardHoly, r[14])
	return w
}

func scaleStat(v uint32, rate float64) uint32 {
	if v == 0 || rate == 1 {
		return v
	}
	return uint32(math.Round(float64(v) * rate))
}
