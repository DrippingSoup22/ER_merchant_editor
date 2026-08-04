# Project status

ER Merchant Editor is a released Go/Gio desktop application for editing
merchant stock in decrypted Elden Ring PS4/PS5 saves. Windows is established;
native Linux and macOS packaging is present and is the next release focus.

## Scope

Supported:

- replace an existing `ShopLineupParam` row's item;
- edit price, quantity, and weapon reinforcement level;
- inspect catalog metadata and restore merchant rows to an embedded vanilla
  baseline;
- unlock or relock merchant stock per character, including supported Twin
  Maiden Husks bell bearings.

Out of scope:

- PlayStation's outer save encryption/signing layer;
- adding/removing shop rows or changing NPC TalkESD shop ranges;
- editing material-priced rows (`mtrlId != -1`);
- Enia unlock flags, which overlap boss-progression flags and are unsafe.

## Hard row-count limit

The save embeds 1,277 `ShopLineupParam` rows, all already used. An NPC's shop
range is selected by TalkESD scripts outside the save. The editor can therefore
change row values but cannot make a merchant expose more rows. Full zstd
recompression removes the old practical limit on how many existing values can
change in one save.

## Save pipeline

```text
decrypted PlayStation save
  -> USERDATA11 (AES-256-CBC)
  -> DCX/zstd regulation.bin
  -> BND4 PARAM entries
  -> validated mutations
  -> full recompression, encryption, and byte-level verification
  -> new output save
```

Merchant rows live in the embedded `regulation.bin`. Character unlock flags
live separately in each `.sl2` character slot. `internal/application.SavePlan`
combines both edit types through ordered temporary outputs and advances the
loaded session only after every stage succeeds.

## Source of truth

- [ARCHITECTURE.md](ARCHITECTURE.md): package ownership and dependencies.
- [MERCHANT_DATA.md](MERCHANT_DATA.md): save container and fixed offsets.
- [SHOP_LINEUP.md](SHOP_LINEUP.md): row schema and generated baseline.
- [MERCHANTS.md](MERCHANTS.md): canonical merchant mapping.
- [ITEM_IDS.md](ITEM_IDS.md): item ID conversion and generated metadata.
- [WRITEBACK.md](WRITEBACK.md): mutation guards and zstd requirements.
- [CHAR_UNLOCK.md](CHAR_UNLOCK.md): per-character flags and safety rules.
- [EDITOR.md](EDITOR.md): Gio state and interaction invariants.
- [PACKAGING.md](PACKAGING.md): native builds and release CI.

The two files under `save_files/` are local, read-only fixtures. Tests skip
fixture-dependent cases when they are absent. Development extractors under
`tools/` are not runtime dependencies.

Completed implementation history belongs in git history; these documents
describe the current system.
