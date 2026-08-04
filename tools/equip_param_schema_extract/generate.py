#!/usr/bin/env python3
"""Derive the 5 EquipParam* row schemas (Weapon/Protector/Accessory/Goods/
Gem -- the same equipType 0-4 convention ShopLineupParam's own equipType
field already uses) from soulsmods/Paramdex. Stdlib only, no vendored
copies. Schema building itself lives in ../paramdex_schema.py, shared with
tools/paramdex_extract and tools/weapon_reinforce_extract.

Needed for the price=0 fix (see docs/MERCHANT_DATA.md's 2026-07-30 entry):
a row's price can only safely go to 0 if the item's OWN sellValue field
(in its EquipParam* entry, a completely separate table from
ShopLineupParam) is also -1 ("cannot be sold back") -- confirmed by
decoding these tables against real items. internal/savefile needs the full row
schema (not just sellValue's offset) to write it correctly, same reasoning
every other table in this project uses.

Regenerate: python3 generate.py  (run from this directory; writes ../../internal/assets/data/)
"""

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
from paramdex_schema import build_schema, fetch  # noqa: E402

BASE_URL = "https://raw.githubusercontent.com/soulsmods/Paramdex/master/ER/Defs/"

# equipType -> (Paramdex Defs filename stem, output schema filename).
TABLES = {
    0: ("EquipParamWeapon", "equip_param_weapon_schema.json"),
    1: ("EquipParamProtector", "equip_param_protector_schema.json"),
    2: ("EquipParamAccessory", "equip_param_accessory_schema.json"),
    3: ("EquipParamGoods", "equip_param_goods_schema.json"),
    4: ("EquipParamGem", "equip_param_gem_schema.json"),
}


def main():
    data_dir = Path(__file__).resolve().parents[2] / "internal" / "assets" / "data"

    for equip_type, (def_name, out_name) in TABLES.items():
        schema = build_schema(fetch(BASE_URL + def_name + ".xml"))
        if not any(f["name"] == "sellValue" for f in schema["fields"]):
            raise ValueError(f"{def_name}: no sellValue field found -- schema/assumption changed?")
        (data_dir / out_name).write_text(json.dumps(schema, indent=2) + "\n")
        print(f"equipType {equip_type} ({def_name}): wrote {len(schema['fields'])} fields, "
              f"row_size={schema['row_size']} -> {out_name}")


if __name__ == "__main__":
    main()
