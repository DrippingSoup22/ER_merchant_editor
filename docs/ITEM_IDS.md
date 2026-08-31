# Item catalog data

`internal/assets/data/items.json` is the generated 1.17 catalog: 2,813 entries with
ID, name, category, subcategory, icon path, and optional `hidden`/`risky`
flags. Every icon path resolves under `internal/assets/icons/items/`.

- `hidden` items remain resolvable for existing shop rows but are omitted from
  browsing, usually because they are internal variants or duplicates.
- `risky` marks cut or online-ban-risk content. It is hidden by default; risk
  propagates from base armor to its altered counterpart by name.
- Same-name items in different categories remain distinct.

Regulation 1.17 adds eight public armament families and four armor sets. Both
real altered armor rows are separate selectable items alongside their normal
forms. The three Spectral Steed Attire goods are legitimate ownership tokens
under `Key Items > Spectral Steed Attires`. Buying one unlocks it for the
game's normal hub selector; the editor does not directly change the separate
mutually-exclusive active-appearance flags. In-game testing confirmed all three
purchase and activate safely. If a duplicate is purchased, the game's shop
overflow can put the second copy in repository storage even though the item
cannot be deposited manually; this caused no observed problem.

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
- `armor_stats.json`: generator input decoded from `EquipParamProtector`; it
  supplies complete damage negation, resistance, poise, and weight data for
  all 741 armor items.
- `weapon_reinforce.json`: real maximum upgrade level per weapon.
- `weapon_reinforce_rates.json`: per-level attack/scaling/guard multipliers.

Armor, spell, and throwable scaling JSON files are generator inputs; they are
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
GOTOOLCHAIN=go1.25.0 go run . -out ../../internal/assets/data/items.json
```

Related generators under `tools/` rebuild sort order, spell stats,
consumable scaling, reinforcement limits, and reinforcement curves from
Paramdex schemas plus a read-only fixture. Run their prerequisite generators
before `itemdb_extract` when refreshing derived details.

Generate armor stats from a regulation-matching baseline first:

```sh
cd tools/armor_stats_extract
ER_PARAMDEX_ROOT=/path/to/Paramdex ../.venv/bin/python3 generate.py --save /path/to/matching-baseline.dat
```

Important schema rule: adjacent PARAMDEF bitfields sharing one storage byte
must remain one group even when one is declared `dummy8`; otherwise every
following offset is wrong. `tools/paramdex_schema.py` owns this behavior.

Generated JSON and icon trees are large. Query them selectively. Put catalog
metadata corrections in the relevant generator; document and pin any vendored
image-byte corrections as below.

Four misassigned vendored PNGs are intentionally replaced with verified item
art from Gamer Guides and pinned by `internal/assets/icons/embed_test.go`:
Scorpion Liver, Piquebone Arrow (Fletched), Serpent Crest Shield, and Golden
Lion Shield. Each source image is downscaled to 256×256 with Lanczos3 before
embedding. Their catalog paths remain the normal generated paths; the
correction is to the image bytes, not item identity.
