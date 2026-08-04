#!/usr/bin/env python3
"""Derive internal/assets/data/consumable_scaling.json: the attribute scaling of damage-
dealing throwable consumables (Fire Pot, Lightning Pot, throwing daggers,
stones, ...) so the item-info popup can show a Scaling panel for them like
weapons.

Elden Ring backs each throwable's damage with a hidden "virtual weapon":
EquipParamGoods.refVirtualWepId points at an EquipParamWeapon row whose
correctStrength/Agility/Magic/Faith/Luck are the throwable's scaling
coefficients. We read those straight from the fixture save's regulation.bin
(schema from soulsmods/Paramdex via tools/paramdex_schema.py, decode via
tools/savescan.py) -- ground truth, same source the game uses.

The popup grades these with the same standard breakpoints it uses for
weapons (internal/ui/gio: scalingGrade). Note the wiki's per-item damage-curve
grades can differ by a letter for some throwables (e.g. Fire Pot); we
deliberately grade from the raw coefficients for consistency with weapon
cards rather than scraping per-item wiki values (user decision 2026-08-03).

Goods id -> raw param row id: strip the 0x40 goods handle prefix
(id & 0x00FFFFFF), same as tools/spell_stats_extract. Restricted to the
`tools` category: that's the damage-throwable set. A few talismans/armor
carry a refVirtualWepId for incidental proc effects, not displayed scaling,
so they're excluded.

Regenerate: tools/.venv/bin/python3 generate.py (run from this directory;
needs the savescan venv -- cryptography + zstandard).
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

GOODS_DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/EquipParamGoods.xml"
WEAPON_DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/EquipParamWeapon.xml"

CONSUMABLE_CATEGORIES = {"tools"}
GOODS_ID_MASK = 0x00FFFFFF

# EquipParamWeapon scaling field -> our short stat key.
SCALE_FIELDS = [
    ("str", "correctStrength"),
    ("dex", "correctAgility"),
    ("int", "correctMagic"),
    ("fai", "correctFaith"),
    ("arc", "correctLuck"),
]


def main():
    items_doc = json.loads((DATA_DIR / "items.json").read_text())
    items = items_doc["items"] if isinstance(items_doc, dict) and "items" in items_doc else items_doc
    consumables = [it for it in items if it["category"] in CONSUMABLE_CATEGORIES]
    print(f"{len(consumables)} {'/'.join(sorted(CONSUMABLE_CATEGORIES))} items.json entries")

    goods_schema = build_schema(fetch(GOODS_DEF_URL))
    weapon_schema = build_schema(fetch(WEAPON_DEF_URL))

    blob = sc._decoded_bnd4(str(FIXTURE_SAVE))
    goods_param = sc.extract_bnd4_entry(blob, "EquipParamGoods.param")
    weapon_param = sc.extract_bnd4_entry(blob, "EquipParamWeapon.param")
    goods_rows = {r["id"]: r["data_offset"] for r in sc.iter_param_rows(goods_param, sc.parse_param_header(goods_param))}
    weapon_rows = {r["id"]: r["data_offset"] for r in sc.iter_param_rows(weapon_param, sc.parse_param_header(weapon_param))}
    print(f"EquipParamGoods rows: {len(goods_rows)}, EquipParamWeapon rows: {len(weapon_rows)}")

    out = []
    for it in consumables:
        goff = goods_rows.get(it["id"] & GOODS_ID_MASK)
        if goff is None:
            continue
        gf = sc.decode_row_fields(goods_param, goff, goods_schema)
        vwep = gf.get("refVirtualWepId", 0)
        if not vwep or vwep <= 0:
            continue
        woff = weapon_rows.get(vwep)
        if woff is None:
            continue
        wf = sc.decode_row_fields(weapon_param, woff, weapon_schema)
        scale = {key: max(0, int(wf[field])) for key, field in SCALE_FIELDS}
        if not any(scale.values()):
            continue  # a virtual weapon with no attribute scaling -- nothing to show
        out.append({"itemId": it["id"], **{k: v for k, v in scale.items() if v}})

    out.sort(key=lambda e: e["itemId"])
    (DATA_DIR / "consumable_scaling.json").write_text(json.dumps({"items": out}, indent=2) + "\n")
    print(f"wrote {len(out)} consumable_scaling.json entries")


if __name__ == "__main__":
    main()
