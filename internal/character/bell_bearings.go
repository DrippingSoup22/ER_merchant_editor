package character

// BellBearings come from EldenRing-SaveForge's bell_bearing_flags.go and
// bell_bearings.go (GPL-3.0), which credit er-save-manager's
// event_flags_db.py.
//
// Two mechanisms share the name. Covered entries (about 15: the Peddler,
// Miner and Glovewort-Picker kind, and the DLC food) gate goods that are
// already rows in Twin Maiden Husks' own block (101800-101899), behind the
// same flag as those rows' eventFlag_forRelease, so the per-row checkboxes
// already show them and BellBearingsForUI leaves them out. The others (about
// 40, merchants with a shop elsewhere) appear in no ShopLineupParam row: the
// flag is a talk-script gate that lets her open that merchant's own range, and
// they get checkboxes of their own.
//
// Merchant is the merchant_catalog.json name: the 15 "npc" entries match by
// name; of the 18 numbered wandering merchants only the 8 with a confident
// location match are named, the rest stay "" rather than guessed.
type BellBearing struct {
	FlagID   uint32
	Name     string // e.g. "Patches' Bell Bearing"
	Merchant string // canonical internal/assets/data/merchant_catalog.json name, "" if not confidently mappable
	Covered  bool   // true = already an ordinary Twin Maiden Husks row checkbox; excluded from BellBearingsForUI
	// Category is SaveForge's own taxonomy for this entry (npc/merchant/
	// peddler/smithing/dlc), used by the Characters view purely for
	// bucketing (Merchant Bell Bearings vs NPC Bell Bearings, among the
	// Covered==false entries).
	Category string
}

// BellBearings is the full ported table, FlagID order matching the
// source.
var BellBearings = []BellBearing{
	{11109710, "Pidia's Bell Bearing", "Pidia Carian Servant", false, "npc"},
	{11109711, "Seluvis's Bell Bearing", "Preceptor Seluvis", false, "npc"},
	{11109712, "Patches' Bell Bearing", "Patches", false, "npc"},
	{11109713, "Sellen's Bell Bearing", "Sorceress Sellen", false, "npc"},
	{11109715, "D's Bell Bearing", "D Hunter of The Dead", false, "npc"},
	{11109716, "Bernahl's Bell Bearing", "Knight Bernahl", false, "npc"},
	{11109717, "Miriel's Bell Bearing", "Miriel", false, "npc"},
	{11109718, "Gostoc's Bell Bearing", "Gatekeeper Gostoc", false, "npc"},
	{11109719, "Thops's Bell Bearing", "Sorcerer Thops", false, "npc"},
	{11109720, "Kalé's Bell Bearing", "Merchant Kale", false, "npc"},
	{11109721, "Nomadic Merchant's Bell Bearing [1]", "", false, "merchant"},
	{11109722, "Nomadic Merchant's Bell Bearing [2]", "", false, "merchant"},
	{11109723, "Nomadic Merchant's Bell Bearing [3]", "", false, "merchant"},
	{11109724, "Nomadic Merchant's Bell Bearing [4]", "", false, "merchant"},
	{11109725, "Nomadic Merchant's Bell Bearing [5]", "", false, "merchant"},
	{11109726, "Isolated Merchant's Bell Bearing [1]", "Isolated Merchant - Weeping Peninsula", false, "merchant"},
	{11109727, "Isolated Merchant's Bell Bearing [2]", "Isolated Merchant - Academy of Raya Lucaria", false, "merchant"},
	{11109728, "Nomadic Merchant's Bell Bearing [6]", "", false, "merchant"},
	{11109729, "Hermit Merchant's Bell Bearing [1]", "Hermit Merchant - Leyndell", false, "merchant"},
	{11109730, "Nomadic Merchant's Bell Bearing [7]", "Nomadic Merchant - Altus Plateau", false, "merchant"},
	{11109731, "Nomadic Merchant's Bell Bearing [8]", "Nomadic Merchant - Mt. Gelmir", false, "merchant"},
	{11109732, "Nomadic Merchant's Bell Bearing [9]", "", false, "merchant"},
	{11109733, "Nomadic Merchant's Bell Bearing [10]", "", false, "merchant"},
	{11109735, "Isolated Merchant's Bell Bearing [3]", "Isolated Merchant - Dragonbarrow", false, "merchant"},
	{11109736, "Hermit Merchant's Bell Bearing [2]", "Hermit Merchant - Mountaintops of the Giants", false, "merchant"},
	{11109737, "Abandoned Merchant's Bell Bearing", "Abandoned Merchant - Siofra River", false, "merchant"},
	{11109738, "Hermit Merchant's Bell Bearing [3]", "Hermit Merchant - Ainsel River", false, "merchant"},
	{11109739, "Imprisoned Merchant's Bell Bearing", "Imprisoned Merchant - Mohgwyn", false, "merchant"},
	{11109740, "Iji's Bell Bearing", "Iji", false, "npc"},
	{11109741, "Rogier's Bell Bearing", "Sorcerer Rogier", false, "npc"},
	{11109742, "Blackguard's Bell Bearing", "Blackguard Big Boggart", false, "npc"},
	{11109743, "Corhyn's Bell Bearing", "Brother Corhyn", false, "npc"},
	{11109744, "Gowry's Bell Bearing", "Gowry", false, "npc"},
	{11109745, "Bone Peddler's Bell Bearing", "", true, "peddler"},
	{11109746, "Meat Peddler's Bell Bearing", "", true, "peddler"},
	{11109747, "Medicine Peddler's Bell Bearing", "", true, "peddler"},
	{11109748, "Gravity Stone Peddler's Bell Bearing", "", true, "peddler"},
	{11109751, "Smithing-Stone Miner's Bell Bearing [1]", "", true, "smithing"},
	{11109752, "Smithing-Stone Miner's Bell Bearing [2]", "", true, "smithing"},
	{11109753, "Smithing-Stone Miner's Bell Bearing [3]", "", true, "smithing"},
	{11109754, "Smithing-Stone Miner's Bell Bearing [4]", "", true, "smithing"},
	{11109755, "Somberstone Miner's Bell Bearing [1]", "", true, "smithing"},
	{11109756, "Somberstone Miner's Bell Bearing [2]", "", true, "smithing"},
	{11109757, "Somberstone Miner's Bell Bearing [3]", "", true, "smithing"},
	{11109758, "Somberstone Miner's Bell Bearing [4]", "", true, "smithing"},
	{11109759, "Somberstone Miner's Bell Bearing [5]", "", true, "smithing"},
	{11109760, "Glovewort Picker's Bell Bearing [1]", "", true, "smithing"},
	{11109761, "Glovewort Picker's Bell Bearing [2]", "", true, "smithing"},
	{11109762, "Glovewort Picker's Bell Bearing [3]", "", true, "smithing"},
	{11109763, "Ghost-Glovewort Picker's Bell Bearing [1]", "", true, "smithing"},
	{11109764, "Ghost-Glovewort Picker's Bell Bearing [2]", "", true, "smithing"},
	{11109765, "Ghost-Glovewort Picker's Bell Bearing [3]", "", true, "smithing"},
	{11109790, "Moore's Bell Bearing", "Moore", false, "dlc"},
	{11109791, "Ymir's Bell Bearing", "Count Ymir", false, "dlc"},
	{11109792, "Herbalist's Bell Bearing", "", true, "dlc"},
	{11109793, "Mushroom-Seller's Bell Bearing [1]", "", true, "dlc"},
	{11109794, "Mushroom-Seller's Bell Bearing [2]", "", true, "dlc"},
	{11109795, "Greasemonger's Bell Bearing", "", true, "dlc"},
	{11109796, "Moldmonger's Bell Bearing", "", true, "dlc"},
	{11109797, "Igon's Bell Bearing", "", true, "dlc"},
	{11109798, "Spell-Machinist Bell Bearing", "", false, "dlc"},
	{11109799, "String-seller's Bell Bearing", "", true, "dlc"},
}

// BellBearingsForUI returns BellBearings filtered to the entries that
// need new UI — Covered==false, i.e. not already exposed as an ordinary
// Twin Maiden Husks row checkbox — in table order.
func BellBearingsForUI() []BellBearing {
	out := make([]BellBearing, 0, len(BellBearings))
	for _, b := range BellBearings {
		if !b.Covered {
			out = append(out, b)
		}
	}
	return out
}
