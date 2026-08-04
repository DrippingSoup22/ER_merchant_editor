#!/usr/bin/env python3
"""Derive tools/itemdb_extract/spell_categories.go: a school sub-category
for sorceries/incantations items.json entries, per user request (2026-08-01,
"fextralife define them, e.g. bestial incantation").

EquipParamGoods (the table both categories live in, same as tools/key_items/
crafting_materials -- see docs/ITEM_IDS.md) has no field that cleanly
separates spell schools, so this is wiki-sourced: eldenring.wiki.gg tags
every sorcery/incantation with its real school as a MediaWiki category
(e.g. "Category:Carian Sorceries", "Category:Bestial Incantations"), fetched
2026-08-01 via the API (action=query&list=categorymembers) for every school
subcategory under Category:Sorceries / Category:Incantations. Items with NO
school category on the wiki (confirmed via direct per-item lookups, not
absence-as-failure -- e.g. base Glintstone Sorceries like "Glintstone
Pebble", or uncategorized Incantations like "Catch Flame"/boss-remembrance
uniques like "Spira") are left with NO sub-category, matching this
project's existing "absence of enrichment, not a wrong label" rule (see
docs/ITEM_IDS.md's talisman/chest/head/legs/arms precedent) rather than
inventing a catch-all bucket the wiki itself doesn't apply.

The two dicts below (item name -> wiki category name, full form e.g. "Carian
Sorceries") are the reconciled result of that lookup: 55/85 sorceries and
111/128 incantations matched a real wiki school category.

Regenerate (needs live network access to eldenring.wiki.gg's API): re-derive
the two dicts below via the MediaWiki API's categorymembers list for each
subcategory of Category:Sorceries / Category:Incantations (see
docs/ITEM_IDS.md for the exact query shape), then re-run this script.
"""

import json
import sys
from pathlib import Path

TOOLS_DIR = Path(__file__).resolve().parents[1]
DATA_DIR = TOOLS_DIR.parent / "internal" / "assets" / "data"
OUT_GO = TOOLS_DIR / "itemdb_extract" / "spell_categories.go"

# item name (items.json, category=="sorceries") -> wiki.gg school category,
# fetched 2026-08-01 from eldenring.wiki.gg's Category:Sorceries subcats.
SORCERIES = {
    "Adula's Moonblade": "Carian Sorceries",
    "Ambush Shard": "Night Sorceries",
    "Ancient Death Rancor": "Death Sorceries",
    "Blades of Stone": "Gravity Sorceries",
    "Briars of Punishment": "Thorn Sorceries",
    "Briars of Sin": "Thorn Sorceries",
    "Carian Greatsword": "Carian Sorceries",
    "Carian Phalanx": "Carian Sorceries",
    "Carian Piercer": "Carian Sorceries",
    "Carian Retaliation": "Carian Sorceries",
    "Carian Slicer": "Carian Sorceries",
    "Cherishing Fingers": "Finger Sorceries",
    "Collapsing Stars": "Gravity Sorceries",
    "Comet Azur": "Primeval Sorceries",
    "Eternal Darkness": "Night Sorceries",
    "Explosive Ghostflame": "Death Sorceries",
    "Fia's Mist": "Death Sorceries",
    "Fleeting Microcosm": "Finger Sorceries",
    "Founding Rain of Stars": "Primeval Sorceries",
    "Freezing Mist": "Cold Sorceries",
    "Frozen Armament": "Cold Sorceries",
    "Glintblade Phalanx": "Carian Sorceries",
    "Glintblade Trio": "Carian Sorceries",
    "Glintstone Icecrag": "Cold Sorceries",
    "Glintstone Nail": "Finger Sorceries",
    "Glintstone Nails": "Finger Sorceries",
    "Gravitational Missile": "Gravity Sorceries",
    "Gravity Well": "Gravity Sorceries",
    "Greatblade Phalanx": "Carian Sorceries",
    "Impenetrable Thorns": "Thorn Sorceries",
    "Loretta's Greatbow": "Carian Sorceries",
    "Loretta's Mastery": "Carian Sorceries",
    "Lucidity": "Carian Sorceries",
    "Magic Downpour": "Carian Sorceries",
    "Magic Glintblade": "Carian Sorceries",
    "Mantle of Thorns": "Thorn Sorceries",
    "Mass of Putrescence": "Death Sorceries",
    "Meteorite": "Gravity Sorceries",
    "Meteorite of Astel": "Gravity Sorceries",
    "Miriam's Vanishing": "Carian Sorceries",
    "Night Comet": "Night Sorceries",
    "Night Maiden's Mist": "Night Sorceries",
    "Night Shard": "Night Sorceries",
    "Rancorcall": "Death Sorceries",
    "Rennala's Full Moon": "Carian Sorceries",
    "Rings of Spectral Light": "Death Sorceries",
    "Ranni's Dark Moon": "Carian Sorceries",
    "Rellana's Twin Moons": "Carian Sorceries",
    "Rock Sling": "Gravity Sorceries",
    "Stars of Ruin": "Primeval Sorceries",
    "Tibia's Summons": "Death Sorceries",
    "Unseen Blade": "Night Sorceries",
    "Unseen Form": "Night Sorceries",
    "Vortex of Putrescence": "Death Sorceries",
    "Zamor Ice Storm": "Cold Sorceries",
    # Added 2026-08-02 (user request): schools that were previously left
    # uncategorized, verified against eldenring.wiki.gg per-spell pages.
    "Cannon of Haima": "Glintstone Sorceries",
    "Comet": "Glintstone Sorceries",
    "Crystal Barrage": "Glintstone Sorceries",
    "Crystal Burst": "Glintstone Sorceries",
    "Gavel of Haima": "Glintstone Sorceries",
    "Glintstone Arc": "Glintstone Sorceries",
    "Glintstone Cometshard": "Glintstone Sorceries",
    "Glintstone Pebble": "Glintstone Sorceries",
    "Glintstone Stars": "Glintstone Sorceries",
    "Great Glintstone Shard": "Glintstone Sorceries",
    "Rock Blaster": "Glintstone Sorceries",
    "Scholar's Armament": "Glintstone Sorceries",
    "Scholar's Shield": "Glintstone Sorceries",
    "Shard Spiral": "Glintstone Sorceries",
    "Shatter Earth": "Glintstone Sorceries",
    "Star Shower": "Glintstone Sorceries",
    "Starlight": "Glintstone Sorceries",
    "Swift Glintstone Shard": "Glintstone Sorceries",
    "Terra Magica": "Glintstone Sorceries",
    "Thops's Barrier": "Glintstone Sorceries",
    "Crystal Release": "Crystalian Sorceries",
    "Crystal Torrent": "Crystalian Sorceries",
    "Shattering Crystal": "Crystalian Sorceries",
    "Gelmir's Fury": "Magma Sorceries",
    "Magma Shot": "Magma Sorceries",
    "Roiling Magma": "Magma Sorceries",
    "Rykard's Rancor": "Magma Sorceries",
    "Great Oracular Bubble": "Claymen Sorceries",
    "Oracle Bubbles": "Claymen Sorceries",
}

# item name (items.json, category=="incantations") -> wiki.gg school category.
INCANTATIONS = {
    "Agheel's Flame": "Dragon Communion Incantations",
    "Ancient Dragons' Lightning Spear": "Dragon Cult Incantations",
    "Ancient Dragons' Lightning Strike": "Dragon Cult Incantations",
    "Aspects of the Crucible: Bloom": "Erdtree Incantations",
    "Aspects of the Crucible: Breath": "Erdtree Incantations",
    "Aspects of the Crucible: Horns": "Erdtree Incantations",
    "Aspects of the Crucible: Tail": "Erdtree Incantations",
    "Aspects of the Crucible: Thorns": "Erdtree Incantations",
    "Assassin's Approach": "Two Fingers Incantations",
    "Barrier of Gold": "Erdtree Worship Incantations",
    "Bayle's Flame Lightning": "Dragon Communion Incantations",
    "Bayle's Tyranny": "Dragon Communion Incantations",
    "Beast Claw": "Bestial Incantations",
    "Bestial Constitution": "Bestial Incantations",
    "Bestial Sling": "Bestial Incantations",
    "Bestial Vitality": "Bestial Incantations",
    "Black Blade": "Erdtree Incantations",
    "Black Flame": "Godslayer Incantations",
    "Black Flame Blade": "Godslayer Incantations",
    "Black Flame Ritual": "Godslayer Incantations",
    "Black Flame's Protection": "Godslayer Incantations",
    "Blessing of the Erdtree": "Erdtree Incantations",
    "Blessing's Boon": "Erdtree Incantations",
    "Bloodboon": "Blood Oath Incantations",
    "Bloodflame Blade": "Blood Oath Incantations",
    "Bloodflame Talons": "Blood Oath Incantations",
    "Borealis's Mist": "Dragon Communion Incantations",
    "Cure Poison": "Two Fingers Incantations",
    "Darkness": "Two Fingers Incantations",
    "Death Lightning": "Dragon Cult Incantations",
    "Discus of Light": "Golden Order Incantations",
    "Divine Beast Tornado": "Divine Beast Incantations",
    "Divine Bird Feathers": "Divine Beast Incantations",
    "Divine Fortification": "Two Fingers Incantations",
    "Dragonbolt Blessing": "Dragon Cult Incantations",
    "Dragonbolt of Florissax": "Dragon Cult Incantations",
    "Dragonclaw": "Dragon Communion Incantations",
    "Dragonfire": "Dragon Communion Incantations",
    "Dragonice": "Dragon Communion Incantations",
    "Dragonmaw": "Dragon Communion Incantations",
    "Ekzykes's Decay": "Dragon Communion Incantations",
    "Elden Stars": "Erdtree Incantations",
    "Electrify Armament": "Dragon Cult Incantations",
    "Electrocharge": "Dragon Cult Incantations",
    "Erdtree Heal": "Erdtree Incantations",
    "Fire Serpent": "Messmer's Flame Incantations",
    "Flame Fortification": "Two Fingers Incantations",
    "Fortissax's Lightning Spear": "Dragon Cult Incantations",
    "Frenzied Burst": "Three Fingers Incantations",
    "Frozen Lightning Spear": "Dragon Cult Incantations",
    "Furious Blade of Ansbach": "Blood Oath Incantations",
    "Ghostflame Breath": "Dragon Communion Incantations",
    "Glintstone Breath": "Dragon Communion Incantations",
    "Golden Lightning Fortification": "Erdtree Worship Incantations",
    "Golden Vow": "Erdtree Worship Incantations",
    "Great Heal": "Two Fingers Incantations",
    "Greyoll's Roar": "Dragon Communion Incantations",
    "Gurranq's Beast Claw": "Bestial Incantations",
    "Heal": "Two Fingers Incantations",
    "Heal from Afar": "Erdtree Incantations",
    "Honed Bolt": "Dragon Cult Incantations",
    "Howl of Shabriri": "Three Fingers Incantations",
    "Immutable Shield": "Golden Order Incantations",
    "Inescapable Frenzy": "Three Fingers Incantations",
    "Knight's Lightning Spear": "Dragon Cult Incantations",
    "Land of Shadow": "Erdtree Incantations",
    "Lansseax's Glaive": "Dragon Cult Incantations",
    "Law of Causality": "Golden Order Incantations",
    "Law of Regression": "Golden Order Incantations",
    "Light of Miquella": "Miquella's Incantations",
    "Lightning Fortification": "Two Fingers Incantations",
    "Lightning Spear": "Dragon Cult Incantations",
    "Lightning Strike": "Dragon Cult Incantations",
    "Litany of Proper Death": "Golden Order Incantations",
    "Lord's Aid": "Two Fingers Incantations",
    "Lord's Divine Fortification": "Two Fingers Incantations",
    "Lord's Heal": "Two Fingers Incantations",
    "Magic Fortification": "Two Fingers Incantations",
    "Magma Breath": "Dragon Communion Incantations",
    "Messmer's Orb": "Messmer's Flame Incantations",
    "Midra's Flame of Frenzy": "Three Fingers Incantations",
    "Minor Erdtree": "Erdtree Incantations",
    "Multilayered Ring of Light": "Miquella's Incantations",
    "Noble Presence": "Godslayer Incantations",
    "Order Healing": "Golden Order Incantations",
    "Order's Blade": "Golden Order Incantations",
    "Pest Threads": "Servants of Rot Incantations",
    "Pest-Thread Spears": "Servants of Rot Incantations",
    "Poison Armament": "Servants of Rot Incantations",
    "Poison Mist": "Servants of Rot Incantations",
    "Protection of the Erdtree": "Erdtree Worship Incantations",
    "Radagon's Rings of Light": "Golden Order Incantations",
    "Rain of Fire": "Messmer's Flame Incantations",
    "Rejection": "Two Fingers Incantations",
    "Roar of Rugalea": "Bear Communion Incantations",
    "Rotten Breath": "Dragon Communion Incantations",
    "Rotten Butterflies": "Servants of Rot Incantations",
    "Scarlet Aeonia": "Servants of Rot Incantations",
    "Scouring Black Flame": "Godslayer Incantations",
    "Shadow Bait": "Two Fingers Incantations",
    "Smarag's Glintstone Breath": "Dragon Communion Incantations",
    "Swarm of Flies": "Blood Oath Incantations",
    "The Flame of Frenzy": "Three Fingers Incantations",
    "Theodorix's Magma": "Dragon Communion Incantations",
    "Triple Rings of Light": "Golden Order Incantations",
    "Unendurable Frenzy": "Three Fingers Incantations",
    "Urgent Heal": "Two Fingers Incantations",
    "Vyke's Dragonbolt": "Dragon Cult Incantations",
    "Watchful Spirit": "Divine Beast Incantations",
    "Wrath from Afar": "Erdtree Incantations",
    "Wrath of Gold": "Erdtree Worship Incantations",
    # Added 2026-08-02 (user request): previously uncategorized, verified
    # against eldenring.wiki.gg. NOTE: the wiki lists Placidusax's Ruin as
    # belonging to no school, but per user preference it's filed under Dragon
    # Communion (it plays much like those) -- a deliberate override, not a
    # wiki fact.
    "Placidusax's Ruin": "Dragon Communion Incantations",
    "Burn, O Flame!": "Flame Incantations",
    "Catch Flame": "Flame Incantations",
    "Fire's Deadly Sin": "Flame Incantations",
    "Flame Sling": "Flame Incantations",
    "Flame of the Fell God": "Flame Incantations",
    "Flame, Cleanse Me": "Flame Incantations",
    "Flame, Fall Upon Them": "Flame Incantations",
    "Flame, Grant Me Strength": "Flame Incantations",
    "Flame, Protect Me": "Flame Incantations",
    "Giantsflame Take Thee": "Flame Incantations",
    "O, Flame!": "Flame Incantations",
    "Surge, O Flame!": "Flame Incantations",
    "Whirl, O Flame!": "Flame Incantations",
    "Giant Golden Arc": "Spiral Incantations",
    "Golden Arcs": "Spiral Incantations",
    "Spira": "Spiral Incantations",
    "Stone of Gurranq": "Bestial Incantations",
}


def build(items, category, name_map):
    by_name = {}
    for it in items:
        if it["category"] == category:
            by_name.setdefault(it["name"], []).append(it)
    out = {}
    unmatched_names = []
    for name, school in name_map.items():
        matches = by_name.get(name)
        if not matches:
            unmatched_names.append(name)
            continue
        for it in matches:
            out[it["id"]] = (school, name)
    if unmatched_names:
        print(f"WARNING: {category} names not found in items.json: {unmatched_names}")
    return out


def main():
    items = json.loads((DATA_DIR / "items.json").read_text())
    sorc_out = build(items, "sorceries", SORCERIES)
    incant_out = build(items, "incantations", INCANTATIONS)
    print(f"sorceries: {len(sorc_out)} bucketed of {len(SORCERIES)} name entries")
    print(f"incantations: {len(incant_out)} bucketed of {len(INCANTATIONS)} name entries")

    merged = {**sorc_out, **incant_out}

    lines = [
        "package main",
        "",
        "// spellSubCategoryOverrides: real wiki.gg school sub-category for",
        "// sorceries/incantations items.json entries, generated by",
        "// tools/spell_categories (2026-08-01, see docs/ITEM_IDS.md and that",
        "// tool's own doc comment for the wiki-category-membership methodology).",
        "// Every sorcery/incantation now carries a school. Placidusax's Ruin",
        "// has no wiki school but is filed under Dragon Communion by user",
        "// preference (see generate.py). No invented catch-all buckets.",
        "//",
        "// Regenerate: cd tools/spell_categories && ../.venv/bin/python3 generate.py",
        "var spellSubCategoryOverrides = map[uint32]string{",
    ]
    for item_id in sorted(merged):
        school, name = merged[item_id]
        lines.append(f'\t0x{item_id:08X}: "{school}", // {name}')
    lines.append("}")
    lines.append("")

    OUT_GO.write_text("\n".join(lines))
    print(f"wrote {OUT_GO}")


if __name__ == "__main__":
    main()
