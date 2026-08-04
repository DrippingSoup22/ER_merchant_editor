# Merchant data in USERDATA11

Status: pipeline built and verified end-to-end (`tools/savescan.py`,
read-only). **Row layout solved** (2026-07-24) — not by hand-RE, but by
pulling the real PARAMDEF + row-name data from `soulsmods/Paramdex` (the
schema source the wider ER modding community already relies on; see
`docs/SHOP_LINEUP.md`). Depends on [ITEM_IDS.md](ITEM_IDS.md) for
recognizing item hits once we're reading rows (see its "two ID encodings"
section for the shop-specific `equipId`->item-id conversion).

## Known constants (copied verbatim from er_pvp_mod, confirmed identical in SaveForge)

- AES-256-CBC key for the regulation.bin blob (same for PC and PS4):
  `99 BF FC 36 6A 6B C8 C6 F5 82 7D 09 36 02 D6 76 C4 28 92 A0 1C 20 7F B0 24 D3 AF 4E 49 3F EF 99`
- Cipher: AES-256-CBC. IV = first 16 bytes right after the unk header;
  ciphertext = the rest, same length as original (can't grow).

## Pipeline (confirmed working technique, prior art from er_pvp_mod/SaveForge)

`USERDATA11` (PS4 fixed offsets, verified against our fixture:
`ud11Off = 0x1960070`, `ud11End = 0x1ba0080` = file size) contains:
unk header (0x10) -> IV (16 bytes) + AES-256-CBC ciphertext -> decrypt with
fixed key -> DCX-compressed (zstd or deflate) -> decompress -> **BND4
archive of named `.param` files**.

The archive contains `ShopLineupParam.param` (97,138 bytes) and
`ShopLineupParam_Recipe.param` (14,374 bytes) among ~194 total named param
files (full paths like
`N:\GR\data\Param\param\GameParam\merged\DLC02\ShopLineupParam.param`).
Row layout: `tools/savescan.py rows` (generic multi-row PARAM parser, no
reference codebase had one — see `docs/SHOP_LINEUP.md` for the schema).

**PS4 write-back constraint**: the encrypted region can't grow row count
regardless of write strategy (see PROJECT.md's "Row-count ceiling" — that's
a BND4/ESD limit, not a compression one). Value edits themselves no longer
have a meaningful capacity ceiling: the write path fully recompresses the
patched blob (2026-08-02, see `docs/WRITEBACK.md`'s "Recompression"),
superseding the earlier per-block Raw-patch approach and its ~5-6-touched-
block cap. Full mechanics + the write-back tool: `docs/ER_PVP_MOD_REFERENCE.md`,
`docs/WRITEBACK.md`.

## Known

- Block name: `USERDATA11`.
- TMH's unlockable stock **does** live in `ShopLineupParam` after all (rows
  101800+, per Paramdex's Names file) — correcting the earlier assumption
  that it was entirely a separate per-character EventFlag mechanism. What's
  actually true: an EventFlag (SaveForge's per-character-slot flag, see
  [SAVEFORGE_REFERENCE.md](SAVEFORGE_REFERENCE.md)) gates *release* of
  pre-existing TMH rows via those rows' own `eventFlag_forRelease` field —
  two mechanisms working together, not one replacing the other.
- **Per-item row fields (user-confirmed in-game, matches the real PARAMDEF)**:
  `quantity` (`sellQuantity`) has a safe finite range of `0..255`, `0` = out
  of stock, and `-1` = unlimited. Although the s16 field itself can hold much
  more, the game's per-row stock counter is 8-bit: purchases above 255 wrap
  (e.g. 999 becomes 231), so the editor clamps every finite entry to 255.
- Full row schema, offsets, and the `equipId`->item-id conversion: see
  `docs/SHOP_LINEUP.md`.

## Price display ceiling (user-confirmed in-game, 2026-07-31)

`value` (price) has a real practical display ceiling far below its s32
storage width, found via a controlled test (`working_copies/
pricetest_final.dat`, 20 rows on Twin Maiden Husks, see
[[test_workflow]]): prices near 1e9 (999999998/999999999/1000000000, all
tested) show **no price text at all** in the shop list — item is visible,
just blank where the cost should be — yet the purchase-gating logic still
reads the real underlying value correctly ("not enough runes" fires on
attempted purchase). User also independently observed visible **footer
icon corruption once a displayed number needs a 7th digit**, a real
rendering-buffer overflow symptom, not just a blank-text one — meaning
values in the low-millions-and-up range risk visible UI corruption, not
merely an unreadable price. Editor's `priceMax` (`app/editor/
row_edit_form.go`) tightened from the raw s32 ceiling (2147483647) to
`999999` (6 digits) as a safe conservative bound. Exact
boundary between 999999 and 999999998 not pinned (nothing in that range
tested yet) — 999999 is a deliberately conservative choice given the
footer-corruption risk, not a precisely measured one.

The earlier `qtyMax=999` conclusion was superseded by controlled player
reports: finite stock is retained in an 8-bit event value. `999` therefore
wraps to `231`; `255` is the true safe finite maximum. Use `-1` for
unlimited stock.

## "Items missing" / "wrong order" — traced to test-tooling gaps, app itself unaffected (2026-07-31)

Investigating the very first crash-test's "most items appear in a
different order" report and this session's follow-up "[item-swapped rows]
didn't show up at all" surfaced two real gaps in every throwaway test
generator this session (and the original max-coverage crash-test
fixture) — neither is a bug in the shipped editor:

1. **sellValue never synced.** Every generator called `catalog.ApplyEdits`
   directly, which writes **only** `ShopLineupParam` — never an item's own
   `EquipParam*.sellValue`. The real GUI's save path (`app/editor/
   state.go`'s `startCombinedSave`) always calls `BuildEdits()` **and**
   `BuildEquipParamEdits()` together, lowering the swapped-in item's
   sellValue to match the newly staged price whenever it would otherwise
   sit above it — this is what enforces the already-documented `price >=
   sellValue` merchant-visibility rule for a freshly swapped item. Direct
   evidence: 4 of 6 order-test items (Twinned Helm 1000, Bloodhound's Fang
   500, Lion's Claw 300, Golden Rune [3] 800) had a real sellValue well
   above the 100-rune test price — the game was correctly hiding them per
   the existing rule, not exhibiting a new bug.
2. **Gated rows weren't checked against the real save.** The row-selection
   step (a Python pre-pass over `data/vanilla_shop_lineup.json`) checked
   for an `eventFlag_forRelease` key that dataset doesn't even carry —
   always reading as unset, so gated rows silently passed the "ungated"
   filter. Real gate state (checked via the Go `catalog`/`charunlock`
   packages against the actual save) showed the flag was already
   *released* on the level-146 test character, so this particular gap
   wasn't actually the blocker here — but it's a real hole in the
   selection method, not just theoretical.

Both are now fixed in a 3rd regenerated `working_copies/ordertest_final.dat`
(flags confirmed released, sellValue rewritten to 100 for all 4 affected
items, ShopLineupParam rows confirmed correct) — byte-verified offline via
`Catalog.SellValue`/`charunlock.IsUnlocked` on the written file, not yet
confirmed in-game as of this entry. **Conclusion so far: a real user
swapping items through the actual editor UI is unaffected** —
`BuildEquipParamEdits` already exists specifically to prevent gap 1, and
the GUI reads gate state from the real save via `charunlock`, not a
static JSON. Lesson for any future throwaway test generator: (a) never
call `catalog.ApplyEdits` alone for an item-identity swap — always also
compute and apply the matching `sellValue` edit(s) via
`Catalog.SellValue`/`shopwrite.EquipParamEntryName`/
`shopwrite.LoadEquipParamSchema`/`shopwrite.ApplyWithSchema`, chained
before the `ShopLineupParam` write, exactly like `combinedApplyWorker`
does; (b) resolve gate state from the live save (`catalog.ShopRows()`'s
`Row.UnlockFlag` + `charunlock.IsUnlocked`), never from a static
generated dataset that doesn't carry that field.

If the regenerated file still shows the 4 non-Good-category items (weapon/
protector/gem) missing while the 2 Good-category ones (Rainbow Stone,
Golden Rune [3]) display fine, that would point to a 3rd, previously
unconsidered cause: Twin Maiden Husks' shop menu may be hard-restricted to
the "Goods" category tab (her entire vanilla lineup is Goods-only), and
simply not render an item of a category her menu type was never built to
show — not yet tested.

Open problems: see `docs/PROJECT.md`. Tool list: see `CLAUDE.md`'s Layout
section.

## Test log

(Append entries here: working copy used, offsets touched, before/after
bytes, in-game observation. Keep terse.)

### Schema verification (2026-07-24, read-only)

`savescan.py list`/`rows` decoded both fixtures end-to-end (vanilla 1277
rows, BetterPSN 1282). 1247/1277 vanilla rows resolve to a known item via
two independent sources (Paramdex Names + `items.json` conversion),
cross-validating the schema. Diff of all 95 TMH rows between the two saves:
every changed field landed on a semantically valid value (price->0,
quantity->unlimited, release-flag cleared) — byte layout confirmed correct,
not just internally consistent.

### Price-0 rows vanishing in-game → the `sellValue` rule

Real cause, after three superseded hypotheses:

**Shop-row visibility is a plain `price >= sellValue` comparison.**
`sellValue` is the ITEM's own per-item field (not the row's `value`), in
the `EquipParam*` table keyed by equip id (`s32`; offsets Weapon 32 /
Protector 32 / Accessory 24 / Goods 20 / Gem 32; row_size 664/416/96/176/96).
`-1` is FromSoft's "never sellable" sentinel but gets **no special
handling** — `-1` and `0` "work at any price" only because they're never
greater than a non-negative price. Vanilla-wide audit: 760 rows with
positive `sellValue`, ZERO where `price < sellValue` (the only violations
are Enia's internal Forging/Remembrance-trade rows, not browsable stock).

Correction trail (why we believe this):
- First hypothesis (2026-07-30): `eventFlag_forRelease` gating. A byte audit
  showed `value==0 && flag!=0` never occurs for a real player-visible
  vanilla row, only Enia's 15 Forging rows (`ENIA_FORGING_ROW_IDS`,
  `app/catalog/enrich.go`). Wrong: an auto-clear guard fixed nothing
  in-game.
- Second (2026-07-30, same day): the real second field is `sellValue`;
  every item that survives price 0 (Cracked Pots, Stonesword Key, etc.) has
  `sellValue == -1`, corroborated by a free BetterPSN Marika's Great Rune
  being unsellable. Theory: `-1` is an opt-out sentinel. Incomplete.
- Corrected (2026-07-31): a row raised 0->1 with `sellValue == -1` was still
  visible, but three controlled Dagger tests (real `sellValue == 100`)
  pinned it to a numeric comparison — `sellValue` forced to 50 showed only
  prices >= 50, exact at the 50==50 boundary; forced `0` behaved identically
  to `-1`.

Impl: `BuildEquipParamEdits` (`staging.go`) mirrors `sellValue` to a row's
effective price for any touched row (equality safe). `Catalog.SellValue`
decodes all 5 tables live from the loaded save, cached per load. Write path
`ApplyWithSchema`/`LoadEquipParamSchema`/`EquipParamEntryName` handle any
table (`docs/WRITEBACK.md`); `combinedApplyWorker` (`state.go`) runs an
ordered N-stage pipeline (flags → one stage per `EquipParam*` table → item
edits) chained through numbered `.tmpN` files.

### `sellValue` target: full-save true minimum, recomputed per save

`computeEquipParamTargets` (`staging.go`) recomputes each touched item's
target from scratch every call as the TRUE minimum effective price across
EVERY `ShopLineupParam` row in the whole save selling that item
(`Catalog.RowsByID()`, staged price/item where pending else current). So
`sellValue` can both **shrink and rise again** — capped by whichever
untouched sibling row still sells the item cheapest (raising past it would
hide that sibling); a raw-`-1`-price row (`row.Price == nil`) can never be
beaten, settling the target at `-1`.

Design trail: a per-batch mirror had a cross-session hole, so it first
became a running minimum clamped to never rise (2026-07-31); that
over-corrected (sellValue could never recover once every row needing it low
was gone), fixed by the full-save recompute above (2026-08-01). Covered by
`TestCombinedApplyWorkerSellValueNeverIncreasesAcrossSaves` and
`...RisesWhenSafeCappedBySiblings` (`state_test.go`). Debug Mode shows each
touched row's `sellValue` as `current -> target` in the Pending Edits modal.

### `applyZeroPriceGateGuard` removed (2026-08-02)

The 2026-07-30 `eventFlag_forRelease` auto-clear guard was built on the
already-superseded first hypothesis. It permanently deleted a row's unlock
requirement in `regulation.bin` (for every character/save) whenever a staged
price hit 0 — but the flag gates the SLOT, not the item in it, and must
never move as a side effect of a price/item edit. Deleted outright; no
staging path touches `eventFlag_forRelease` now. Unlock state changes only
via `app/charunlock` (reversible per-character bit, Enia excluded — see
`docs/CHAR_UNLOCK.md`). `staging_test.go` asserts the gate is never staged.

### Write-path capacity → full recompression (2026-08-02)

Price=0 on half of Enia's ~100-item stock hit "exceeds capacity by 32875
bytes; refusing to write." The old per-block Raw-patch growth tax (~55-65KB
per distinct 64KB block touched, ~5-6 blocks' budget) blew out on 49
distinct `EquipParamProtector` items alone (147KB over before
`ShopLineupParam` was touched); all 93 TMH rows (79 distinct items) failed
the same way, 371KB over — not Enia-specific. `BetterPSN.dat` (real,
in-game-confirmed) proved the Raw-patch assumption wrong: its stream is a
genuine full recompression (824/824 Compressed blocks, zero Raw), smaller
than vanilla's. Write path switched to full recompression (see
`docs/WRITEBACK.md`'s "Recompression"); both scenarios now pass with room
to spare (Enia: 366KB slack).
