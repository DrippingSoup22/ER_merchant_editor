#!/usr/bin/env python3
"""Derive shop_lineup_schema.json + shop_row_names.json + equip_mtrl_set_schema.json
from soulsmods/Paramdex (the community-maintained FromSoftware PARAMDEF/
row-name source, see docs/SHOP_LINEUP.md). Stdlib only, no vendored copies
of the source files. Schema building itself lives in ../paramdex_schema.py,
shared with tools/weapon_reinforce_extract.

Regenerate: python3 generate.py  (run from this directory; writes ../../internal/assets/data/)
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

# Paramdex is current at ff7245e5 but its ShopLineup row-name file predates
# regulation 1.17. These identities come from the new rows themselves plus
# their contiguous merchant/service families in the 1.17 regulation. Keep the
# patch here so every regeneration retains the new slots until Paramdex gains
# equivalent names.
PATCH_117_NAMES = {
    "100568": {"merchant": "Merchant - East Limgrave", "item_label": "Hefty Scimitar"},
    "100666": {"merchant": "Isolated Merchant - Weeping Peninsula", "item_label": "Steel Helm"},
    "100667": {"merchant": "Isolated Merchant - Weeping Peninsula", "item_label": "Steel Armor"},
    "100668": {"merchant": "Isolated Merchant - Weeping Peninsula", "item_label": "Steel Gauntlets"},
    "100669": {"merchant": "Isolated Merchant - Weeping Peninsula", "item_label": "Steel Greaves"},
    "100709": {"merchant": "Merchant - North Liurnia", "item_label": "Silver Grooved Shield"},
    "100710": {"merchant": "Merchant - North Liurnia", "item_label": "Silver Grooved Helm"},
    "100711": {"merchant": "Merchant - North Liurnia", "item_label": "Silver Grooved Armor"},
    "100712": {"merchant": "Merchant - North Liurnia", "item_label": "Silver Grooved Gauntlets"},
    "100713": {"merchant": "Merchant - North Liurnia", "item_label": "Silver Grooved Greaves"},
    "101896": {"merchant": "Twin Maiden Husks", "item_label": "Reverse-Bladed Sword"},
    "110084": {"merchant": "Alteration", "item_label": "Silver Grooved Armor (Altered)"},
    "111084": {"merchant": "Alteration", "item_label": "Silver Grooved Armor (Altered)"},
    "110085": {"merchant": "Alteration", "item_label": "Leontiel's Hat (Altered)"},
    "111085": {"merchant": "Alteration", "item_label": "Leontiel's Hat (Altered)"},
    "110284": {"merchant": "Reversion", "item_label": "Silver Grooved Armor"},
    "111284": {"merchant": "Reversion", "item_label": "Silver Grooved Armor"},
    "110285": {"merchant": "Reversion", "item_label": "Leontiel's Hat"},
    "111285": {"merchant": "Reversion", "item_label": "Leontiel's Hat"},
}


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
    rows.update(PATCH_117_NAMES)
    return rows


def main():
    data_dir = Path(__file__).resolve().parents[2] / "internal" / "assets" / "data"

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
