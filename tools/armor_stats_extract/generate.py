#!/usr/bin/env python3
"""Generate complete armor popup stats from EquipParamProtector.

SaveForge's descriptions.go contains hand-curated ArmorStats for only part of
the catalog. EquipParamProtector is the authoritative source for every real
armor row, including Shadow of the Erdtree and regulation 1.17 additions.

Regenerate from this directory:
  ../.venv/bin/python3 generate.py --save /path/to/matching-baseline.dat
"""

import argparse
import json
import sys
from pathlib import Path

TOOLS_DIR = Path(__file__).resolve().parents[1]
DATA_DIR = TOOLS_DIR.parent / "internal" / "assets" / "data"
sys.path.insert(0, str(TOOLS_DIR))

from paramdex_schema import build_schema, fetch  # noqa: E402
import savescan as sc  # noqa: E402

PROTECTOR_DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/EquipParamProtector.xml"
ARMOR_CATEGORIES = {"head", "chest", "arms", "legs"}


def negation(multiplier: float) -> float:
    """Convert the stored damage multiplier to the one-decimal UI negation."""
    return round((1.0 - float(multiplier)) * 100.0, 1)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--save", type=Path, required=True,
                        help="decrypted save whose embedded regulation supplies armor rows")
    args = parser.parse_args()

    items = json.loads((DATA_DIR / "items.json").read_text())
    armor = [it for it in items if it["category"] in ARMOR_CATEGORIES]

    schema = build_schema(fetch(PROTECTOR_DEF_URL))
    blob = sc._decoded_bnd4(str(args.save))
    param = sc.extract_bnd4_entry(blob, "EquipParamProtector.param")
    header = sc.parse_param_header(param)
    rows = {r["id"]: r["data_offset"] for r in sc.iter_param_rows(param, header)}

    out = []
    missing = []
    for item in armor:
        row_id = item["id"] - 0x10000000
        offset = rows.get(row_id)
        if offset is None:
            missing.append((item["id"], item["name"]))
            continue
        f = sc.decode_row_fields(param, offset, schema)
        out.append({
            "itemId": item["id"],
            "weight": round(float(f["weight"]), 1),
            "physical": negation(f["neutralDamageCutRate"]),
            "strike": negation(f["blowDamageCutRate"]),
            "slash": negation(f["slashDamageCutRate"]),
            "pierce": negation(f["thrustDamageCutRate"]),
            "magic": negation(f["magicDamageCutRate"]),
            "fire": negation(f["fireDamageCutRate"]),
            "lightning": negation(f["thunderDamageCutRate"]),
            "holy": negation(f["darkDamageCutRate"]),
            # The menu's displayed poise is toughnessCorrectRate * 1000.
            # saDurability is separate super-armor durability (often 100).
            "poise": round(float(f["toughnessCorrectRate"]) * 1000.0, 1),
            "immunity": int(f["resistPoison"]),
            "robustness": int(f["resistBlood"]),
            "focus": int(f["resistSleep"]),
            "vitality": int(f["resistCurse"]),
        })

    if missing:
        raise SystemExit(f"{len(missing)} catalog armor rows missing from EquipParamProtector: {missing[:10]}")
    out.sort(key=lambda entry: entry["itemId"])
    (DATA_DIR / "armor_stats.json").write_text(
        json.dumps({"armor": out}, indent=2) + "\n"
    )
    print(f"wrote {len(out)} complete armor records from {len(rows)} protector rows")


if __name__ == "__main__":
    main()
