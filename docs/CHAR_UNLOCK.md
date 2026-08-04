# Per-character merchant-stock unlock

Status: backend complete + real-hardware-verified; GUI (Characters view)
complete, went through several user-driven redesign rounds and is now in
regular real-window use with no outstanding issues. Plan/phasing history:
`character_flag_unlock_feature` session memory.

## Why this exists

`ShopLineupParam` rows carry `eventFlag_forRelease` (decoded by
`app/catalog`'s `enrich.go`) — until that flag is set for the active
character, the row stays locked in-game no matter what we write to
`regulation.bin`. The flag lives in a **per-character-slot bit array**
inside the save's `.sl2` character-slot region — a completely separate
part of the file from `USERDATA11`/`regulation.bin` (which was all this
project touched before this feature).

## Licensing

This project was MIT through 2026-07-27. The flag-ID -> byte/bit packing
scheme could not be independently reconstructed from public
Paramdex/eventparam data (falsified by a real merchant flag, id 60150 —
matches neither the naive fallback formula nor any findable coarse-range
declaration). User authorized relicensing the whole repo to GPLv3
specifically to permit porting EldenRing-SaveForge's proven algorithm and
data tables. See `docs/SAVEFORGE_REFERENCE.md`'s "Attribution" section
for exactly what was ported vs. independently derived.

## `app/charflags` — flag ID -> byte/bit, get/set

`Get(flags []byte, id uint32) (bool, error)` / `Set(flags []byte, id
uint32, value bool) error` operate on a `FlagsByteCount` (`0x1BF99F`)
byte bitfield. Resolution order: ~880-entry hand-verified exception table
(`event_flags_exceptions.go`) -> ~12k-entry BST block table
(`bst.go`/`eventflag_bst.txt`) -> fallback formula
(`byte=id/8, bit=7-(id%8)`). All three ported from SaveForge (GPLv3).
Cross-checked in `flags_test.go` against SaveForge's own
`GetEventFlag`/`SetEventFlag` via a throwaway oracle, using real
`eventFlag_forRelease` values pulled from the fixture save.

## `app/charslot` — locate a slot's event-flags region + identity

PS4 `.sl2` layout (independently established, matches
`tools/savescan.py`'s constants): `HeaderSize=0x70`, `SlotSize=0x280000`,
`NumSlots=10`. `Slots(data)` splits a loaded save into its 10 fixed-size
slot regions; `IsEmpty`/`Version` read the u32 at slot offset 0 (0 =
unused).

**EventFlagsOffset** (independently found, not from SaveForge): an 8-byte
magic `AE 00 01 00 00 04 00 00` appears exactly once per real slot,
marking the start of TutorialData; `+ 0x425` (1061 bytes) from there is
the start of the event-flags bitfield. Verified byte-exact against all 15
real slots across both fixture saves (`slot_test.go`).

**Character identity** (ported from SaveForge, `magicPattern` +
`offCharacterName`/`offLevel` in `slot.go`): a 65-byte pattern anchors
`PlayerGameData`; character name (UTF-16LE, null-terminated) sits at
`magic - 0x11B`, level (u32) at `magic - 335`. Same test cross-checks
name/level against the SaveForge oracle.

## `app/charunlock` — read side

`ListCharacters(saveData) []Character` enumerates non-empty slots
(index + name/level, via `charslot`). `LockedRows(saveData, charIndex,
rows)` / `IsUnlocked(saveData, charIndex, row)` cross-reference
`catalog.Row.UnlockFlag` against that slot's flags (verified against
fixture slots: locked-row counts move sensibly with character level).
`LockStates(saveData, charIndex, rows) (map[int64]bool, error)` is
`LockedRows`'s sibling: returns unlocked/locked state for *every* gated
row (needed so the GUI can show an already-unlocked row as a checked,
re-lockable checkbox rather than omitting it). `FlagStates` is
`LockStates`' row-agnostic counterpart, resolving raw flag IDs with no
backing `catalog.Row` (for bell-bearing flags, below).

**Caveat:** `Catalog.ShopRows()` (unfiltered) includes ~40 internal
non-merchant rows (row_id 1600101-1600123, 1600401-1600420 — the game's
starting-class-loadout table, `merchant: null`) sharing two sentinel
`eventFlag_forRelease` values (99019775 / 99019780) that don't resolve to
any bitfield position even via SaveForge's own resolver (confirmed via the
oracle — not a bug in this port). Already excluded by
`Catalog.MerchantRows` (`isBrowsable`), so they never reach `charunlock`
in real use; `LockedRows` skips an unresolvable row defensively anyway.

## `app/charunlock` — write side

`SetReleaseBatch(saveData, charIndex, targets []FlagTarget) (int, error)`
is the core write. `FlagTarget` is `{FlagID int64; Released bool; Label
string}` (generalized from an earlier `{Row *catalog.Row; Released bool}`
— `SetReleaseBatch` only ever used `Row` for its `UnlockFlag`, so the
generalization needed zero new write logic). It sets each target's flag to
its own `Released` value if not already there, in one round-trip-checked
pass — different rows in the same call can go different directions (one
staged to unlock, another to re-lock, same character). `SetRelease(...,
rows, released bool)` is a thin uniform-direction wrapper; `Unlock` =
`SetRelease(..., true)`. `SetBellBearingsBatch` is a thin convenience
wrapper over the same path.

**No per-slot checksum needs recomputing**: SaveForge's
`CSPlayerGameDataHash` (the only slot-internal hash found anywhere in its
source — see `hash.go`) hashes Level/Stats/Class/Souls/SoulMemory/
Equipment; its field list never touches the event-flags region, so a flag
edit provably can't desync it. Verified by reading the hash source, not
assumed.

**Round-trip self-check before committing** (adapted from
`app/shopwrite.Apply` to an in-place bit-flip): stage every write on a
scratch copy, then (1) confirm every byte outside the intended-touch set
is unchanged (catches a `charflags` bug aliasing two flag IDs onto one
byte), and (2) confirm every intended flag reads back at *its own* target
value (not one shared value, since mixed directions are supported). Only
then is the real buffer updated; any failure leaves `saveData` untouched
and returns an error.

`ApplyBatchToFile(inPath, outPath, edits map[int][]FlagTarget) (int,
error)` reads inPath once, applies each character's batch to the same
buffer in turn (safe: characters' flags never overlap), writes once.
`ApplyToFile` is the single-character wrapper.

`app/cmd/charunlock` is the CLI wrapper (`-list-chars`, `-list-locked`,
`-merchant`/`-rows` + `-out` to write, `-lock` to reverse). **Byte-verified**
against a `working_copies/` copy: unlocking a shared-flag row set changed
exactly the 1 predicted byte at the exact absolute offset (`HeaderSize +
slot*SlotSize + EventFlagsOffset + charflags.Position(...)`), size
unchanged, reversible. Mixed-direction/multi-character cases covered by
`TestSetReleaseBatchMixedDirections`/`TestApplyBatchToFileMultipleCharacters`
(`write_test.go`, tempdir only).

## Characters view GUI (`app/editor/character_panel.go`)

The **landing view** (`NewState` default). 3-column drill-down:
characters -> that character's gated merchants -> per-merchant flag
checkboxes. Checkboxes group by **shared `UnlockFlag`, not by row**
(`groupFlagRows` -> `flagGroup`; `flagChecks` keyed by flag id): rows
sharing one flag collapse to one checkbox, toggled together.

**Staging**: a checkbox toggle calls `stageFlag`, recording/clearing an
entry in `PendingFlagEdits` (`map[charIndex]map[rowID]bool`); staging back
to the on-disk value un-stages it (same rule as item-edit `PendingEdits`).
Staged-but-unsaved rows render amber and survive navigating between
merchants (each checkbox seeds from its staged value if present, else
on-disk).

**One shared Save button / footer** for every view, hoisted to
`window.go`'s `Layout` outside the view switch
(`layoutFooterPendingControls`, `pending_edits.go`; Save left, "Pending
(N)" right). `state.go`'s `startCombinedSave`/`combinedApplyWorker`
commits both edit kinds at once through one Save-As dialog:
- item edits only -> `Catalog.ApplyEdits`.
- flag edits only -> `charunlock.ApplyBatchToFile`, then `Catalog.LoadSave`
  so the catalog's loaded-save notion moves forward.
- both -> write flag edits to a `<outPath>.tmp` file first (never the real
  input path, satisfying `ApplyBatchToFile`'s safety check), reload the
  catalog from that tmp file, then run `ApplyEdits` on top into the real
  `outPath` — one physical file, both edit kinds, one dialog.

`consumeReset` clears `PendingFlagEdits` on any successful save/reload
(the whole app state resets on reload). The Pending dropdown lists staged
flag edits per character (un-stage from the Characters view checkboxes).
Verified end to end (real file written + re-read) by
`TestCombinedApplyWorkerMergesItemAndFlagEdits`.

**Shop Editor grid lock badge** is bound to the selected character
(`rowLockedForDisplay` in `merchant_panel.go`): with **no character
selected, every gated row shows locked** (deselecting must clear
"unlocked" back to "locked," not leave a stale character's state); with
one selected, a row shows locked only if still locked *for that
character*. Backed by `charFlagState map[int64]bool` (on-disk unlocked
state for every gated row of the selected character) +
`charFlagMerchant map[int64]string`, both computed by `ensureMerchantGated`;
`effectiveRowUnlocked(rowID)` overlays staged `PendingFlagEdits` on top so
the grid badge and live merchant recolor (`displayMerchantUnlocked`) see
staged toggles immediately.

`enrich.go`'s `rowWarnings` attaches a "gated behind event flag" warning
to every gated row unconditionally (character-independent) — it's listed
in `nonHazardWarningPrefixes` (`merchant_panel.go`) so it never
red-squares an *unlocked* gated row; keep this pairing in sync if either
side's wording changes (it already caused one regression when
`rowLockedForDisplay` became character-aware).

**Known narrow edge case:** `charFlagState`/`charFlagMerchant` are only
recomputed by `ensureMerchantGated`, which runs only while the Characters
view lays out. Saving straight to the Shop Editor without the Characters
view rendering one frame first can briefly bind to pre-reload state. Not
addressed; low impact.

The view went through several user-driven redesign rounds (see git log)
that surfaced two real Gio layout gotchas, kept as standalone facts and
now guarded by dimension-assertion tests:
- **`panelSurface` vs `barSurface`**: `panelSurface` forces
  `Constraints.Min = Max` (correct only inside a `Flexed(1)` region); a
  single-line `Rigid` bar reusing it makes `material.Button`'s background
  paint at a huge `Min`. Single-line bars must use `barSurface`, which
  reads back natural content size.
- **empty `Flexed` spacer**: a spacer returning bare
  `layout.Dimensions{}` ignores the `Min=Max=itsShare` a Flexed child is
  given and contributes zero width. Use `flexSpacer`, which returns
  `layout.Dimensions{Size: gtx.Constraints.Min}`.
- (related) `verticalDivider` trusts `gtx.Constraints.Max.Y` for its own
  height; inside a single-line bar (`Rigid` child of a Vertical Flex,
  effectively unbounded `Max.Y`) it needs an explicit height clamp
  (`dividerBarHeight`) first.

**Testing:** compiles, `go vet` clean, cross-compiles for Windows via
`app/build.sh`. State machine cross-checked against `app/charunlock` in
`character_panel_test.go`/`merchant_panel_test.go`; combined-save covered
by the end-to-end write test above. No GUI automation/screenshot tooling
in this dev environment (`xdotool`/`import` absent, no sudo) — the user's
own regular real-window use is the authoritative check for layout/binding
issues, and it has found the view working. See `docs/EDITOR.md` for the
UI-goroutine vs worker concurrency contract in `state.go`.

## Enia excluded entirely (safety-critical)

**Confirmed via real in-game test**: unlocking "all" of Enia's flags via
this view forced a Radahn boss cutscene/teleport sequence. Root cause
(byte-compared directly via `tools/savescan.py rows`): her armor-unlock
flags and her internal Remembrance-hand-in-trigger rows' flags are
**identical** to each other — e.g. "Radahn's Lion Armor" and the
"Remembrance of the Starscourge" trigger row both use flag **`9130`** —
and sit in a tight **`9100`-`9199` cluster** completely separate from
every other merchant's real bell-bearing flags (`11109710`+, confirmed
against EldenRing-SaveForge's curated `BellBearingItemToFlagID` table,
sourced from `er-save-manager`, which has **no Enia entry at all**). These
are **not shop-unlock flags** — they are the game's own "boss defeated"
signal; setting them tells any unrelated quest/cutscene logic reading the
same flags that those bosses were killed. **Any bulk-unlock path that
includes Enia can corrupt a user's save with forced boss cutscenes.**

**Exclusion mechanism**: `character_panel.go`'s `eniaMerchantName` const +
`ensureMerchantGated`'s skip (before she ever enters
`merchantGatedTotal`/`charFlagState`) removes her from the Characters
view's merchant list entirely, with a defensive no-op guard in
`selectFlagMerchant`. Cascades for free: any of her rows resolves
`effectiveRowUnlocked`'s `known=false`, so the Shop Editor grid falls back
to its "unknown = locked" default — there is no way to represent her as
unlocked through this app, the correct safe behavior. Scoped to Enia by
name specifically (not a blanket flag-range filter) since she is the only
merchant with overloaded flags (see audit below). `row_edit_form.go`'s
`gateActionSpec` checks `s.lastMerchant == eniaMerchantName` FIRST and
disables the Unlock/Lock button unconditionally for her, regardless of
character selection (2026-08-03: this must be a real disabled button, not
just descriptive text, since that same change made the button stay enabled
in the general `!known`/no-character-selected case elsewhere — see
`docs/EDITOR.md`'s "Gate action"). `setRowUnlockForAllChars`
(character_panel.go) also defensively re-checks `s.lastMerchant !=
eniaMerchantName` before staging anything, so a future call path that
somehow bypasses `gateActionSpec` still can't reach her flags. Since
item/price edits never touch any row's `eventFlag_forRelease` at all (see
`docs/MERCHANT_DATA.md` 2026-08-02), this exclusion is airtight, not merely
the only path left.

### Flag-collision audit: is Enia's bug unique?

Game-wide check (prompted also by the item-swap/ban-risk angle — see
`ITEM_IDS.md`'s "Ban-risk tier classification"), not just the one prior
finding: pulled SaveForge's curated boss-defeat flag table (`bosses.go`,
110 flags, 9xxx-range global signals) and full NPC questline table
(`quests.go`, 2747 flags), decoded
every browsable row's `eventFlag_forStock` (set *by* a purchase — the
dangerous write path) and `eventFlag_forRelease` (gate/visibility —
read-only) for all 991 rows, cross-referenced against both.

- **`eventFlag_forStock` (write path): exactly 1 collision game-wide, and
  it is benign** — TMH's own vanilla row 101802 (Spirit Calling Bell) sets
  flag 60110, which `quests.go` defines as that item's own "you obtained
  it" tracker. Also, item/price edits never write `eventFlag_forStock` at
  all, so swapping any item onto any row can never introduce a new
  collision.
- **`eventFlag_forRelease` matching an *actual boss* flag: 50 rows, all
  Enia, nothing else** — confirms she is the only merchant with this bug.
- **Other `eventFlag_forRelease` hits (53 rows): all benign questline
  flags** — Brother Corhyn 21, Miriel 21, Patches 6, Knight Bernahl 2,
  Gatekeeper Gostoc 1, Gowry 1, Iji 1. Each is that merchant's own
  single-meaning questline-progress flag, legitimately reused by their own
  shop rows by design (e.g. Corhyn's Black Flame rows gate on 11109876 =
  "Giving him Godskin Prayerbook", the same vanilla milestone). Forcing
  one only fast-forwards that NPC's own dialogue by a step — not a boss
  kill, not an unrelated system. Left as-is; only Enia warranted exclusion.

The dividing line: Enia's `9130` meant two *unrelated* things at once (an
armor-drop trigger AND the real Radahn-defeated global signal); the other
7 merchants' flags each mean one coherent thing.

## Bell-bearing acquisition toggles for Twin Maiden Husks

TMH's real in-game mechanic: giving her another merchant's Bell Bearing
key item makes her *also* sell that merchant's whole real stock. Two
unrelated things share the name "Bell Bearing":

- **~15 flags** (Peddler/Miner/Glovewort-Picker/DLC-food NPCs) gate
  ordinary rows already inside her own row block (101800-101899) as that
  row's `eventFlag_forRelease` — e.g. flag `11109751` = rows 101808/101809
  (Smithing Stone [1]/[2]). **Already handled** by the per-row checkboxes,
  no new support needed.
- **~40 flags** (Patches, Sellen, Corhyn, and the Nomadic/Isolated/Hermit/
  Imprisoned/Abandoned wandering merchants) never appear in
  `ShopLineupParam` at all — pure talk-script gates (same class as the DLC
  Grand Altar "Heart of Bayle" case) controlling whether her talk-script
  opens that merchant's own existing row range through her (that
  merchant's own per-row gating still applies on top). **New support.**

**Data**: `app/charunlock/bell_bearings.go`'s `BellBearing`/`BellBearings`
table (62 entries, ported from EldenRing-SaveForge, itself citing
`er-save-manager` — see `docs/SAVEFORGE_REFERENCE.md`). `BellBearing`
carries SaveForge's `Category` taxonomy (`npc`/`merchant`/`peddler`/
`smithing`/`dlc`) as sourced data (not currently bucketed by in the UI).
`BellBearingsForUI()` filters out the already-row-covered subset.
Merchant-name mapping: the 15 NPC-category entries map 1:1 by name to
`data/merchant_catalog.json`; of the 18 numbered wandering entries, only 8
got high-confidence Fextralife matches — the other 10 ship with
`Merchant: ""` per the "don't guess-merge" rule (see `docs/MERCHANTS.md`'s
`unknown_merchant` precedent).

**Staging + write merge**: new map `PendingBellBearingEdits` (charIndex ->
bell-bearing flagID -> target), same shape/rule as `PendingFlagEdits`,
kept separate since row IDs and bell-bearing flag IDs are different ID
spaces. Both maps are **merged into the same `flagTargets`/
`SetReleaseBatch` call in `startCombinedSave` — one write stage, not two.**
The Pending modal's "Character unlocks" card sums both maps per character;
`RemovePendingFlagsForChar` clears both.

**Not cross-referenced elsewhere (explicit non-goal)**: the Shop Editor
grid's purple-lock display for *other* merchants' own rows is not made
"reachable via TMH"-aware — it reflects a row's own real unlock state
regardless of access route; adding that awareness is unneeded complexity.

**Layout** (`layoutTMHFlagsGrid`, TMH-specific, mirrors the
`eniaMerchantName` special-case pattern but additive): **one scrollable
`material.List` / `Flexed(1)` region, one scrollbar (`tmhColList`), no
trailing `Rigid` sibling** — 3 sections stacked top to bottom, each
skipped when empty. This single-region structure is a **hard invariant**:
an earlier version put a `Rigid` section after the list's `Flexed(1)` in
one Flex, and the trailing section only got leftover space (only ~12 of
~36 rows visible, unscrollable) — same Flex-ordering gotcha as the
`panelSurface`/`flexSpacer` bugs above. `tmhFlagSections` (pure
classification, no rendering) + `TestLayoutTMHFlagsSectionCoversEveryEntry`
guard it (a "does it panic" test would not have caught the clip). The 3
sections:

1. **TMH Bell Bearings** — her own gated-row groups whose flag matches a
   `BellBearing` entry, labeled with the bearing's name
   (`layoutFlagRowNamed`) with unlocked item names as muted subtext. One
   item per row (`addTMHSection` — the subtext can run long, keeps full
   row width). Still staged through the **row-based**
   `stageFlag`/`PendingFlagEdits` path (deliberately **not** the flag-only
   `stageBellBearing`/`PendingBellBearingEdits` path): only
   `PendingFlagEdits` feeds `effectiveRowUnlocked`/`displayMerchantUnlocked`,
   so the flag-only path would silently drop staged state from the Shop
   Editor grid preview and merchant recolor.
2. **NPC Bell Bearings** — `BellBearingsForUI()` in full
   (`layoutBellBearingRow`), staged via `stageBellBearing`/
   `PendingBellBearingEdits`. Rendered as **two balanced independent columns**
   sharing one scrollbar: named Shop 1/2 entries, Kalé, then Shop 5 at left;
   Nomadic [1–10] then the other Shop 4 families at right. This keeps each
   numbered family vertically scannable and avoids blank trailing rows.
   Guarded by `TestNamedBellBearingShopSequence` /
   `TestNPCBellBearingColumnsKeepNomadicFamilyVertical`.
3. **Other Items** — her gated-row groups with **no** `BellBearing` match,
   item-name labeled (`layoutFlagRow`), 2 items per row row-major
   (`addTMHSectionPaired`, blank trailing cell if odd). This is her rows
   whose flag is a bell-bearing-range flag with no `BellBearings` table
   entry (9 flags `11109770`-`11109777`/`11109781` gating DLC-tier/catch-up
   rows never a real key item) plus 2 rows (Spirit Calling Bell / Lone
   Wolf Ashes) on an unrelated quest flag `1042369416`.

TMH Bell Bearings + Other Items together are the union of her own gated
rows (neither opens a *different* merchant's shop through her); NPC Bell
Bearings is the set that does (`Covered==true` vs `false`). "Check all
remaining" covers both mechanisms (row groups + `BellBearingsForUI`).

**Not verified in-game**: these ~40 flags have never been set by any tool
(SaveForge only *lists* the table — it never wrote `ShopLineupParam`).
`go test ./app/...`, `go vet ./...`, `app/build.sh` cross-compile, and a
headless run all pass; whether TMH actually shows the other merchant's
wares afterward is the user's own follow-up.

## Scroll/Prayerbook unlocks for Brother Corhyn, Miriel, Sorceress Sellen

2026-08-01 user request: give their scroll-gated row groups TMH's own
"named group + unlocks subtext" treatment instead of a flat item-name
checkbox list. Their real mechanic: giving one of 14 base-game Sorcery/
Incantation Scrolls to a "learned sorcerer/cleric" unlocks the spells it
teaches — unlike TMH's bell bearings, never opens a *different* merchant's
shop, so there's no NPC-Bell-Bearings-equivalent section, just 2: named
(scroll-matched) groups + a plain "Other Items" fallback for gated rows no
scroll explains (each merchant's own independent questline-progress
unlocks, e.g. Corhyn's Great Heal/Discus of Light/Immutable Shield,
Sellen's Shard Spiral).

**Data**: `app/editor/character_panel_scrolls.go`'s `scrollUnlocks` (11
scrolls with a real spell list; 3 more — Erdtree Prayerbook, Erdtree Codex,
Golden Order Principles — have no spell list even in SaveForge's own data,
likely non-functional cut-content duplicates of Golden Order Principia,
omitted), ported from EldenRing-SaveForge's curated FMG item captions
(`data/item_text_generated.go`, each scroll's `Caption` literally lists
"Can be given to a learned sorcerer/cleric to gain access to the following
sorceries/incantations: - X - Y"). Matched to a merchant's gated-row group
by **exact item-name-set equality** (`scrollNameForGroup`), not a flag-ID
table like `BellBearing`'s — a scroll's real event flag differs per
merchant (e.g. Fire Monks' Prayerbook is flag `11109874` at Corhyn but
`1037469305` at Miriel), so flag ID doesn't generalize across the three the
way spell-set identity does. Cross-checked against the fixture save
(`tools/savescan.py`) with zero partial/ambiguous matches: 8/11 of Corhyn's
gated groups, all 11/11 of Miriel's, 3/4 of Sellen's — guarded by
`TestScrollFlagSectionsMatchesFixtureSave`.

**Layout** (`layoutScrollFlagsGrid`, mirrors `layoutTMHFlagsGrid`'s single-
`Flexed(1)`-region invariant, own scroll-position widget `scrollColList` —
not shared with `tmhColList`/`flagColList`, same reasoning as those already
being kept separate): "Scrolls & Prayerbooks" section (one item per row,
`layoutFlagRowLabeled` — `layoutFlagRowNamed`'s `charunlock.BellBearing`-
free counterpart, since a scroll isn't a bell bearing) + "Other Items"
(2 per row, `addTMHSectionPaired`). Staged through the same **row-based**
`stageFlag`/`PendingFlagEdits` path every other merchant's flat list uses
(no new staging mechanism needed — these are ordinary gated `ShopLineupParam`
rows, not TMH's bell-bearing-flag special case). Dispatched in
`layoutFlagsColumn` via `scrollFlagsMerchants` (a plain 3-name set, checked
alongside the existing `twinMaidenHusksMerchantName` branch).

## Merchant ordering (`catalog.MerchantSortKey`)

Exported from `app/catalog/catalog.go` (was `merchantSortKey`) so
`app/editor` reuses the identical scheme (SSOT) for both the Shop Editor
filter dropdown and the Characters view merchant column
(`sortedGatedMerchants` sorts by it). Group order:

- 0 = Twin Maiden Husks (hardcoded first).
- 1 = Bell Bearing Shop 1's base-game specialist sellers, in game-menu
  order: Sellen, Seluvis, Thops, Corhyn, Miriel, D, Gowry, Rogier, Bernahl,
  then Iji. Miriel and Gowry each have two game-menu entries (Sorcery and
  Incantations) but one save flag.
- 2 = Bell Bearing Shop 2, in game-menu order: Gatekeeper Gostoc, Pidia,
  Patches, then Blackguard Big Boggart.
- 3 = Bell Bearing Shop 3: Nomadic merchants, by Bell Bearing number.
- 4 = Bell Bearing Shop 4: Kalé, then Isolated, Hermit, Abandoned, and
  Imprisoned merchants (numbered families keep their Bell Bearing order).
- 5 = Bell Bearing Shop 5: Moore/Thiollier, then Count Ymir. Thiollier has
  no separate Twin Maiden Husks flag/checkbox; Moore's bearing carries his
  progressed wares into the same Shop 5 stock.
- 6 = independent/non-TMH shop entries.
- 7 = Dragon Communion altars.

Within the NPC Bell Bearings section, `bellBearingGroupRank` ranks each
`BellBearing` into the same Shop 1–5 order. The Characters view deliberately
uses balanced family columns: named Shop 1/2 entries plus Kalé/Shop 5 at left,
and the full Nomadic sequence plus the other Shop 4 families at right. The
Shop Editor filter uses the same shared sort key. Shops 1 and 2 use the
game's named-bearing sequence (the game itself turns each into one item-sorted
stock grid, so it has no seller-order view). Guarded by
`TestMerchantSortKeyOrdersGroups` (`app/catalog`),
`TestBellBearingGroupRankClustersShopFamilies` /
`TestBellBearingSortKeyGroupsFamiliesByNumber` /
`TestNamedBellBearingShopSequence` (`app/editor`), and
`TestThiollierHasNoSeparateTMHBearing` (`app/charunlock`).
