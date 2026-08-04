#!/usr/bin/env python3
"""Derive shop_lineup_schema.json + shop_row_names.json + equip_mtrl_set_schema.json
from soulsmods/Paramdex (the community-maintained FromSoftware PARAMDEF/
row-name source, see docs/SHOP_LINEUP.md). Stdlib only, no vendored copies
of the source files. Schema building itself lives in ../paramdex_schema.py,
shared with tools/weapon_reinforce_extract.

Regenerate: python3 generate.py  (run from this directory; writes ../../data/)
"""

import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
from paramdex_schema import build_schema, fetch  # noqa: E402

DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/ShopLineupParam.xml"
NAMES_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Names/ShopLineupParam.txt"
MTRL_DEF_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/EquipMtrlSetParam.xml"

NAME_LINE_RE = re.compile(r"^(?P<id>\d+)\s+(?:\[(?P<merchant>[^\]]+)\]\s*)?(?P<label>.*)$")


def build_names(names_text: str) -> dict:
    rows = {}
    for line in names_text.splitlines():
        line = line.strip()
        if not line:
            continue
        m = NAME_LINE_RE.match(line)
        if not m:
            raise ValueError(f"unparsed name line: {line!r}")
        rows[m.group("id")] = {
            "merchant": m.group("merchant"),
            "item_label": m.group("label").strip(),
        }
    return rows


def main():
    data_dir = Path(__file__).resolve().parents[2] / "data"

    schema = build_schema(fetch(DEF_URL))
    (data_dir / "shop_lineup_schema.json").write_text(json.dumps(schema, indent=2) + "\n")
    print(f"wrote {len(schema['fields'])} fields, row_size={schema['row_size']}")

    names = build_names(fetch(NAMES_URL))
    (data_dir / "shop_row_names.json").write_text(json.dumps(names, indent=2) + "\n")
    print(f"wrote {len(names)} row-name entries")

    mtrl_schema = build_schema(fetch(MTRL_DEF_URL))
    (data_dir / "equip_mtrl_set_schema.json").write_text(json.dumps(mtrl_schema, indent=2) + "\n")
    print(f"wrote {len(mtrl_schema['fields'])} fields, row_size={mtrl_schema['row_size']} (EquipMtrlSetParam)")


if __name__ == "__main__":
    main()
