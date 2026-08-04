# Item catalog data

`internal/assets/data/items.json` is the generated catalog: 2,784 entries with
ID, name, category, subcategory, icon path, and optional `hidden`/`risky`
flags. Every icon path resolves under `internal/assets/icons/items/`.

- `hidden` items remain resolvable for existing shop rows but are omitted from
  browsing, usually because they are internal variants or duplicates.
- `risky` marks cut or online-ban-risk content. It is hidden by default; risk
  propagates from base armor to its altered counterpart by name.
- Same-name items in different categories remain distinct.

## ID spaces

Do not mix these encodings:

- catalog item IDs use category high ranges (`0x00` weapon, `0x10` armor,
  `0x20` talisman, `0x40` goods, `0x80` Ash of War);
- inventory GaItem handles use `0x80` through `0xC0` ranges;
- shop `equipId` is a raw row ID in the `EquipParam*` table selected by
  `equipType`.

Shop conversion is documented in [SHOP_LINEUP.md](SHOP_LINEUP.md) and
implemented by `internal/catalog.EquipRefForItemID`.

## Runtime datasets

- `item_sort_order.json`: in-game sort/group IDs; catalog subcategory is the
  outer grouping, game order applies within it.
- `item_details.json`: descriptions, locations, weight, and structured weapon,
  armor, spell, and scaling data used by the info popup.
- `weapon_reinforce.json`: real maximum upgrade level per weapon.
- `weapon_reinforce_rates.json`: per-level attack/scaling/guard multipliers.

Spell stats and throwable scaling JSON files are generator inputs; they are
not embedded at runtime. `internal/assets/data/embed.go` is the authoritative
list of embedded datasets.

## Reinforcement

Generated weapon IDs represent the `+0` base. A reinforced shop weapon uses
`baseEquipID + level`. Maximum level comes from its real
`ReinforceParamWeapon` sequence: normally +25 for standard, +10 for somber,
and +0 for non-upgradeable weapons. Catalog decoding validates the fallback
against the generated maximum so a saved `+N` row reloads correctly.

## Generation

`tools/itemdb_extract` projects the PS4 item database from the external
GPLv3 SaveForge source and applies documented category, hidden, risky, and
icon overrides. Regenerate from that tool's directory:

```sh
cd tools/itemdb_extract
GOTOOLCHAIN=go1.25.0 go run . > ../../internal/assets/data/items.json
```

Related generators under `tools/` rebuild sort order, spell stats,
consumable scaling, reinforcement limits, and reinforcement curves from
Paramdex schemas plus a read-only fixture. Run their prerequisite generators
before `itemdb_extract` when refreshing derived details.

Important schema rule: adjacent PARAMDEF bitfields sharing one storage byte
must remain one group even when one is declared `dummy8`; otherwise every
following offset is wrong. `tools/paramdex_schema.py` owns this behavior.

Generated JSON and icon trees are large. Query them selectively and never
hand-edit them; put corrections in the relevant generator.
