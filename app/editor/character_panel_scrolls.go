package editor

import (
	"sort"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"er_merchant_editor/app/catalog"
)

// Scroll/Prayerbook-based unlock layout for Brother Corhyn, Miriel, and
// Sorceress Sellen (2026-08-01 user request: "replicate what you have done
// for TMH for her own bell bearings... for these merchants"). Their real
// in-game mechanic: giving one of the 11 base-game Sorcery/Incantation
// Scrolls to a "learned sorcerer/cleric" unlocks the spells it teaches --
// unlike Twin Maiden Husks' bell bearings, this never opens a DIFFERENT
// merchant's shop, so there's no "NPC Bell Bearings"-equivalent section
// here, just the 2 sections TMH's own stock gets: named groups + a plain
// "Other Items" fallback for gated rows no scroll explains (each
// merchant's own independent questline-progress unlocks -- e.g. Corhyn's
// Great Heal/Discus of Light/Immutable Shield, Sellen's Shard Spiral).
//
// Scroll -> unlocked-spells data ported from EldenRing-SaveForge's own
// curated FMG item captions (data.ItemTexts / item_text_generated.go,
// each scroll's Caption literally lists "Can be given to a learned
// sorcerer/cleric to gain access to the following sorceries/incantations:
// - X - Y"), cross-checked against this repo's own fixture save (2026-08-01,
// tools/savescan.py): every one of Corhyn's/Miriel's/Sellen's gated-row
// item-name SETS either matches one of these 14 scrolls' spell sets
// exactly, or matches none (the "Other Items" case) -- no partial/ambiguous
// matches found, so exact-set matching (scrollNameForGroup) is sufficient,
// no per-merchant flag-ID table needed the way charunlock.BellBearing
// needed one (a scroll's own real event flag differs per merchant --
// e.g. Fire Monks' Prayerbook is flag 11109874 at Corhyn but 1037469305 at
// Miriel -- so matching by SPELL SET, not flag ID, is the only thing that
// generalizes across all three). 3 scrolls (Erdtree Prayerbook, Erdtree
// Codex, Golden Order Principles) have no spell list in their own caption
// text (likely non-functional cut-content duplicates of Golden Order
// Principia -- Erdtree Codex is already `risky: true`, see
// docs/ITEM_IDS.md) and are omitted here; they were never a real
// gated-row group in the fixture save to match anyway.
var scrollUnlocks = []struct {
	Name   string
	Spells []string
}{
	{"Conspectus Scroll", []string{"Glintstone Cometshard", "Star Shower"}},
	{"Royal House Scroll", []string{"Glintblade Phalanx", "Carian Slicer"}},
	{"Academy Scroll", []string{"Great Glintstone Shard", "Swift Glintstone Shard"}},
	{"Fire Monks' Prayerbook", []string{"O, Flame!", "Surge, O Flame!"}},
	{"Giant's Prayerbook", []string{"Giantsflame Take Thee", "Flame, Fall Upon Them"}},
	{"Godskin Prayerbook", []string{"Black Flame", "Black Flame Blade"}},
	{"Two Fingers' Prayerbook", []string{"Lord's Heal", "Lord's Aid"}},
	{"Assassin's Prayerbook", []string{"Assassin's Approach", "Darkness"}},
	{"Golden Order Principia", []string{"Radagon's Rings of Light", "Law of Regression"}},
	{"Dragon Cult Prayerbook", []string{"Lightning Spear", "Honed Bolt", "Electrify Armament"}},
	{"Ancient Dragon Prayerbook", []string{"Ancient Dragons' Lightning Spear", "Ancient Dragons' Lightning Strike"}},
}

// scrollFlagsMerchants: the 3 merchants whose Flags column uses the
// scroll-grouped layout instead of the plain flat list every other
// merchant (besides Twin Maiden Husks) gets.
var scrollFlagsMerchants = map[string]bool{
	"Brother Corhyn":   true,
	"Miriel":           true,
	"Sorceress Sellen": true,
}

// scrollNameForGroup reports the scroll whose spell set exactly matches
// g's item names (order-independent), if any.
func scrollNameForGroup(g flagGroup) (string, bool) {
	names := make(map[string]bool, len(g.Rows))
	for _, r := range g.Rows {
		n := r.DisplayName()
		if n == "" {
			n = r.Label
		}
		names[n] = true
	}
	for _, su := range scrollUnlocks {
		if len(su.Spells) != len(names) {
			continue
		}
		match := true
		for _, sp := range su.Spells {
			if !names[sp] {
				match = false
				break
			}
		}
		if match {
			return su.Name, true
		}
	}
	return "", false
}

// scrollGroupEntry pairs a flagGroup with its resolved scroll name (empty
// if none matched -- handled as an "Other Items" entry).
type scrollGroupEntry struct {
	group flagGroup
	name  string
}

// scrollFlagSections classifies rows' gated-row groups into named
// (scroll-matched) and other (unmatched) buckets, both sorted alphabetically
// by their display label for a stable, scannable order (no natural family/
// number grouping the way TMH's bell bearings have).
func scrollFlagSections(rows []*catalog.Row) (named, other []scrollGroupEntry) {
	for _, g := range groupFlagRows(rows) {
		if name, ok := scrollNameForGroup(g); ok {
			named = append(named, scrollGroupEntry{group: g, name: name})
		} else {
			other = append(other, scrollGroupEntry{group: g})
		}
	}
	sort.Slice(named, func(i, j int) bool { return named[i].name < named[j].name })
	sort.Slice(other, func(i, j int) bool {
		return flagGroupLabel(other[i].group) < flagGroupLabel(other[j].group)
	})
	return named, other
}

// layoutScrollFlagsGrid is the Flags column body for Brother Corhyn/Miriel/
// Sorceress Sellen: one scrollable list (same single-Flexed-region pattern
// layoutTMHFlagsGrid uses, no trailing Rigid sibling -- see that function's
// own doc comment for why), 2 sections stacked top to bottom, each omitted
// when empty.
func (s *State) layoutScrollFlagsGrid(gtx layout.Context, th *material.Theme) layout.Dimensions {
	named, other := scrollFlagSections(s.FlagRows)

	var blocks []layout.Widget
	addTMHSection(th, &blocks, "Scrolls & Prayerbooks", len(named), func(i int) layout.Widget {
		e := named[i]
		return func(gtx layout.Context) layout.Dimensions { return s.layoutFlagRowLabeled(gtx, th, e.group, e.name) }
	})
	addTMHSectionPaired(th, &blocks, "Other Items", len(other), func(i int) layout.Widget {
		e := other[i]
		return func(gtx layout.Context) layout.Dimensions { return s.layoutFlagRow(gtx, th, e.group) }
	})

	s.scrollColList.Axis = layout.Vertical
	return material.List(th, &s.scrollColList).Layout(gtx, len(blocks), func(gtx layout.Context, i int) layout.Dimensions {
		return blocks[i](gtx)
	})
}

// layoutFlagRowLabeled is layoutFlagRowNamed's charunlock.BellBearing-free
// counterpart: same checkbox/staging/amber-if-staged logic and "unlocks: X,
// Y" subtext, but the primary label is a plain string (a scroll's name)
// instead of a BellBearing entry's .Name -- scrolls aren't bell bearings,
// no shared data type needed.
func (s *State) layoutFlagRowLabeled(gtx layout.Context, th *material.Theme, g flagGroup, label string) layout.Dimensions {
	chk, staged := s.flagGroupCheckbox(g)

	box := material.CheckBox(th, chk, label)
	if staged {
		box.Color = colorAmber
	}
	subtext := material.Body2(th, "unlocks: "+flagGroupLabel(g))
	subtext.Color = colorMuted

	return layout.Inset{Top: 2, Bottom: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(box.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(28)}.Layout(gtx, subtext.Layout)
			}),
		)
	})
}
