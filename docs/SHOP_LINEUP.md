# ShopLineupParam row schema

Source: **`soulsmods/Paramdex`** (github.com/soulsmods/Paramdex, MIT-style
community project — the schema source used by Smithbox/DSMapStudio/WitchyBND),
not hand-reverse-engineered. See `docs/MERCHANT_DATA.md`'s "Known" section for
how this fits the bigger picture.

## Row layout — 52 bytes (`ParamdefDataVersion` 3, confirmed matches our
## fixture's header)

| offset | size | type   | field                | notes |
|-------:|-----:|--------|----------------------|-------|
| 0      | 4    | s32    | equipId              | raw row ID in the per-`equipType` EquipParam* table — see conversion below |
| 4      | 4    | s32    | value                | price override, -1 = don't override |
| 8      | 4    | s32    | mtrlId               | row ID into `EquipMtrlSetParam.param` (not an item id — see "mtrlId resolution" below), -1 = none |
| 12     | 4    | u32    | eventFlag_forStock   | flag that preserves remaining quantity across visits |
| 16     | 4    | u32    | eventFlag_forRelease | flag gating whether this row is purchasable at all (this is how Bell Bearing unlocks work — see MERCHANT_DATA.md) |
| 20     | 2    | s16    | sellQuantity         | -1 = unlimited, 0 = out of stock, else finite count |
| 22     | 1    | dummy8 | pad                  | |
| 23     | 1    | u8     | equipType            | 0=Weapon 1=Protector 2=Accessory 3=Good 4=Gem(AoW); 5/6 seen in real data, undocumented |
| 24     | 1    | u8     | costType             | applies only when `value` overrides price |
| 25     | 1    | dummy8 | pad                  | |
| 26     | 2    | u16    | setNum               | items granted per purchase (default 1) |
| 28     | 4    | s32    | value_Add            | price = basePrice * value_Magnification + value_Add (when not overridden) |
| 32     | 4    | f32    | value_Magnification  | |
| 36     | 4    | s32    | iconId               | -1 = don't override |
| 40     | 4    | s32    | nameMsgId            | -1 = don't override |
| 44     | 4    | s32    | menuTitleMsgId       | shop-tab title override, -1 = don't override |
| 48     | 2    | s16    | menuIconId           | -1 = don't override |
| 50     | 2    | dummy8 | pad                  | |

`internal/assets/data/shop_lineup_schema.json` holds this as structured data (regenerable,
see below) — this table is just the human-readable summary.

## `equipId` -> item-id conversion (see `docs/ITEM_IDS.md`)

`equipId` is **not** an `items.json` key directly. Add an offset by
`equipType` (confirmed empirically against real rows across all 5 known
types):

```
item_id = equipId + {0: 0x00000000, 1: 0x10000000, 2: 0x20000000,
                      3: 0x40000000, 4: 0x80000000}[equipType]
```

`tools/savescan.py rows` applies this automatically (see its `item_name`
output field). The inverse (`equip_ref_for_item_id`, name -> `find-item`
subcommand, no save file needed) is exact — round-trip-verified against
all 2772 `items.json` entries and cross-checked against all 1247 resolvable
real shop rows (0 mismatches both ways). The 5 category offset ranges never
overlap in real data, so "largest offset <= item_id" always picks the
right bucket. 7/2772 items (0.25%) share a name across categories (e.g. a
sorcery and an Ash of War both called "Glintstone Pebble") — `find-item`
returns every match; a future interface must let the user disambiguate.

## `mtrlId` resolution

`mtrlId` is a row ID into `EquipMtrlSetParam.param` (841 rows in our
fixture, same PARAM/BND4 container, no new parsing code needed —
`parse_param_header`/`iter_param_rows` are already generic). Its schema
(from Paramdex's `ER/Defs/EquipMtrlSetParam.xml`, byte math confirmed
against our fixture: 52 bytes/row exactly): up to 6 parallel material
slots, each `materialId` (s32) + `itemNum` (s8 qty) + `materialCate` (u8,
`GAITEM_CATEGORY` enum — found in `ER/Tdfs/GAITEM_CATEGORY.tdf`, not the
Defs XML: `0=Weapon 1=Protector 2=Accessory 3=Good 4=Gem`, same numbering
as `equipType`). `-1` = empty slot.

`materialId + offset[materialCate]` resolves into `items.json` using the
same conversion table as `equipId` above. Verified against all 217
non-empty material slots referenced by our fixture's mtrlId-bearing rows:
0 unresolvable. Two gotchas:
- **`materialCate == 4` (Gem) is an unreliable default** — 51/217 slots
  (23.5%) carry the field's own shipped default value even though the item
  actually lives in the Good offset (`0x40000000`), not Gem
  (`0x80000000`). Fix: try all 5 category offsets, take the unique
  `items.json` hit rather than trusting `materialCate` directly.
- 7/222 material sets are genuinely ambiguous by raw id (2 offsets both
  hit `items.json`) — in every case `materialCate` (always `1`/Protector
  here) correctly picks the right one, so use it only as a tiebreaker
  when >1 offset hits, not as the primary lookup.
- 5/222 mtrlId values (901000-901004) don't exist as `EquipMtrlSetParam`
  rows at all, but are only referenced by unnamed/unreachable debug rows
  (9,000,000+ id range, no merchant in the Names file) — harmless.

In practice: 84.5% of mtrlId rows (332/393) are the "Alteration"/
"Reversion" armor-recolor trade (materialId resolves to the un-altered
counterpart of the altered item being sold); most of the rest are
Remembrance-for-weapon trades (Enia/Roundtable Hold, `value`=0). `costType`
is always `0` on these rows — `mtrlId` stacks on top of the rune price,
it doesn't replace it.

**Implemented** in `tools/savescan.py` (`resolve_material_item_id`,
`resolve_materials`) and exercised by its `merchants` subcommand, which
also flags the two data-quality exceptions below inline as `warnings`.

## Row -> merchant mapping

Comes straight from Paramdex's `ER/Names/ShopLineupParam.txt`
(`row_id -> "[Merchant Name] Item Name"`, 1261 entries) — vendored as
`internal/assets/data/shop_row_names.json`. No TalkParam/event-script RE needed. Not every
row has a name-file entry (16/1277 in our fixture didn't) — those are
untitled/unused-looking rows, not evidence the mapping is wrong.

**Its raw `merchant` string is not a clean merchant list** — conflates one
NPC's multiple unlock tiers, distinct NPCs with similar `"Name - Suffix"`
naming, and non-NPC mechanic pseudo-shops (Alteration/Reversion/Dragon
Communion). Reconciled into `internal/assets/data/merchant_catalog.json` (35 real
merchants) by `tools/merchant_catalog/generate.py` — see
[MERCHANTS.md](MERCHANTS.md) for the full research and rules. Anything
merchant-identity-related in the app should read that file, not the raw
Names file's `merchant` field directly.

## Regenerate

```
cd tools/paramdex_extract && python3 generate.py
```

Fetches `ER/Defs/ShopLineupParam.xml`, `ER/Names/ShopLineupParam.txt`, and
`ER/Defs/EquipMtrlSetParam.xml` from Paramdex's `master` branch
(stdlib-only, no vendored deps) and writes `internal/assets/data/shop_lineup_schema.json` +
`internal/assets/data/shop_row_names.json` + `internal/assets/data/equip_mtrl_set_schema.json`. The schema
builder collapses PARAMDEF bitfields (`type name:bits`) into a single
padding entry so offset math stays correct without needing to address
individual bits (only `EquipMtrlSetParam` has any, all unused by us). Since
it tracks Paramdex's `master`, re-running later could pick up upstream
changes — that's fine (rarely-rerun generator, same pattern as
`tools/itemdb_extract`).

## `costType` is a real currency enum, not always runes

Dragon Communion is bought with Dragon Hearts, not runes. `costType` (only
meaningful when `value != -1`, an override price)
is non-zero for all of Dragon Communion's rows — `costType 0` (1118/1277
rows) is the rune default, but 6 distinct values exist:
`0`=runes(1118), `1`=Dragon Communion's real rows + the stale duplicate
900000-series blocks now excluded (see MERCHANTS.md) — 22 total,
`2`=**Starlight Shards** (confirmed 2026-07-27: the only 4 browsable
costType-2 rows are Seluvis's puppets 100300/100301/100302/100310 with
values 2/5/3/5 = exactly their in-game Starlight Shard prices; the other 5
costType-2 rows are excluded 900000-series duplicates), `3`=a subset of the
excluded 900000-series duplicate block(5), `4`=121 rows incl. "Impaling
Thrust" (all in excluded/unattributed blocks — never user-visible, generic
label kept), `5`=the DLC Grand
Altar's Bayle-tier rows(2, matches Fextralife: the Grand Altar specifically
needs "Heart of Bayle", not a normal Dragon Heart — distinct value from
`costType 1`'s regular Dragon Hearts is consistent with that, though this
requirement itself isn't encoded via `mtrlId`/`eventFlag_forRelease` at
all — see MERCHANTS.md). Not fixed into a warning (user: "don't think this
is an issue, but worth remembering") — noted here for whenever a
price-editing feature needs to
know a row's real currency isn't runes.

**In-game price=0 test, 2026-07-31**: user confirmed Dragon Communion's
price edits take effect correctly in-game (which led to the Church/
Cathedral/Grand Altar split above).

## Data-quality audit (2026-07-25, all 1277 rows, pre-write-back-tool check)

- **Name/icon override exceptions — editor must not blindly assume
  item-swap inherits display name/icon.** 35/1277 rows (exact count, via
  `tools/savescan.py`'s `row_warnings`; an earlier manual pass under-counted
  this at 24) override one of `iconId`/`nameMsgId`/`menuTitleMsgId`/
  `menuIconId` away from the `-1` ("inherit from item") sentinel: most are
  lore "Note:" items (e.g. row `100500`, Merchant Kale, "Note: Flask of
  Wondrous Physick", `nameMsgId=8752`, some recurring across merchants/DLC
  areas) and the rest are spell/Ash-of-War "category header" slots where
  the row sells a base spell but displays as a shop-tab title (e.g. row
  `100050`, Sorceress Sellen, "Glintstone Pebble", `menuTitleMsgId=231010`).
  The other 1242 rows all have `-1` in these four fields (safe to assume
  item-swap "just works" for display). **Resolved in the editor 2026-07-27:**
  staging an item swap on such a row auto-stages `-1` into every non-`-1`
  override field (attached to the swap, so undo drops it; verified via
  savescan cross-read on rows 100500/100050), so swaps always display
  correctly and the GUI no longer paints these rows as red-square hazards
  (the warning stays in `catalog`/`savescan` output for oracle parity).
- **`row_warnings` also flags material-gated and event-flag-gated rows
  (2026-07-25).** Any row with `mtrlId != -1` (393/1277 rows) now warns:
  the rune price alone doesn't cover the real cost, a material (e.g. a
  Remembrance) is also consumed — most visible on Enia (51/116 rows). Any
  row with `eventFlag_forRelease != 0` (469/1277 rows) now warns it's
  gated behind a quest/boss-kill/bell-bearing flag, not available from the
  start — e.g. 7/39 Dragon Communion rows are per-dragon-kill-gated DLC
  incantations (Agheel's Flame/Borealis's Mist/Smarag's Glintstone
  Breath/Ekzykes's Decay/Magma Breath/Theodorix's Magma/Greyoll's Roar),
  vs. the always-available base 5-spell set + Bayle/Ghostflame tier
  (`eventFlag_forRelease == 0`). Combined, 864/1277 rows now carry at
  least one warning (up from 35) — broad, but each is independently real
  and useful editing context, not noise.
- **`ENIA_FORGING_ROW_IDS` (15 rows, 101775-101792 minus 3 gaps) — probably
  not real player-facing stock.** Paramdex labels these "Enia - Forging",
  distinct from her armor rows and the actual reward rows (101900+,
  mtrlId-gated). Each sells the Remembrance item itself at price 0 with a
  unique per-boss `eventFlag_forRelease`/`eventFlag_forStock` pair, unlike
  every other Enia row. Confirmed via research you cannot buy back a
  Remembrance from any merchant after redeeming it, so this can't be a
  literal "for sale" row — best-supported theory (not fully confirmed) is
  it's the engine's internal trigger for the hand-in dialogue action
  itself. Flagged via a dedicated `row_warnings` warning rather than
  hidden/reclassified, since the underlying data should stay fully
  visible/editable even though the UI meaning is uncertain.
- **`equipType` 5/6 — negligible.** equipType 6 doesn't occur at all in
  this save's 1277 rows; equipType 5 occurs exactly once (row `101932`,
  "Sword Lance (Spinning Gravity Thrust)", `equipId=4400039`) — one
  low-priority unresolvable-item_name edge case, not a real blocker.
- **`mtrlId` — resolved (2026-07-25), see "mtrlId resolution" below.** Not
  an item id at all — it's a foreign key into `EquipMtrlSetParam.param`
  (a separate table, already present in the same archive). 393/1277 rows
  (30.8%) use it.
- **`value`/`costType` — barely a gap.** 1275/1277 rows (99.8%) already
  carry an explicit price override in `value`; only 2 rows are `value ==
  -1` (base price comes from the item's own EquipParam, which we don't
  join) — too small to matter for the editor's price-editing feature.
- **`ShopLineupParam_Recipe.param` decodes cleanly under the exact same
  schema** — 188 rows, `equipType` restricted to Weapon(0)/Good(3) only,
  187/188 resolve a real `item_name`, `mtrlId` present in its own separate
  **300000-range** namespace (recipe/crafting-kit unlocks, not item
  purchases — `value`/`costType` are near-universally 0). Confirmed not
  garbage, still low priority/out of scope for the editor.

## `internal/assets/data/vanilla_shop_lineup.json` — Reset to Vanilla baseline (2026-07-31)

Embedded snapshot of every row's original FromSoftware value for the 8
fields a merchant-row edit can ever touch (`equipId`, `equipType`,
`value`, `sellQuantity`, plus the 4 name/icon override fields), keyed by
row id as a JSON string (same convention as `shop_row_names.json`/
`merchant_catalog.json`). Powers the Settings view's "Reset to Vanilla"
button (`internal/catalog/vanilla.go`'s `DiffFromVanilla`/`VanillaDiffs`):
diffs the currently loaded save's live rows against this baseline and
stages a revert for whatever differs, undoing drift from any number of
past sessions, not just the current one.

Generated once from `save_files/vanilla_fresh_character.dat` (regulation.bin
is shared game-version data, not per-character, so any unedited save of
the same patch is byte-identical here) by
`tools/vanilla_shop_lineup_extract/generate.py`, which reuses
`tools/savescan.py`'s own decrypt/BND4/param-decode helpers directly (same
pattern as `tools/weapon_reinforce_extract`). Regenerate:
`tools/.venv/bin/python3 tools/vanilla_shop_lineup_extract/generate.py`.

**Caveat**: this baseline goes stale if FromSoftware ever patches these
values — would need re-extracting from a fresh vanilla save of the new
patch. **Also**: character-unlock flags aren't part of this dataset and
can't be reverted by this feature for changes already written to a save
(no baseline is tracked for those) — see `docs/EDITOR.md`'s "Reset to
Vanilla" entry.
