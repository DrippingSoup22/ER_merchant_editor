# `ShopLineupParam` schema

The 52-byte row schema comes from `soulsmods/Paramdex`, the PARAMDEF source
used by common Elden Ring modding tools. Its generated form is
`internal/assets/data/shop_lineup_schema.json`.

| Offset | Type | Field | Meaning |
|---:|---|---|---|
| 0 | s32 | `equipId` | ID within the table selected by `equipType` |
| 4 | s32 | `value` | price override; `-1` uses base price |
| 8 | s32 | `mtrlId` | `EquipMtrlSetParam` row; `-1` means none |
| 12 | u32 | `eventFlag_forStock` | persistent stock counter flag |
| 16 | u32 | `eventFlag_forRelease` | per-character availability gate |
| 20 | s16 | `sellQuantity` | `-1` unlimited, `0` unavailable |
| 23 | u8 | `equipType` | 0 weapon, 1 armor, 2 talisman, 3 good, 4 Ash of War |
| 24 | u8 | `costType` | currency kind |
| 26 | u16 | `setNum` | items granted per purchase |
| 28 | s32 | `value_Add` | base-price adjustment |
| 32 | f32 | `value_Magnification` | base-price multiplier |
| 36 | s32 | `iconId` | display override; `-1` inherits item |
| 40 | s32 | `nameMsgId` | display override; `-1` inherits item |
| 44 | s32 | `menuTitleMsgId` | menu-title override; `-1` inherits item |
| 48 | s16 | `menuIconId` | menu-icon override; `-1` inherits item |

The remaining bytes are padding.

## Item ID conversion

```text
itemID = equipId + offset[equipType]
offset = {0:0x00000000, 1:0x10000000, 2:0x20000000,
          3:0x40000000, 4:0x80000000}
```

`internal/catalog.EquipRefForItemID` implements the inverse. Reinforced weapon
rows may encode the level in the raw equipment ID; catalog resolution handles
that fallback while the generated item list keeps base IDs.

## Materials and currencies

`mtrlId` references an `EquipMtrlSetParam` row with up to six material slots.
Material IDs use the same category offsets. Some rows carry an unreliable
default category, so the catalog tests all five offsets and uses the declared
category only to break ambiguous matches.

`costType` is not always runes: Dragon Communion and Seluvis puppet rows use
other currencies. Preserve it during ordinary item edits.

## Names and baseline

Paramdex row names are stored in `shop_row_names.json`; they are diagnostic
input, not canonical merchant identity. Use `merchant_catalog.json`, described
in [MERCHANTS.md](MERCHANTS.md).

`vanilla_shop_lineup.json` stores the eight mutable fields for all 1,296 rows
and powers Reset to Vanilla. It must be regenerated after a game patch changes
the embedded regulation data.

Regenerate schemas/names with:

```sh
python3 tools/paramdex_extract/generate.py
```

Regenerate the baseline with:

```sh
tools/.venv/bin/python3 tools/vanilla_shop_lineup_extract/generate.py --save /path/to/matching-baseline.dat
```
