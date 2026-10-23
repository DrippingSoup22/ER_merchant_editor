#!/usr/bin/env python3
"""Derive internal/assets/data/vanilla_shop_lineup.json: for every ShopLineupParam row, its
original FromSoftware vanilla value for each of the 8 fields a merchant-row
edit can ever touch (equipId/equipType/value/sellQuantity plus the 4
name/icon override fields -- see internal/catalog/enrich.go's
nameIconOverrideFields). This is the baseline the GUI's "Reset to Vanilla"
button diffs a loaded save against.

Ground truth is supplied explicitly with --save. ShopLineupParam lives in
regulation.bin, which is shared game-version data, not per-character -- any
unedited save of the same game patch has byte-identical rows. The shipped
dataset was generated from regulation 11701000; always provide a matching
baseline when FromSoftware patches it.

Regenerate: python3 tools/vanilla_shop_lineup_extract/generate.py --save <baseline>
"""

import json
import argparse
import sys
from pathlib import Path

TOOLS_DIR = Path(__file__).resolve().parents[1]
DATA_DIR = TOOLS_DIR.parent / "internal" / "assets" / "data"
sys.path.insert(0, str(TOOLS_DIR))
import param_reader as pr  # noqa: E402

# The complete editable surface of a ShopLineupParam row -- mirrors
# internal/catalog/enrich.go's own field list exactly (equipId/equipType/value/
# sellQuantity plus nameIconOverrideFields).
FIELDS = ["equipId", "equipType", "value", "sellQuantity",
          "iconId", "nameMsgId", "menuTitleMsgId", "menuIconId"]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--save", type=Path, required=True,
                        help="decrypted save whose embedded regulation supplies the vanilla rows")
    args = parser.parse_args()
    schema = pr.load_schema("shop_lineup_schema.json")
    param, rows = pr.load_param(args.save, "ShopLineupParam.param")
    print(f"ShopLineupParam rows: {len(rows)}")

    out = {}
    for row in rows:
        fields = pr.decode_row_fields(param, row["data_offset"], schema)
        out[str(row["id"])] = {f: fields[f] for f in FIELDS}

    (DATA_DIR / "vanilla_shop_lineup.json").write_text(
        json.dumps(out, indent=2, sort_keys=True) + "\n"
    )
    print(f"wrote {len(out)} vanilla_shop_lineup.json entries")


if __name__ == "__main__":
    main()
