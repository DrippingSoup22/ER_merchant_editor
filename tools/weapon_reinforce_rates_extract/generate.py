#!/usr/bin/env python3
"""Derive internal/assets/data/weapon_reinforce_rates.json: the per-upgrade-level multiplier
curves the game applies to a weapon's base (+0) attack / scaling / guard
stats, so the item-info popup can show a weapon's stats AT its actual "+N"
level instead of always +0 (user request 2026-08-03: "a max level weapon has
much higher scaling than a +0 one").

Elden Ring stores every weapon's base combat stats at +0 (EquipParamWeapon)
and a `reinforceTypeId`; the stats at level L are the base times the rate
columns of ReinforceParamWeapon row (reinforceTypeId + L). Standard smithing
weapons use reinforceTypeId 0 (+25 max); somber weapons 2200 (+10 max); a few
unique/boss weapons have their own type. Reading each weapon's OWN
reinforceTypeId is the correct "standard scaling" for a base merchant weapon
(no Ash-of-War affinity infusion changes the base row) -- see docs/ITEM_IDS.md.

Output:
  {
    "columns": [...15 rate names, documenting the per-level arrays...],
    "types":   { "<reinforceTypeId>": [ [15 floats] per level 0..max ], ... },
    "weapons": { "<itemId>": <reinforceTypeId>, ... }
  }
Only reinforceTypeIds actually used by an items.json weapon-category entry are
emitted, and only weapon-category items appear in "weapons".

Regenerate: tools/.venv/bin/python3 generate.py (run from this directory;
needs the same venv as savescan.py -- cryptography + zstandard -- since it
imports savescan.py directly to decode the fixture's regulation.bin).
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

WEAPON_DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/EquipParamWeapon.xml"
REINFORCE_DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/ReinforceParamWeapon.xml"

WEAPON_CATEGORIES = {"melee_armaments", "shields", "ranged_and_catalysts"}

# The 15 ReinforceParamWeapon rate columns we keep, in this fixed order (the
# per-level arrays follow it). Attack + scaling + guard-cut -- everything the
# popup's weapon card shows that reinforcement actually scales. (Guard boost /
# crit have no rate column and stay at their base value.)
COLUMNS = [
    "physicsAtkRate", "magicAtkRate", "fireAtkRate", "thunderAtkRate", "darkAtkRate",
    "correctStrengthRate", "correctAgilityRate", "correctMagicRate", "correctFaithRate", "correctLuckRate",
    "physicsGuardCutRate", "magicGuardCutRate", "fireGuardCutRate", "thunderGuardCutRate", "darkGuardCutRate",
]


def max_level_for(reinforce_type, reinforce_row_ids):
    if reinforce_type not in reinforce_row_ids:
        return None
    level = 0
    while (reinforce_type + level + 1) in reinforce_row_ids:
        level += 1
    return level


def main():
    items_doc = json.loads((DATA_DIR / "items.json").read_text())
    items = items_doc["items"] if isinstance(items_doc, dict) and "items" in items_doc else items_doc
    weapons = [it for it in items if it["category"] in WEAPON_CATEGORIES]
    print(f"{len(weapons)} weapon-category items.json entries")

    wschema = build_schema(fetch(WEAPON_DEF_URL))
    rschema = build_schema(fetch(REINFORCE_DEF_URL))

    blob = sc._decoded_bnd4(str(FIXTURE_SAVE))
    wparam = sc.extract_bnd4_entry(blob, "EquipParamWeapon.param")
    rparam = sc.extract_bnd4_entry(blob, "ReinforceParamWeapon.param")
    wrows = {r["id"]: r["data_offset"] for r in sc.iter_param_rows(wparam, sc.parse_param_header(wparam))}
    rrows = {r["id"]: r["data_offset"] for r in sc.iter_param_rows(rparam, sc.parse_param_header(rparam))}

    # Per weapon: its reinforceTypeId (skip weapons absent from the param).
    weapon_types = {}
    for it in weapons:
        off = wrows.get(it["id"])
        if off is None:
            continue
        f = sc.decode_row_fields(wparam, off, wschema)
        weapon_types[it["id"]] = int(f["reinforceTypeId"])

    # Emit each distinct reinforceTypeId's per-level rate curve.
    types = {}
    for rt in sorted(set(weapon_types.values())):
        maxlvl = max_level_for(rt, rrows)
        if maxlvl is None:
            print(f"  WARNING reinforceTypeId {rt}: no base row, skipped")
            continue
        curve = []
        for L in range(maxlvl + 1):
            f = sc.decode_row_fields(rparam, rrows[rt + L], rschema)
            curve.append([round(float(f[c]), 4) for c in COLUMNS])
        types[str(rt)] = curve

    out = {
        "columns": COLUMNS,
        "types": types,
        "weapons": {str(k): v for k, v in sorted(weapon_types.items())},
    }
    (DATA_DIR / "weapon_reinforce_rates.json").write_text(json.dumps(out, separators=(",", ":")) + "\n")
    print(f"wrote {len(types)} reinforce curves, {len(weapon_types)} weapon->type entries")


if __name__ == "__main__":
    main()
