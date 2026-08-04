#!/usr/bin/env python3
"""Derive tools/itemdb_extract/key_item_categories.go: a per-item subCategory
override map that replaces key_items' current 8 subcategories (several of
them dump buckets, e.g. "Inactive Great Runes + Keys + Medallions" holding
133 of 349 items) with a 17-way taxonomy.

Ground truth is EquipParamGoods' own `sortGroupId`/`goodsType`/`refCategory`
fields -- the game's own internal item-sort grouping -- decoded from our
fixture save the same way tools/weapon_reinforce_extract decodes
EquipParamWeapon (reuses tools/paramdex_schema.py + tools/savescan.py, no
new parsing code). A handful of items (which "scroll/prayerbook" items are
Sorcery vs Incantation, whether a couple are real or cut content) aren't
decidable from fields alone -- those are a short explicit list below,
verified against wiki content 2026-07-30 (see docs/ITEM_IDS.md), not
guessed from naming patterns.

Regenerate: tools/.venv/bin/python3 generate.py (run from this directory;
needs the same venv as savescan.py -- cryptography + zstandard).
"""

import json
import subprocess
import sys
from pathlib import Path

TOOLS_DIR = Path(__file__).resolve().parents[1]
DATA_DIR = TOOLS_DIR.parent / "internal" / "assets" / "data"
FIXTURE_SAVE = TOOLS_DIR.parent / "save_files" / "vanilla_fresh_character.dat"
OUT_GO = TOOLS_DIR / "itemdb_extract" / "key_item_categories.go"

sys.path.insert(0, str(TOOLS_DIR))
from paramdex_schema import build_schema, fetch  # noqa: E402
import savescan as sc  # noqa: E402

GOODS_DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/EquipParamGoods.xml"
GOODS_OFFSET = 0x40000000

# Old subcategories that are being replaced. Anything NOT in one of these
# buckets (World Maps, Crystal Tears) is already correct and untouched.
TOUCHED_OLD_SUBCATS = {
    "Inactive Great Runes + Keys + Medallions",
    "Larval Tears + Deathroot + Lost Ashes of War",
    "Cookbooks",
    "Sorcery Scrolls + Incantation Scrolls",
    "Containers + Slot Upgrades",
    "DLC Keys",
}

# Explicit per-name overrides for items that can't be cleanly bucketed by
# field alone (misc sortGroupId 255 stragglers, and the scroll/prayerbook
# Sorcery-vs-Incantation split, verified against wiki content -- see
# docs/ITEM_IDS.md's 2026-07-30 entry. NOT derived from name patterns:
# "Erdtree Codex" would have been misclassified as Sorcery by name alone
# (it's Incantation, confirmed-cut content); "Secret Rite Scroll" would have
# looked like a spell-unlock scroll by name alone (it's a pure NPC-questline
# hand-in item, no spell unlock).
EXPLICIT = {
    # sortGroupId 255 stragglers (even the engine calls this bucket misc)
    "Asimi, Silver Tear": "Quest Items",
    "Iji's Confession": "Quest Items",
    "Asimi's Husk": "Quest Items",
    "Asimi, Silver Chrysalid": "Quest Items",
    "Nomadic Merchant's Bell Bearing [11]": "Merchant Bell Bearings",
    "Fugitive Warrior's Recipe [5]": "Cookbooks",
    # singletons outside the sortGroupId-40 dump bucket
    "Memory of Grace": "Quest Items",
    "Phantom Great Rune": "Great Runes",
    "Great Rune of the Unborn": "Great Runes",
    "Whetstone Knife": "Whetblades",
    # DLC Keys bucket: this one is a map, not a key
    "Cross-Marked Map": "Clues & Maps",
    # Sorcery Scrolls + Incantation Scrolls split (verified via web search,
    # not name-guessed)
    "Conspectus Scroll": "Sorcery Scrolls",
    "Royal House Scroll": "Sorcery Scrolls",
    "Academy Scroll": "Sorcery Scrolls",
    "Fire Monks' Prayerbook": "Incantation Scrolls",
    "Giant's Prayerbook": "Incantation Scrolls",
    "Godskin Prayerbook": "Incantation Scrolls",
    "Two Fingers' Prayerbook": "Incantation Scrolls",
    "Assassin's Prayerbook": "Incantation Scrolls",
    "Erdtree Prayerbook": "Incantation Scrolls",
    "Erdtree Codex": "Incantation Scrolls",
    "Golden Order Principia": "Incantation Scrolls",
    "Golden Order Principles": "Incantation Scrolls",
    "Dragon Cult Prayerbook": "Incantation Scrolls",
    "Ancient Dragon Prayerbook": "Incantation Scrolls",
    "Secret Rite Scroll": "Quest Items",
}


def classify(name: str, old_sub: str, goods_type, ref_category, sort_group) -> str:
    if name in EXPLICIT:
        return EXPLICIT[name]

    if old_sub == "Inactive Great Runes + Keys + Medallions":
        if sort_group == 40:
            if "Great Rune" in name:
                return "Great Runes"
            if name.startswith("Mending Rune"):
                return "Mending Runes"
            return "Keys & Medallions"
        if sort_group == 50:
            return "Quest Items"
        if sort_group == 80:
            return "Merchant Bell Bearings"
        if sort_group == 90:
            return "Crafting Bell Bearings"
        if goods_type == 12:
            return "Clues & Maps"
        raise ValueError(f"unhandled dump-bucket item {name!r} sig={(goods_type, ref_category, sort_group)}")

    if old_sub == "Larval Tears + Deathroot + Lost Ashes of War":
        return "Quest Items"

    if old_sub == "Cookbooks":
        if "Whetblade" in name:
            return "Whetblades"
        if "Cookbook" in name:
            return "Cookbooks"
        return "Crafting Tools"

    if old_sub == "Containers + Slot Upgrades":
        return "Containers" if goods_type == 11 else "Slot Upgrades"

    if old_sub == "DLC Keys":
        return "DLC Keys"

    raise ValueError(f"unhandled old subcategory {old_sub!r} for {name!r}")


def main():
    items = json.loads((DATA_DIR / "items.json").read_text())
    key_items = [it for it in items if it["category"] == "key_items"]
    print(f"{len(key_items)} key_items entries")

    schema = build_schema(fetch(GOODS_DEF_URL))
    blob = sc._decoded_bnd4(str(FIXTURE_SAVE))
    goods_param = sc.extract_bnd4_entry(blob, "EquipParamGoods.param")
    goods_header = sc.parse_param_header(goods_param)
    goods_rows = {r["id"]: r["data_offset"] for r in sc.iter_param_rows(goods_param, goods_header)}

    overrides = {}  # id -> new subCategory (only when it differs from old)
    new_counts = {}
    for it in key_items:
        old_sub = it.get("subCategory")
        raw_id = it["id"] - GOODS_OFFSET
        offset = goods_rows.get(raw_id)
        fields = sc.decode_row_fields(goods_param, offset, schema) if offset is not None else {}
        goods_type = fields.get("goodsType")
        ref_category = fields.get("refCategory")
        sort_group = fields.get("sortGroupId")

        if it["name"] in EXPLICIT:
            new_sub = EXPLICIT[it["name"]]
        elif old_sub in TOUCHED_OLD_SUBCATS:
            new_sub = classify(it["name"], old_sub, goods_type, ref_category, sort_group)
        else:
            new_sub = old_sub  # World Maps / Crystal Tears -- already correct

        new_counts[new_sub] = new_counts.get(new_sub, 0) + 1
        if new_sub != old_sub:
            overrides[it["id"]] = (new_sub, it["name"])

    assert sum(new_counts.values()) == len(key_items), "count mismatch"
    print(f"{len(overrides)} items get a new subCategory")
    for sub, count in sorted(new_counts.items(), key=lambda kv: -kv[1]):
        print(f"  {sub}: {count}")

    lines = [
        "// Code generated by tools/key_item_categories/generate.py. DO NOT EDIT.",
        "// Ground truth: EquipParamGoods sortGroupId/goodsType/refCategory decoded",
        "// from the fixture save, plus a short wiki-verified list for the handful",
        "// of items fields alone can't settle -- see docs/ITEM_IDS.md.",
        "package main",
        "",
        "// keyItemSubCategoryOverrides replaces key_items' old dump-bucket",
        "// subcategories (\"Inactive Great Runes + Keys + Medallions\" etc, see",
        "// ITEM_IDS.md) with a 17-way taxonomy. Applied in main()'s override loop.",
        "var keyItemSubCategoryOverrides = map[uint32]string{",
    ]
    for item_id in sorted(overrides):
        new_sub, name = overrides[item_id]
        lines.append(f"\t0x{item_id:08X}: {json.dumps(new_sub)}, // {name}")
    lines.append("}")
    lines.append("")

    OUT_GO.write_text("\n".join(lines) + "\n")
    subprocess.run(["gofmt", "-w", str(OUT_GO)], check=True)
    print(f"wrote {OUT_GO}")


if __name__ == "__main__":
    main()
