#!/usr/bin/env python3
"""Derive internal/assets/data/spell_stats.json: for every sorcery/incantation items.json
entry, its cast stats (FP cost, memory slots, INT/FAI/ARC requirements).

Ground truth from MagicParam in our own fixture save's regulation.bin --
schema from soulsmods/Paramdex (tools/paramdex_schema.py, shared with the
other param extractors), row values decoded via tools/savescan.py's
decrypt/DCX/BND4/param decode (no new parsing code). This supersedes
SaveForge's hand-curated descriptions.go spell table, which predates the
Shadow of the Erdtree DLC and so omits ~42 DLC spells; the param covers
every spell uniformly.

Mapping: a sorcery/incantation is a Goods item whose items.json id is the
goods-prefixed form (0x40000000 | rawId); its MagicParam row id is that raw
id (id & 0x00FFFFFF). Verified against SaveForge's curated values, e.g.
Glintstone Pebble (id 0x40000FA0 -> Magic row 4000: mp 7, INT 10).

Field map (MagicParam): mp -> fp, slotLength -> slots,
requirementIntellect -> reqInt, requirementFaith -> reqFai,
requirementLuck -> reqArc.

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

MAGIC_DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/MagicParam.xml"

# Spell categories in items.json (Goods items backed by MagicParam).
SPELL_CATEGORIES = {"sorceries", "incantations"}

# Goods id -> raw param row id: strip the 0x40 goods handle prefix.
GOODS_ID_MASK = 0x00FFFFFF


def main():
    items_doc = json.loads((DATA_DIR / "items.json").read_text())
    items = items_doc["items"] if isinstance(items_doc, dict) and "items" in items_doc else items_doc
    spell_items = [it for it in items if it["category"] in SPELL_CATEGORIES]
    print(f"{len(spell_items)} sorcery/incantation items.json entries")

    schema = build_schema(fetch(MAGIC_DEF_URL))
    print(f"MagicParam: {len(schema['fields'])} fields, row_size={schema['row_size']}")

    blob = sc._decoded_bnd4(str(FIXTURE_SAVE))
    magic_param = sc.extract_bnd4_entry(blob, "Magic.param")
    header = sc.parse_param_header(magic_param)
    rows = {r["id"]: r["data_offset"] for r in sc.iter_param_rows(magic_param, header)}
    print(f"MagicParam rows: {len(rows)}")

    out = []
    missing = []
    for it in spell_items:
        raw_id = it["id"] & GOODS_ID_MASK
        off = rows.get(raw_id)
        if off is None:
            missing.append(it["name"])
            continue
        f = sc.decode_row_fields(magic_param, off, schema)
        out.append({
            "itemId": it["id"],
            "fp": int(f["mp"]),
            "slots": int(f["slotLength"]),
            "reqInt": int(f["requirementIntellect"]),
            "reqFai": int(f["requirementFaith"]),
            "reqArc": int(f["requirementLuck"]),
        })

    if missing:
        print(f"WARNING: {len(missing)} spell items have no MagicParam row: {missing[:10]}")

    out.sort(key=lambda e: e["itemId"])
    (DATA_DIR / "spell_stats.json").write_text(
        json.dumps({"spells": out}, indent=2) + "\n"
    )
    print(f"wrote {len(out)} spell_stats.json entries")


if __name__ == "__main__":
    main()
