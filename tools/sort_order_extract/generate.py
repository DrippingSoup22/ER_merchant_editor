#!/usr/bin/env python3
"""Derive internal/assets/data/item_sort_order.json: {itemId: {sortId, sortGroupId}} for
every items.json entry that has real game-defined sort data, per user
request (2026-08-01): reorder the Catalog grid to match the game's own
in-menu order instead of raw item-id order.

Every EquipParam* table carries the same two sort columns FromSoftware's
own UI reads (confirmed via soulsmods/Paramdex schemas): `sortId` (s32,
position within a group) and `sortGroupId` (u8, "Type" filter group,
e.g. 10=dagger/20=straight sword for weapons). This mirrors
EldenRing-SaveForge's own `data.ItemSortKeys`, but re-derived independently
via this project's house method (tools/paramdex_schema.py +
tools/savescan.py against our own fixture save) rather than depending on
SaveForge's CSV-dump pipeline (tools/import_sort_ids.go), which needs a
regulation.bin CSV export we don't have checked out -- same reasoning as
weapon_reinforce_extract/vanilla_shop_lineup_extract.

Covers all 5 tables the equipType offsets map to (see docs/ITEM_IDS.md):
EquipParamWeapon/Protector/Accessory (melee/ranged/shields/armor/talismans),
EquipParamGem (ashes_of_war), EquipParamGoods (tools/crafting_materials/
bolstering_materials/sorceries/incantations/key_items/info/gestures/ashes --
every "Goods"-offset category). Items with no sortId (or sortId==0, which
Paramdex disables) are simply absent from the output; internal/catalog falls
back to id order for those.

Regenerate: tools/.venv/bin/python3 generate.py (run from this directory;
needs the same venv as savescan.py).
"""

import json
import sys
from pathlib import Path

TOOLS_DIR = Path(__file__).resolve().parents[1]
DATA_DIR = TOOLS_DIR.parent / "internal" / "assets" / "data"
FIXTURE_SAVE = TOOLS_DIR.parent / "save_files" / "vanilla_fresh_character.dat"

sys.path.insert(0, str(TOOLS_DIR))
from paramdex_schema import build_schema, fetch  # noqa: E402
import savescan as sc  # noqa: E402

PARAMDEX_BASE = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs"

# (param table, BND4 entry name, item-id offset -- see docs/ITEM_IDS.md's
# equipTypeItemIDOffset) for each equip-type-offset space.
TABLES = [
    ("EquipParamWeapon", "EquipParamWeapon.param", 0x00000000),
    ("EquipParamProtector", "EquipParamProtector.param", 0x10000000),
    ("EquipParamAccessory", "EquipParamAccessory.param", 0x20000000),
    ("EquipParamGoods", "EquipParamGoods.param", 0x40000000),
    ("EquipParamGem", "EquipParamGem.param", 0x80000000),
]


def main():
    items = json.loads((DATA_DIR / "items.json").read_text())
    item_ids = {it["id"] for it in items}
    print(f"{len(item_ids)} items.json entries")

    blob = sc._decoded_bnd4(str(FIXTURE_SAVE))

    out = {}
    for table, entry_name, offset in TABLES:
        schema = build_schema(fetch(f"{PARAMDEX_BASE}/{table}.xml"))
        param = sc.extract_bnd4_entry(blob, entry_name)
        header = sc.parse_param_header(param)
        rows = sc.iter_param_rows(param, header)
        hits = 0
        for row in rows:
            item_id = row["id"] + offset
            if item_id not in item_ids:
                continue
            fields = sc.decode_row_fields(param, row["data_offset"], schema)
            sort_id = fields.get("sortId")
            sort_group = fields.get("sortGroupId")
            if sort_id is None or sort_id == 0:
                continue
            out[item_id] = {"sortId": sort_id, "sortGroupId": sort_group}
            hits += 1
        print(f"{table}: {len(rows)} rows, {hits} matched a real items.json id with sortId != 0")

    coverage = len(out) / len(item_ids)
    print(f"{len(out)}/{len(item_ids)} items.json entries got sort data ({coverage:.0%})")

    (DATA_DIR / "item_sort_order.json").write_text(
        json.dumps({str(k): v for k, v in sorted(out.items())}, indent=2) + "\n"
    )
    print("wrote internal/assets/data/item_sort_order.json")


if __name__ == "__main__":
    main()
