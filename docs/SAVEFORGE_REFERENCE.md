# Findings mined from EldenRing-SaveForge (prior art)

Source: `/mnt/c/Users/danie/Desktop/EldenRing-SaveForge-main` (external,
trusted, not part of this repo — read-only). Used for `data/items.json`
(via `tools/itemdb_extract`, see `ITEM_IDS.md`) and as one of several
independent AES-key/pipeline cross-checks (see
`ER_PVP_MOD_REFERENCE.md`'s "Independent verification" section).

## Bell Bearing / Twin Maiden Husk unlock mechanism

Classic Souls EventFlag bit, per-character-slot
(`db.SetEventFlag`/`GetEventFlag`, `slot.Data[EventFlagsOffset:]`) — lives
inside each character slot's own buffer, **not** `UserData10`/`UserData11`.
Gates *release* of TMH's `ShopLineupParam` rows via each row's own
`eventFlag_forRelease` (see `MERCHANT_DATA.md`'s "Known" section) — two
mechanisms working together, not one replacing the other.
`BellBearingItemToFlagID` (`backend/db/data/bell_bearing_flags.go`) was
ported by SaveForge from an external Python project (`er-save-manager`),
not independently reverse-engineered by SaveForge itself. (2026-08-03:
this table is now actually ported into this repo,
`app/charunlock/bell_bearings.go` — see `docs/CHAR_UNLOCK.md`'s
"Bell-bearing acquisition toggles" entry. SaveForge itself only ever
*lists* the table — it never writes `ShopLineupParam` at all, see
"Confirmed: no merchant shop-content prior art here" below.)

## Two ID encodings

See `docs/ITEM_IDS.md`'s "Critical gotcha" section — SaveForge's `db.go`
is the source for both encodings documented there.

## Confirmed: no merchant shop-content prior art here

SaveForge never touches `ShopLineupParam` — the only param table it parses
out of the embedded regulation data is `NetworkParam.param` (for PvP
tuning). Shop-lineup editing was novel work in this repo.

## Attribution (GPLv3 relicense, 2026-07-28)

This project was MIT through 2026-07-27. The per-character event-flag
unlock feature (`app/charflags`, in progress — see
`character_flag_unlock_feature` planning) adapts SaveForge's `.sl2`
slot-parsing and flag byte/bit packing algorithm/table
(`backend/core/section_eventflags.go`, `backend/db`), which we could not
independently reconstruct from public documentation alone (see
`PROJECT.md`). Per user authorization (project owner, personally
acquainted with SaveForge's author), the whole repo was relicensed to
GPLv3 to permit this reuse cleanly rather than maintain a partial-license
split within one compiled binary. Everything else in this repo (shop-row
schema/write-back, item catalog, GUI) remains independently developed —
this section exists to satisfy GPL's attribution requirement for the
adapted portion specifically, not to imply the whole project derives from
SaveForge. The 2026-08 bell-bearing flag/name table
(`app/charunlock/bell_bearings.go`) is the same class of adapted external
data, covered by this same relicense.
