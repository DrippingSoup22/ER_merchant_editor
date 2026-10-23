#!/usr/bin/env python3
"""Derive internal/assets/data/weapon_reinforce.json: for every weapon-table item.json entry
(equipType 0 -- melee_armaments/shields/ranged_and_catalysts), its real max
upgrade ("+N") level.

Two sources, both read the same way every other table in this project is:
- EquipParamWeapon's `reinforceTypeId` per weapon -- schema from
  soulsmods/Paramdex (tools/paramdex_schema.py, shared with
  tools/paramdex_extract), actual row values decoded from our own fixture
  save's regulation.bin (through tools/param_reader.py).
- ReinforceParamWeapon's row ids, decoded the same way, to find the real
  max level per reinforceTypeId: confirmed empirically 2026-07-28 that
  `row_id = reinforceTypeId + level`, contiguous from level 0 up to the
  weapon's actual max (25 for standard/Smithing-Stone weapons, 10 for
  somber/Somber-Smithing-Stone weapons -- bows, staves, seals, torches,
  and most legendary/boss-drop melee weapons -- and occasionally 0 for a
  handful of genuinely non-reinforceable weapons). No "+10 vs +25 by
  subcategory" guessing: every weapon's real max comes straight from which
  ReinforceParamWeapon rows actually exist.

Regenerate: python3 tools/weapon_reinforce_extract/generate.py --save <baseline>
"""

import json
import argparse
import sys
from pathlib import Path

TOOLS_DIR = Path(__file__).resolve().parents[1]
DATA_DIR = TOOLS_DIR.parent / "internal" / "assets" / "data"
sys.path.insert(0, str(TOOLS_DIR))
from paramdex_schema import build_schema, fetch  # noqa: E402
import param_reader as pr  # noqa: E402

WEAPON_DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/EquipParamWeapon.xml"
REINFORCE_DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/ReinforceParamWeapon.xml"

# Weapon-table categories in items.json (equipType 0).
WEAPON_CATEGORIES = {"melee_armaments", "shields", "ranged_and_catalysts"}


def max_level_for(reinforce_type: int, reinforce_row_ids: set[int]) -> int | None:
    """Largest contiguous level starting at 0 for which reinforce_type+level
    is a real ReinforceParamWeapon row, or None if reinforce_type itself
    isn't a real row (no reinforcement data at all -- shouldn't happen for
    a real weapon, but don't fabricate a level if it does)."""
    if reinforce_type not in reinforce_row_ids:
        return None
    level = 0
    while (reinforce_type + level + 1) in reinforce_row_ids:
        level += 1
    return level


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--save", type=Path, required=True,
                        help="decrypted save whose embedded regulation supplies weapon rows")
    args = parser.parse_args()
    items_doc = json.loads((DATA_DIR / "items.json").read_text())
    items = items_doc["items"] if isinstance(items_doc, dict) and "items" in items_doc else items_doc
    weapon_items = [it for it in items if it["category"] in WEAPON_CATEGORIES]
    print(f"{len(weapon_items)} weapon-table items.json entries")

    weapon_schema = build_schema(fetch(WEAPON_DEF_URL))
    reinforce_schema = build_schema(fetch(REINFORCE_DEF_URL))
    print(f"EquipParamWeapon: {len(weapon_schema['fields'])} fields, row_size={weapon_schema['row_size']}")
    print(f"ReinforceParamWeapon: {len(reinforce_schema['fields'])} fields, row_size={reinforce_schema['row_size']}")
    weapon_param, weapon_param_rows = pr.load_param(args.save, "EquipParamWeapon.param")
    reinforce_param, reinforce_param_rows = pr.load_param(args.save, "ReinforceParamWeapon.param")
    weapon_rows = {r["id"]: r["data_offset"] for r in weapon_param_rows}
    reinforce_row_ids = {r["id"] for r in reinforce_param_rows}
    print(f"EquipParamWeapon rows: {len(weapon_rows)}, ReinforceParamWeapon rows: {len(reinforce_row_ids)}")

    out = []
    missing_row = []
    missing_reinforce = []
    for it in weapon_items:
        item_id = it["id"]
        data_offset = weapon_rows.get(item_id)
        if data_offset is None:
            missing_row.append(it["name"])
            continue
        fields = pr.decode_row_fields(weapon_param, data_offset, weapon_schema)
        reinforce_type = fields["reinforceTypeId"]
        max_level = max_level_for(reinforce_type, reinforce_row_ids)
        if max_level is None:
            missing_reinforce.append((it["name"], reinforce_type))
            continue
        out.append({"itemId": item_id, "maxLevel": max_level})

    if missing_row:
        print(f"WARNING: {len(missing_row)} weapon-table items.json entries have no EquipParamWeapon row: {missing_row[:10]}")
    if missing_reinforce:
        print(f"WARNING: {len(missing_reinforce)} weapons have a reinforceTypeId with no ReinforceParamWeapon row: {missing_reinforce[:10]}")

    out.sort(key=lambda e: e["itemId"])
    (DATA_DIR / "weapon_reinforce.json").write_text(
        json.dumps({"weapons": out}, indent=2) + "\n"
    )
    print(f"wrote {len(out)} weapon_reinforce.json entries")


if __name__ == "__main__":
    main()
