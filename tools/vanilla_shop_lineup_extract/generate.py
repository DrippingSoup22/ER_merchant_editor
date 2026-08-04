#!/usr/bin/env python3
"""Derive internal/assets/data/vanilla_shop_lineup.json: for every ShopLineupParam row, its
original FromSoftware vanilla value for each of the 8 fields a merchant-row
edit can ever touch (equipId/equipType/value/sellQuantity plus the 4
name/icon override fields -- see internal/catalog/enrich.go's
nameIconOverrideFields). This is the baseline the GUI's "Reset to Vanilla"
button diffs a loaded save against.

Ground truth: save_files/vanilla_fresh_character.dat, this project's own
trusted, never-edited vanilla fixture (same file tools/savescan.py's golden
test and every other "vanilla" cross-check in this repo already trusts).
ShopLineupParam lives in regulation.bin, which is shared game-version data,
not per-character -- any unedited save of the same game patch has byte-
identical rows. Caveat: this baseline goes stale if FromSoftware ever
patches these values; re-run against a fresh vanilla save if that happens.

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
import savescan as sc  # noqa: E402

# The complete editable surface of a ShopLineupParam row -- mirrors
# internal/catalog/enrich.go's own field list exactly (equipId/equipType/value/
# sellQuantity plus nameIconOverrideFields).
FIELDS = ["equipId", "equipType", "value", "sellQuantity",
          "iconId", "nameMsgId", "menuTitleMsgId", "menuIconId"]


def main():
    schema = sc.load_schema("shop_lineup_schema.json")

    blob = sc._decoded_bnd4(str(FIXTURE_SAVE))
    param = sc.extract_bnd4_entry(blob, "ShopLineupParam.param")
    header = sc.parse_param_header(param)
    rows = sc.iter_param_rows(param, header)
    print(f"ShopLineupParam rows: {len(rows)}")

    out = {}
    for row in rows:
        fields = sc.decode_row_fields(param, row["data_offset"], schema)
        out[str(row["id"])] = {f: fields[f] for f in FIELDS}

    (DATA_DIR / "vanilla_shop_lineup.json").write_text(
        json.dumps(out, indent=2, sort_keys=True) + "\n"
    )
    print(f"wrote {len(out)} vanilla_shop_lineup.json entries")


if __name__ == "__main__":
    main()
