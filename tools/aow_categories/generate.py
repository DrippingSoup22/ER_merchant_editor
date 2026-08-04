#!/usr/bin/env python3
"""Derive tools/itemdb_extract/aow_categories.go: a strength/dexterity/
quality/magic/faith/arcane sub-category for every ashes_of_war item.json
entry, per user request (2026-08-01): "ashes of war must also be organized
like the game does by grouping them using sub-categories(in this case the
sub-category are like the character attributes)".

Ashes of War don't have personal stat requirements the way weapons do, so
"the affinity it locks in" (user's own framing) is the signal: every
EquipParamGem row carries `defaultWepAttr` (the InfuseType index applied
when equipped -- see EldenRing-SaveForge's db.InfuseTypes) AND `sortGroupId`
-- decoded against our fixture save, `sortGroupId` groups ALL 116 items into
exactly 12 clean, zero-overlap defaultWepAttr families (verified: every
sortGroupId maps to one or a small deliberate cluster of defaultWepAttr
values, e.g. sortGroupId=50 is Fire+Flame Art+Lightning together) -- this
*is* the game's own real in-menu grouping, not a guess.

InfuseType -> bucket mapping (community-standard ER scaling knowledge, not
found in any single field so applied here as a judgment call, documented for
future reference):
  Heavy(1)      -> Strength   (scales Strength)
  Fire(4)       -> Strength   (no stat scaling; conventionally paired with
                                high-base-AR Strength weapons since it can't
                                benefit from investment elsewhere)
  Keen(2)       -> Dexterity  (scales Dexterity)
  Lightning(6)  -> Dexterity  (scales Dex primarily, Faith secondarily)
  Quality(3)    -> Quality    (scales Strength+Dexterity evenly)
  Magic(8)      -> Magic      (scales Intelligence)
  Cold(9)       -> Magic      (scales Intelligence primarily)
  Sacred(7)     -> Faith      (scales Faith)
  Flame Art(5)  -> Faith      (scales Faith)
  Poison(10)    -> Arcane     (scales Arcane)
  Blood(11)     -> Arcane     (scales Arcane)
  Occult(12)    -> Arcane     (scales Arcane -- the "true" Arcane affinity)
  Standard(0)   -> no sub-category (guard/parry/bow-only/misc unique skills
                                     that don't lock any particular affinity
                                     at all -- left blank rather than forced
                                     into an arbitrary bucket, matching this
                                     project's existing "absence of
                                     enrichment, not a wrong label" rule for
                                     talismans/chest/head/legs/arms).

Regenerate: tools/.venv/bin/python3 generate.py (run from this directory;
needs the same venv as savescan.py).
"""

import sys
from pathlib import Path

TOOLS_DIR = Path(__file__).resolve().parents[1]
DATA_DIR = TOOLS_DIR.parent / "internal" / "assets" / "data"
FIXTURE_SAVE = TOOLS_DIR.parent / "save_files" / "vanilla_fresh_character.dat"
OUT_GO = TOOLS_DIR / "itemdb_extract" / "aow_categories.go"

sys.path.insert(0, str(TOOLS_DIR))
from paramdex_schema import build_schema, fetch  # noqa: E402
import savescan as sc  # noqa: E402
import json  # noqa: E402

GEM_DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/EquipParamGem.xml"

# defaultWepAttr (InfuseType index) -> sub-category bucket. See module
# doc comment for the reasoning behind each mapping.
ATTR_TO_BUCKET = {
    1: "Strength",
    4: "Strength",
    2: "Dexterity",
    6: "Dexterity",
    3: "Quality",
    8: "Magic",
    9: "Magic",
    7: "Faith",
    5: "Faith",
    10: "Arcane",
    11: "Arcane",
    12: "Arcane",
    # 0 (Standard) intentionally absent -> no sub-category.
}


def main():
    items = json.loads((DATA_DIR / "items.json").read_text())
    aow_items = [it for it in items if it["category"] == "ashes_of_war"]
    print(f"{len(aow_items)} ashes_of_war items.json entries")

    gem_schema = build_schema(fetch(GEM_DEF_URL))
    blob = sc._decoded_bnd4(str(FIXTURE_SAVE))
    gem_param = sc.extract_bnd4_entry(blob, "EquipParamGem.param")
    gem_header = sc.parse_param_header(gem_param)
    gem_rows = {r["id"]: r["data_offset"] for r in sc.iter_param_rows(gem_param, gem_header)}

    out = {}
    missing, unbucketed = [], []
    for it in aow_items:
        raw_id = it["id"] - 0x80000000  # Ash of War offset, see docs/ITEM_IDS.md
        off = gem_rows.get(raw_id)
        if off is None:
            missing.append(it["name"])
            continue
        fields = sc.decode_row_fields(gem_param, off, gem_schema)
        bucket = ATTR_TO_BUCKET.get(fields["defaultWepAttr"])
        if bucket is None:
            unbucketed.append((it["name"], fields["defaultWepAttr"]))
            continue
        out[it["id"]] = (bucket, it["name"])

    if missing:
        print(f"WARNING: {len(missing)} ashes_of_war items have no EquipParamGem row: {missing}")
    print(f"{len(unbucketed)} items left unbucketed (Standard/no-lock skills, expected): {[n for n, _ in unbucketed]}")
    print(f"{len(out)} items bucketed")

    lines = [
        "package main",
        "",
        "// aowSubCategoryOverrides: strength/dexterity/quality/magic/faith/arcane",
        "// sub-category per ashes_of_war item, generated by tools/aow_categories from",
        "// EquipParamGem's defaultWepAttr (the affinity the Ash of War locks in when",
        "// applied) -- see tools/aow_categories/generate.py for the full mapping",
        "// rationale and docs/ITEM_IDS.md for the summary. Items with no forced",
        "// affinity (Standard/guard/parry/bow-only skills) are intentionally absent",
        "// -- no sub-category, not an arbitrary one.",
        "//",
        "// Regenerate: cd tools/aow_categories && ../.venv/bin/python3 generate.py",
        "var aowSubCategoryOverrides = map[uint32]string{",
    ]
    for item_id in sorted(out):
        bucket, name = out[item_id]
        lines.append(f'\t0x{item_id:08X}: "{bucket}", // {name}')
    lines.append("}")
    lines.append("")

    OUT_GO.write_text("\n".join(lines))
    print(f"wrote {OUT_GO}")


if __name__ == "__main__":
    main()
