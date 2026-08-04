// Command itemdb_extract dumps a light id->name/category lookup table by
// importing EldenRing-SaveForge's already-built, FMG-sourced item DB
// (db.GetAllItems) and projecting it down to just the fields we need.
//
// db.GetAllItems deliberately excludes data.KeyItems entries flagged
// "no_database" (SaveForge hides them from its own add-item picker since
// they're single-use unlock triggers, not generically addable inventory
// items -- see EldenRing-SaveForge's key_items_routing_test.go). That
// exclusion doesn't apply to us: several (Memory Stone, Talisman Pouch,
// Cracked Pot, Ritual Pot, Perfume Bottle, Hefty Cracked Pot) are real
// ShopLineupParam-sold goods our editor needs to display (confirmed
// 2026-07-25, see docs/ITEM_IDS.md). Merge in every data.KeyItems entry
// GetAllItems didn't already return, keyed by id -- regenerable, not a
// hand-maintained list.
//
// Regenerate: go run . > ../../internal/assets/data/items.json  (run from this directory)
package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/oisis/EldenRing-SaveForge/backend/db"
	"github.com/oisis/EldenRing-SaveForge/backend/db/data"
)

type itemRecord struct {
	ID          uint32 `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	SubCategory string `json:"subCategory,omitempty"`
	IconPath    string `json:"iconPath,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
	Risky       bool   `json:"risky,omitempty"`
}

// riskyFlags: SaveForge's curated per-item flags marking items that are cut
// content and/or a known online-ban risk. Items carrying either are tagged
// risky in items.json; the editor only offers them in Debug mode.
var riskyFlags = map[string]bool{"cut_content": true, "ban_risk": true}

// riskyOverrides corrects upstream flags where an item is either omitted from
// the cut-content classification or incorrectly included in it. These are
// applied before altered-armor propagation so items.json remains generated
// from one canonical rule set.
var riskyOverrides = map[uint32]bool{
	// Found in the game files but never implemented as a playable pickup.
	// Unlike its similarly-unused Erdtree Codex counterpart, SaveForge does
	// not flag this prayerbook.
	0x4000229C: true, // Erdtree Prayerbook

	// A legitimate Leyndell pickup at Lower Capital Church. Deathbed Smalls
	// (a different item) are the cut-content piece.
	0x101D7374: false, // Deathbed Dress
}

// hiddenItemIDs: kept in items.json (so shop rows referencing them still
// resolve to a name/icon) but hidden from the editor's catalog grid --
// internal same-name/same-category state variants of another id, where the
// other id is the one the game's own vanilla shop rows use. Only one case
// known (2026-07-28): the Flask of Wondrous Physick pair 0x400000FA (filled;
// what Twin Maiden Husks' vanilla row 101878 sells, equipId 250) vs
// 0x400000FB (state variant). Same-NAME cross-CATEGORY pairs (e.g. Carian
// Greatsword the sorcery vs the Ash of War) are distinct real items -- never
// list them here.
// gestures + non-purchasable info items (2026-07-30): cross-referenced every
// gestures/info item name against every real ShopLineupParam row in the
// fixture save (base game + DLC, 1277 rows) -- see docs/ITEM_IDS.md. Never
// meant to be merchant stock, so hidden from the browsable catalog entirely
// (not just Debug-mode-gated like `risky`).
var hiddenItemIDs = map[uint32]bool{
	0x400000FB: true, // Flask of Wondrous Physick (state variant of 0x400000FA)

	// Mutually-exclusive same-name "(Variant)" quest-state duplicate
	// (2026-08-03, user request): the game gives two ids the same display
	// name for a single item whose two states are never held at once, so it
	// "appears as one" in-game. Confirmed the distinction against the
	// fixture's own EquipParamGoods (real goods id = SaveForge id & 0x3FFFFFFF)
	// plus wiki.gg: Miniature Ranni 8187 (base, iconId 3224) vs 8146
	// ("(Variant)", DIFFERENT iconId 3074) -- Max.Held 1, a Lively/Lifeless
	// quest-state pair, only one ever obtainable. Hide the variant.
	// NOT hidden -- genuinely two obtainable, separately-holdable copies in
	// one run (each id is maxNum=1; the pair is stackable, not exclusive):
	// Academy Glintstone Key (8174/8109, same icon, two world pickups),
	// Crimson/Cerulean/Ruptured Crystal Tear (same refId/effect, two Erdtree
	// Avatar drops each) -- matches the user's "keep the Crimson tear" rule.
	0x40001FD2: true, // Miniature Ranni (Variant) (state variant of 0x40001FFB)

	// Flasks (2026-08-01, user request): infinite-heal exploit risk
	// (Crimson/Cerulean refill HP/FP, multiple copies = unlimited healing)
	// plus Wondrous Physick should stay singular/early-game-only -- hidden
	// entirely rather than just de-subcategorized, same as the "Flasks"
	// sub-category itself being removed (see FLASK subcategory below).
	0x400000FA: true, // Flask of Wondrous Physick
	0x400003E9: true, // Flask of Crimson Tears
	0x4000041B: true, // Flask of Cerulean Tears

	// Session-only / default items (2026-08-02, user request): items the
	// player never legitimately keeps as merchant-tradable stock. The three
	// "Phantom" items are temporarily granted while invading another world and
	// consumed on use (verified against eldenring.wiki.gg) -- their permanent
	// counterparts (Bloody Finger, Recusant Finger, Cipher Rings, etc.) stay.
	// Spectral Steed Whistle is a default starting item, not shop stock.
	0x40000082: true, // Spectral Steed Whistle
	0x4000006B: true, // Phantom Bloody Finger
	0x40000072: true, // Phantom Recusant Finger
	0x40000087: true, // Phantom Great Rune

	// gestures (54) -- 0/54 ever appear in a real ShopLineupParam row
	0x40002328: true, // Bow
	0x40002329: true, // Polite Bow
	0x4000232A: true, // My Thanks
	0x4000232B: true, // Curtsy
	0x4000232C: true, // Reverential Bow
	0x4000232D: true, // My Lord
	0x4000232E: true, // Warm Welcome
	0x4000232F: true, // Wave
	0x40002330: true, // Casual Greeting
	0x40002331: true, // Strength!
	0x40002332: true, // As You Wish
	0x40002333: true, // Point Forwards
	0x40002334: true, // Point Upwards
	0x40002335: true, // Point Downwards
	0x40002336: true, // Beckon
	0x40002337: true, // Wait!
	0x40002338: true, // Calm Down!
	0x40002339: true, // Nod In Thought
	0x4000233A: true, // Extreme Repentance
	0x4000233B: true, // Grovel For Mercy
	0x4000233C: true, // Rallying Cry
	0x4000233D: true, // Heartening Cry
	0x4000233E: true, // By My Sword
	0x4000233F: true, // Hoslow's Oath
	0x40002340: true, // Fire Spur Me
	0x40002341: true, // The Carian Oath
	0x40002342: true, // Bravo!
	0x40002343: true, // Jump for Joy
	0x40002344: true, // Triumphant Delight
	0x40002345: true, // Fancy Spin
	0x40002346: true, // Finger Snap
	0x40002347: true, // Dejection
	0x40002348: true, // Patches' Crouch
	0x40002349: true, // Crossed Legs
	0x4000234A: true, // Rest
	0x4000234B: true, // Sitting Sideways
	0x4000234C: true, // Dozing Cross-Legged
	0x4000234D: true, // Spread Out
	0x4000234E: true, // Fetal Position
	0x4000234F: true, // Balled Up
	0x40002350: true, // What Do You Want?
	0x40002351: true, // Prayer
	0x40002352: true, // Desperate Prayer
	0x40002353: true, // Rapture
	0x40002355: true, // Erudition
	0x40002356: true, // Outer Order
	0x40002357: true, // Inner Order
	0x40002358: true, // Golden Order Totality
	0x4000235A: true, // The Ring
	0x401EA7A8: true, // Ring of Miquella
	0x401EA7A9: true, // May the Best Win
	0x401EA7AA: true, // The Two Fingers
	0x401EA7AB: true, // Let Us Go Together
	0x401EA7AC: true, // O Mother

	// info, non-purchasable (84) -- never appear in a real ShopLineupParam
	// row; the 18 real "Note: X"/Weathered Map items stay visible.
	0x40001FBF: true, // Letter from Volcano Manor (Istvan)
	0x40001FC3: true, // Irina's Letter
	0x40001FC4: true, // Letter from Volcano Manor (Rileigh)
	0x40001FC5: true, // Red Letter
	0x40001FE7: true, // Letter to Patches
	0x40001FED: true, // Letter to Bernahl
	0x40001FF5: true, // Burial Crow's Letter
	0x40002008: true, // Homing Instinct Painting
	0x40002009: true, // Resurrection Painting
	0x4000200A: true, // Champion's Song Painting
	0x4000200B: true, // Sorcerer Painting
	0x4000200C: true, // Prophecy Painting
	0x4000200D: true, // Flightless Bird Painting
	0x4000200E: true, // Redmane Painting
	0x4000201D: true, // Zorayas's Letter
	0x4000201F: true, // Rogier's Letter
	0x40002202: true, // Note: Great Coffins
	0x4000220A: true, // Note: Miquella's Needle
	0x4000220C: true, // Note: The Lord of Frenzied Flame
	0x40002312: true, // Sellia's Secret
	0x4000238C: true, // About Sites of Grace
	0x4000238D: true, // About Sorceries and Incantations
	0x4000238E: true, // About Bows
	0x4000238F: true, // About Crouching
	0x40002390: true, // About Stance-Breaking
	0x40002391: true, // About Stakes of Marika
	0x40002392: true, // About Guard Counters
	0x40002393: true, // About the Map
	0x40002394: true, // About Guidance of Grace
	0x40002395: true, // About Horseback Riding
	0x40002396: true, // About Death
	0x40002397: true, // About Summoning Spirits
	0x40002398: true, // About Guarding
	0x40002399: true, // About Item Crafting
	0x4000239B: true, // About Flask of Wondrous Physick
	0x4000239C: true, // About Adding Skills
	0x4000239D: true, // About Birdseye Telescopes
	0x4000239E: true, // About Spiritspring Jumping
	0x4000239F: true, // About Vanquishing Enemy Groups
	0x400023A0: true, // About Teardrop Scarabs
	0x400023A1: true, // About Summoning Other Players
	0x400023A2: true, // About Cooperative Multiplayer
	0x400023A3: true, // About Competitive Multiplayer
	0x400023A4: true, // About Invasion Multiplayer
	0x400023A5: true, // About Hunter Multiplayer
	0x400023A6: true, // About Summoning Pools
	0x400023A7: true, // About Monument Icon
	0x400023A8: true, // About Requesting Help from Hunters
	0x400023A9: true, // About Skills
	0x400023AA: true, // About Fast Travel to Sites of Grace
	0x400023AB: true, // About Strengthening Armaments
	0x400023AC: true, // About Roundtable Hold
	0x400023AE: true, // About Materials
	0x400023AF: true, // About Containers
	0x400023B0: true, // About Adding Affinities
	0x400023B1: true, // About Pouches
	0x400023B2: true, // About Dodging
	0x400023B4: true, // About Wielding Armaments
	0x400023B5: true, // About Great Runes
	0x400023B6: true, // About the Cave of Knowledge
	0x400023BE: true, // About Duels
	0x400023BF: true, // About United Combat and Combat Ordeals
	0x400023C0: true, // About Combat with Spirit Ashes
	0x400023C1: true, // About Marika's Effigy at the Roundtable
	0x400023EB: true, // About Multiplayer
	0x401EA3C7: true, // Cross Map
	0x401EA3CF: true, // Letter for Freyja
	0x401EA3D0: true, // Ruins Map
	0x401EA3D1: true, // Ruins Map (2nd)
	0x401EA3D2: true, // Ruins Map (3rd)
	0x401EA3D9: true, // Furnace Keeper's Note
	0x401EA3DB: true, // Castle Cross Message
	0x401EA3DC: true, // Ancient Ruins Cross Message
	0x401EA3DD: true, // Monk's Missive
	0x401EA3DE: true, // Storehouse Cross Message
	0x401EA3E0: true, // Torn Diary Page
	0x401EA3E2: true, // Message from Leda
	0x401EA3E5: true, // Tower of Shadow Message
	0x401EA488: true, // Incursion Painting
	0x401EA489: true, // The Sacred Tower Painting
	0x401EA48A: true, // Domain of Dragons Painting
	0x401EA848: true, // About the Scadutree Blessing
	0x401EA849: true, // About the Revered Spirit Ash Blessing
	0x401EA84A: true, // About New Inventory Features
}

// categoryOverrides fixes items whose game-data category/subCategory is
// simply wrong (SaveForge's static table, name-derived rather than
// field-derived). Rotten Staff (2026-07-30): its EquipParamWeapon
// weaponCategory=5/wepType=41 matches 19 other Colossal Weapons exactly and
// no real Glintstone Staff (weaponCategory=8/wepType=57) -- confirmed the
// only mismatch across all 479 weapon-table items, see docs/ITEM_IDS.md.
// 2026-08-02 additions (protectors/accessories/gems/tools audit, see
// ITEM_IDS.md): Golden Prosthetic's EquipParamProtector protectorCategory=2
// (arms), not chest -- the only mismatch across all 723 protector items.
// Roundrock/Ruin Fragment's EquipParamGoods signature (sortGroupId=50,
// goodsType=0, refCategory=1) exactly matches the 27 other Throwables items,
// not crafting_materials's own distinct signature (goodsType=2) -- confirmed
// via wiki that both are primarily throwable tools that also double as
// crafting ingredients, not name-guessed (a same-named different item,
// "Scadutree Fragment", was the initial wrong hunch -- these are unrelated).
// Rain of Fire (2026-08-01, found while wiki-sourcing spell school
// sub-categories, see tools/spell_categories): SaveForge tags it
// "sorceries", but eldenring.wiki.gg confirms it's Messmer's DLC
// incantation (Category:Messmer's Flame Incantations) -- no such sorcery
// exists in the base game or Shadow of the Erdtree.
var categoryOverrides = map[uint32]struct{ Category, SubCategory string }{
	0x016116A0: {"melee_armaments", "Colossal Weapons"},          // Rotten Staff
	0x101E1018: {"arms", ""},                                     // Golden Prosthetic
	0x400006E0: {"tools", "Throwables"},                          // Ruin Fragment
	0x401ED2BB: {"tools", "Throwables"},                          // Roundrock
	0x401EA302: {"incantations", "Messmer's Flame Incantations"}, // Rain of Fire
}

// Manual patch: 0x400000FA ("Flask of Wondrous Physick", filled) is a real
// GoodsParam id (has its own description + game-limits entry in SaveForge's
// own db/data package) but is simply missing from tools.go's static
// name/category/icon table -- only its sibling 0x400000FB ("...", empty)
// made it in. Not a data.KeyItems "no_database" exclusion, just an upstream
// gap; same icon as 0x400000FB since both describe the same flask object.
// Confirmed 2026-07-25 (see docs/ITEM_IDS.md), row 101878 (Twin Maiden
// Husks) sells this id.
var manualPatchItems = []itemRecord{
	{ID: 0x400000FA, Name: "Flask of Wondrous Physick", Category: "tools", SubCategory: "Flasks", IconPath: "items/tools/flask_of_wondrous_physick.png"},
}

// toolSubCategoryOverrides: small hand-picked tools sub-category moves,
// 2026-08-01 user request. Perfumed Oil of Ranah reads as a reusable
// tool (an aromatic you apply, not a one-shot Perfume Arts throwable) --
// moved out of "Perfume Arts" (whose remaining 6 items get renamed to
// "Perfumes" below, a blanket rename since they're the correct group,
// just a clearer label).
var toolSubCategoryOverrides = map[uint32]string{
	0x401E90C4: "Reusable Tools", // Perfumed Oil of Ranah
}

// Icon patch: 2 of SaveForge's 20 vendored "Note: X" icon files are simply
// the wrong PNG -- each is byte-for-byte identical to the icon of the real
// item/place the note is *about*, instead of a generic note/parchment
// graphic (confirmed 2026-07-25 by exact icon-hash collision, cross-checked
// against Fextralife: both are genuine pickup-able written messages, not
// the item itself -- e.g. "Note: Flask of Wondrous Physick" is Merchant
// Kalé's hint pointing to the flask's location, sold as its own row). Safe
// to alias to any other note's icon: the game itself reuses one of a
// handful of generic parchment graphics across every "Note:" entry
// (confirmed: 6 of the 18 correctly-iconed notes already share one
// identical file byte-for-byte).
var iconPathOverrides = map[uint32]string{
	0x400021FE: "items/info/note_waypoint_ruins.png", // Note: Flask of Wondrous Physick
	0x4000220A: "items/info/note_waypoint_ruins.png", // Note: Miquella's Needle

	// Full-catalog icon audit (2026-07-25, see docs/ITEM_IDS.md) -- same bug
	// class: each shows a byte-identical copy of a different, unrelated
	// real item's icon, confirmed against Fextralife. 4 SotE bell bearings
	// showed the crafting material they sell instead of a bell -- aliased
	// to pidias_bell_bearing.png, the file SaveForge already uses for every
	// *other* SotE merchant bell bearing (Greasemonger's/Moldmonger's/
	// Spellmachinist's/Igon's/Ymir's).
	0x401EA746: "items/key_items/pidias_bell_bearing.png", // Herbalist's Bell Bearing (was herba.png)
	0x401EA747: "items/key_items/pidias_bell_bearing.png", // Mushroom-Seller's Bell Bearing [1] (was mushroom.png)
	0x401EA748: "items/key_items/pidias_bell_bearing.png", // Mushroom-Seller's Bell Bearing [2] (was mushroom.png)
	0x401EA74D: "items/key_items/pidias_bell_bearing.png", // String-Seller's Bell Bearing (was string.png)
	// Sharp Gravel Stone showed the Gravel Stone Seal (a catalyst), not a
	// stone -- aliased to base Gravel Stone's own (correct) icon.
	0x401ED2C2: "items/crafting_materials/gravel_stone.png", // Sharp Gravel Stone (was gravel_stone_seal.png)
	// Greatsword of Radahn (Light) showed the generic colossal Greatsword;
	// its (Lord) twin -- the same physical sword, differing only in weapon
	// skill per Fextralife -- already has the correct unique icon.
	0x00456D70: "items/melee_armaments/greatsword_of_radahn_lord.png", // Greatsword of Radahn (Light) (was generic greatsword.png)
	// Golden Beast Crest Shield showed the Golden Lion Shield (a different
	// DLC greatshield); correct art vendored 2026-07-28 from
	// eldenring.wiki.gg (game-extracted original, downscaled to 256px --
	// recipe in docs/ITEM_IDS.md).
	0x01E9A790: "items/shields/golden_beast_crest_shield.png", // Golden Beast Crest Shield (was golden_lion_shield.png)
}

// alteredSuffix marks an armor piece's "Altered" (cut-down at Boc's) variant.
// The two are the same physical armor, so a cut-content/ban-risk base piece
// must carry the same warning in its Altered form.
const alteredSuffix = " (Altered)"

// propagateRiskyToAltered copies each armor piece's `risky` flag onto its
// "(Altered)" counterpart, matched by name within the same category.
//
// SaveForge's curated flag list is per-item and misses the Altered halves:
// Ragged Hat is flagged cut_content but Ragged Hat (Altered) is not, even
// though it's the same cut armor (user-reported 2026-08-03; wiki.gg's own
// "Unused Content" gallery lists BOTH halves side by side). Deriving it from
// the base piece rather than hardcoding the three known ids keeps this
// correct if SaveForge's flags are ever regenerated or extended.
//
// Matched by name, NOT by id arithmetic: the base->Altered id delta is not
// constant (Ragged Hat +1000, Brave's Battlewear +1900).
//
// One-directional on purpose -- an Altered piece is only ever obtained by
// altering the base one, so "base is safe but Altered is cut" can't happen
// (verified: zero such pairs in the current data).
func propagateRiskyToAltered(out []itemRecord) {
	type key struct{ category, name string }
	riskyBase := make(map[key]bool)
	for _, it := range out {
		if it.Risky && !strings.HasSuffix(it.Name, alteredSuffix) {
			riskyBase[key{it.Category, it.Name}] = true
		}
	}
	for i, it := range out {
		base, ok := strings.CutSuffix(it.Name, alteredSuffix)
		if !ok || it.Risky {
			continue
		}
		if riskyBase[key{it.Category, base}] {
			out[i].Risky = true
		}
	}
}

func main() {
	items := db.GetAllItems("ps4")

	if err := writeItemDetails(items); err != nil {
		panic(err)
	}

	seen := make(map[uint32]bool, len(items))
	out := make([]itemRecord, 0, len(items))
	for _, it := range items {
		seen[it.ID] = true
		risky := false
		for _, f := range it.Flags {
			if riskyFlags[f] {
				risky = true
				break
			}
		}
		out = append(out, itemRecord{
			ID:          it.ID,
			Name:        it.Name,
			Category:    it.Category,
			SubCategory: it.SubCategory,
			IconPath:    it.IconPath,
			Risky:       risky,
		})
	}

	for id, it := range data.KeyItems {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, itemRecord{
			ID:          id,
			Name:        it.Name,
			Category:    it.Category,
			SubCategory: it.SubCategory,
			IconPath:    it.IconPath,
		})
	}

	for _, it := range manualPatchItems {
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		out = append(out, it)
	}

	for i, it := range out {
		if risky, ok := riskyOverrides[it.ID]; ok {
			out[i].Risky = risky
		}
		if override, ok := iconPathOverrides[it.ID]; ok {
			out[i].IconPath = override
		}
		if hiddenItemIDs[it.ID] {
			out[i].Hidden = true
		}
		if override, ok := categoryOverrides[it.ID]; ok {
			out[i].Category = override.Category
			out[i].SubCategory = override.SubCategory
		}
		if override, ok := keyItemSubCategoryOverrides[it.ID]; ok {
			out[i].SubCategory = override
		}
		if override, ok := aowSubCategoryOverrides[it.ID]; ok {
			out[i].SubCategory = override
		}
		if override, ok := spellSubCategoryOverrides[it.ID]; ok {
			out[i].SubCategory = override
		}
		if override, ok := toolSubCategoryOverrides[it.ID]; ok {
			out[i].SubCategory = override
		}
		// "Perfume Arts" -> "Perfumes" (2026-08-01 user request): a blanket
		// rename, not a per-item override -- the remaining group (everything
		// but Perfumed Oil of Ranah, moved above) is the right set already.
		if out[i].SubCategory == "Perfume Arts" {
			out[i].SubCategory = "Perfumes"
		}
		// Ashes of War that take no affinity (bow/shield/hand-to-hand/unique
		// skills like Parry, No Skill, Mighty Shot) get an explicit "No
		// Affinity" bucket instead of sitting uncategorized (2026-08-02 user
		// request). Applied last so any real affinity above always wins;
		// self-maintaining for future affinity-less skills.
		if out[i].Category == "ashes_of_war" && out[i].SubCategory == "" {
			out[i].SubCategory = "No Affinity"
		}
	}

	propagateRiskyToAltered(out)

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		panic(err)
	}
}
