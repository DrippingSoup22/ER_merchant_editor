# Project status

Elden Ring save-file editor, focused on merchant shop contents.

## Goal

**Free merchant editor** — let the user edit any merchant's existing
`ShopLineupParam` rows: swap the item, set price, set quantity. Unlock
gating (`eventFlag_forRelease`) is never touched by item edits (2026-08-02
— it gates the SLOT, not the item in it) — the only way to change unlock
state is per-character via `app/charunlock`/the Characters view, see
`docs/CHAR_UNLOCK.md`. Value edits only, never add/remove rows (see
"Row-count ceiling" below). Originally framed as two stages ("super-merchant"
selling every item, then a full editor) — dropped, since one merchant
selling every item in the game is architecturally impossible via save-file
edits (confirmed 2026-07-25).

## Confirmed facts

- Merchant shop data lives in the `USERDATA11` block of the save file.
- Editing bytes in that block and loading the save in-game has a confirmed
  effect (a raw byte edit was already tested successfully).

## Pipeline

`UserData11` = AES-256-CBC-encrypted + DCX/zstd-compressed embedded
`regulation.bin` (BND4 archive of named `.param` files). Constants,
offsets, and exact write mechanics: [MERCHANT_DATA.md](MERCHANT_DATA.md),
[ER_PVP_MOD_REFERENCE.md](ER_PVP_MOD_REFERENCE.md),
[WRITEBACK.md](WRITEBACK.md). **Critical, confirmed constraint**: the
encrypted region's row COUNT can't grow (see "Row-count ceiling" below) —
that's a BND4/ESD-script limit, unrelated to compression. Row VALUES have
no meaningful ceiling: the write path fully recompresses the patched blob
(2026-08-02, see WRITEBACK.md's "Recompression"), so editing many rows/items
at once is safe.

## Row-count ceiling (confirmed 2026-07-25 — this is why the goal changed)

Three findings together make "one merchant, every item" architecturally
impossible, not just hard:

1. The save's embedded regulation.bin archive is **100% `.param` files**
   (194/194, verified by extension scan) — no scripts of any kind. The
   NPC-to-shop-row-range binding is **not stored in the save at all**.
2. That binding lives in each NPC's **TalkESD script** (a hardcoded
   `OpenRegularShop(min_id, max_id)` call), which ships with the base game,
   entirely outside save-file reach. We can never repoint a merchant to a
   different/larger row range.
3. There is **no slack to reclaim**: 0/1277 rows in `ShopLineupParam.param`
   are unused placeholders (checked `equipId == -1` and `sellQuantity == 0`,
   both zero hits). Every row is already a live sale.

Consequence: max rows for a single merchant = whatever their own ESD range
already grants (Twin Maiden Husks, the largest, has 95). Max total distinct
items reachable across *every* merchant combined = 1277 (the game's entire
`ShopLineupParam` row budget), against a 2772-item catalog — a hard ~46%
ceiling even in the best case. See [MERCHANT_DATA.md](MERCHANT_DATA.md) for
the full row/merchant distribution data.

Twin Maiden Husks specifically turned out simpler than expected: her whole
catalog (95 rows) is **one contiguous block** (`101800-101899`), not
multiple separate shops — 72/95 rows are individually gated, 23 are
always-on (flag `0`). See [MERCHANT_DATA.md](MERCHANT_DATA.md).

The "cannot grow" constraint is about row *count* only — same-row-count
value edits (however many rows/items at once) fit comfortably now that the
write path fully recompresses the patched blob ([WRITEBACK.md](WRITEBACK.md)'s
"Recompression"); inserting new rows is the one thing still off the table
(would require re-encoding ~13MB of trailing archive content, and repointing
NPC ESD scripts outside save-file reach regardless — see above).

## Resolved (Steps 1-7 — full read+write pipeline is feature-complete)

Item ID<->name mapping ([ITEM_IDS.md](ITEM_IDS.md)); found
`ShopLineupParam.param`; solved its row schema + row->merchant mapping via
`soulsmods/Paramdex`, no hand-RE ([SHOP_LINEUP.md](SHOP_LINEUP.md)); row-count
ceiling confirmed (above); built + verified the write-back tool
([WRITEBACK.md](WRITEBACK.md)); resolved `mtrlId` (material-trade rows —
[SHOP_LINEUP.md](SHOP_LINEUP.md)'s "mtrlId resolution").

## Editor UI status

Full Gio-based two-panel editor (catalog + merchant stock, item swap,
price/quantity, weapon "+N" leveling, per-character unlock via the
Characters view) — feature-complete and shipped through several public
releases (see "Next: public release" below). Write-back runs in-process, no
backend/frontend split. See [EDITOR.md](EDITOR.md) for the Gio architecture
and [CHAR_UNLOCK.md](CHAR_UNLOCK.md) for the Characters view. There is no
per-item "clear unlock-gate" control — item edits never touch a row's own
gate (see "Goal" above). Deep cross-check audits done (rows vs. Fextralife,
item icons vs. MD5-hash collisions — see
[MERCHANTS.md](MERCHANTS.md)/[ITEM_IDS.md](ITEM_IDS.md)).

mtrlId-gated rows (Enia's 51 Remembrance-hand-in rows) are deliberately
blocked from editing — the "price" is a material trade via a separate,
undecoded table (`EquipMtrlSetParam`), not runes. Possible future work if
ever wanted (would mean decoding+writing a new param table), not an open
problem today. Enia's non-material-locked rows (item swap, price, unlock
gate) are otherwise fully supported and real-hardware confirmed (2026-08-02
all-merchants crash test).

Low-quality/inconsistent subcategory names inherited from SaveForge are
being actively re-audited against real `EquipParam*` fields rather than
SaveForge's name-derived guesses (same method as the Rotten Staff/
`key_items` fixes) — see [ITEM_IDS.md](ITEM_IDS.md) for current status.

## Next: public release

Shipped: the app is a downloadable GitHub-release tool (5 public releases
through v1.1.0). It's one pure-Go binary — Gio GUI + `app/catalog` read +
`app/shopwrite` engine in-process, icons/data JSONs embedded via go:embed,
87MB windows/amd64 exe cross-compiled from WSL with plain `go build` (no
cgo). No hardcoded default save (user opens one explicitly); edits stage
client-side and write once on Save. See `docs/EDITOR.md`, `docs/PACKAGING.md`,
`docs/WRITEBACK.md`. History of the Tauri/Dear-PyGui stacks that preceded
this one: in the Log below and git history before the pivot commits.

## Platform: PS, with external encrypt/decrypt

Target platform is **PS** (PS4/PS5), save file `memory.dat`. There are two
independent encryption layers:
1. **Outer, platform-level** encryption/signature on the whole save file.
   The user handles this separately via an **external Discord bot**
   (encrypt/decrypt round-trip) — out of scope for our tool entirely. Our
   `save_files/vanilla_fresh_character.dat` is already past this layer
   (i.e. in the same decrypted state SaveForge/er_pvp_mod expect as input).
2. **Inner, regulation.bin-level** AES+DCX/zstd encryption on the data
   embedded in `USERDATA11` specifically. This is the layer our tool needs
   to handle itself (constants/pipeline: [MERCHANT_DATA.md](MERCHANT_DATA.md)).

## External reference tools

- `/mnt/c/Users/danie/Desktop/EldenRing-SaveForge-main` — external, trusted
  full save editor. Source of `data/items.json` + Bell Bearing/TMH mechanism
  (now superseded by our own findings). See [SAVEFORGE_REFERENCE.md](SAVEFORGE_REFERENCE.md).
- `/mnt/c/Users/danie/Desktop/er_pvp_mod` — the user's own tool; its PS4
  zstd raw-block-patch technique is what `app/shopwrite` adapts and
  generalizes. Note: PS4 edits only need a *second* game launch to take
  effect when playing **online**; offline, edits load correctly from the
  first launch. See [ER_PVP_MOD_REFERENCE.md](ER_PVP_MOD_REFERENCE.md).
- **`github.com/soulsmods/Paramdex`** — community PARAMDEF/row-name schema
  source (used by Smithbox/DSMapStudio/WitchyBND); source of
  `ShopLineupParam`'s actual row schema and row-to-merchant names, no
  hand-RE needed. See [SHOP_LINEUP.md](SHOP_LINEUP.md).

## Tooling plan

See `CLAUDE.md`'s "Tooling strategy": everything shipped is Go (one binary:
Gio GUI + `app/catalog` reads + `app/shopwrite` writes). Python survives
only in dev-side `tools/` (`savescan.py` = the independent read oracle for
golden tests, `paramdex_extract`, `merchant_catalog`).

## Reference fixtures

- `save_files/vanilla_fresh_character.dat` — fresh vanilla character,
  slot 8, no edits made. Primary ground truth.
- `save_files/BetterPSN.dat` — edited by a third-party tool (Twin Maiden
  Husk's stock differs). User-confirmed it loads correctly in-game, but
  that doesn't guarantee byte-perfect internals — use as a cross-check
  signal (e.g. diffing decoded rows against the vanilla fixture), not as
  a primary source of new factual claims about file structure.

Both treated as read-only (see CLAUDE.md for the copy/backup rule).

## Log (milestones only — full history in git log; per-topic detail in the docs above)

- 2026-07-23: pipeline confirmed; AES key + PS4 offsets verified; no-growth
  write-back constraint found.
- 2026-07-24: `items.json` + `savescan.py` built; found
  `ShopLineupParam.param`; solved row schema + row->merchant mapping via
  Paramdex (no hand-RE).
- 2026-07-25: row-count ceiling confirmed (goal reframed to a free editor);
  `shopwrite` built + verified; `mtrlId` resolved; whole pipeline
  independently re-verified against external sources (zstd spec,
  SoulsFormats, AES-key provenance) with no corrections.
- 2026-07-25: `merchant_catalog.json` reconciled (35 canonical merchants);
  deep row cross-check vs Fextralife (2 misattributed-row bugs + 1 archetype
  mislabel fixed); full-catalog icon audit (8 icon bugs fixed).
- 2026-07-26: write-back end-to-end with edit UI, user-verified live. PS4
  edits need a second game launch only when playing **online**; offline they
  load from the first launch.
- 2026-07-27: repo restructured (`app/` ships, `tools/` dev-only). Public
  release stack pivoted **Tauri+React+FastAPI -> Dear PyGui -> Go/Gio (final)**,
  each fully shipped before being discarded — Tauri even published a live
  `v0.1.0` (3-OS CI, 7 installers) — dropped for 4 toolchains + 2 IPC
  boundaries on a local single-user file tool (reasoning in git log before the
  pivot commits + `docs/PACKAGING.md` "Superseded"). Read side golden-tested
  vs `savescan.py` (1277 rows exact). The Gio rewrite found + fixed a latent
  write-engine bug (zstd LZ back-references cross block boundaries, so a
  single-block edit silently corrupted the following block) — motivated the
  round-trip self-check `shopwrite.Apply` still uses
  (`TestApplySingleBlockEditRoundTrips`).
- 2026-07-28: **per-character merchant-unlock feature** (`docs/CHAR_UNLOCK.md`).
  Row unlock flags (`eventFlag_forRelease`) live in the per-character `.sl2`
  slot's event-flags bitfield, not `regulation.bin` — higher blast radius
  (can affect character progress). Region anchor found independently (8-byte
  TutorialData magic + constant `0x425` offset, verified vs all 15 slots in
  both fixtures); the flag-ID -> byte/bit packing could **not** be
  reconstructed from public data, so relicensed MIT -> GPLv3 to port
  SaveForge's algorithm/tables (`app/charflags`; attribution in
  `docs/SAVEFORGE_REFERENCE.md`). Added `app/charslot` + `app/charunlock`
  (bidirectional staged writes). No checksum recompute needed — the only
  slot-internal hash (`CSPlayerGameDataHash`) never covers the flag region —
  but writes still round-trip self-check. CLI-verified both directions;
  Characters-view GUI added, not yet human-clicked.
- 2026-07-28: editor polish — display-override auto-reset, material-locked
  rows hidden, stale-swap-tooltip/WebP-icon fixes, DLC merchant mapping,
  catalog dedup, Debug mode (red-borders risky/cut-content items).
- 2026-07-28: weapon "+N" leveling shipped (`data/weapon_reinforce.json`,
  staging + GUI); `docs/ITEM_IDS.md`.
- 2026-07-31: Settings view reworked — theme/font pickers, auto-swap
  defaults, and "Reset to Vanilla" (new `data/vanilla_shop_lineup.json`
  baseline + diff/stage revert); `docs/EDITOR.md`.
- 2026-08-02: write path **replaced per-block Raw-patch with full zstd
  recompression** (removes the ~5-6-touched-block-per-write ceiling; value
  edits now unlimited). First attempt passed every offline check but crashed
  the game — the output zstd frame must match SoulsFormats' `ZstdHelper.WriteZstd`
  shape (no checksum, no explicit content size, 64KB window), which klauspost's
  defaults don't produce. Fixed and real-hardware confirmed (608-row
  all-merchants swap loaded, every item correct); `docs/WRITEBACK.md`.
  Same day: item edits confirmed to gate the SLOT not the item, so item edits
  never touch `eventFlag_forRelease` anymore; and a reload bug for leveled
  rows fixed (`resolveItemIDWithLevel`, `app/catalog`).
