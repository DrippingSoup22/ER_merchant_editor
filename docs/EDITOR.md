# Editor UI

Gio (gioui.org) desktop app, pure Go (replaced a same-day Dear PyGui
version — see `PACKAGING.md`'s "Superseded"). One process, one binary:
`internal/ui/gio/` (GUI) calls `internal/catalog/` in-process for reads and
`internal/savefile/`'s `Apply` in-process for writes. No Python at runtime.

## Run (dev)

`go run ./cmd/ermerchanteditor` (repo root). Starts with no save loaded; the
header opens one via a native dialog (ncruces/zenity — pure syscalls on
Windows, the `zenity` binary on Linux dev). `ER_EDITOR_SAVE=<path>` loads a
save at startup — dev only, never point it at `save_files/` (copy to
`working_copies/` first). Linux dev builds need cgo + X11/EGL dev packages
(see PACKAGING.md); the Windows target is cgo-free.

Starts maximized (`cmd/ermerchanteditor/main.go`). Gotcha: `app.Size` resets the
window mode to Windowed per its own doc comment, so `app.Maximized.Option()`
must come AFTER `app.Size`/`app.MinSize` in the option list to take effect.

## Data/logic layer (`internal/catalog`, package catalog)

Port of the former Python catalog and `tools/savescan.py` enrichment, golden-
tested field-for-field against `tools/savescan.py rows` (1277 rows, zero
mismatches — `decode_test.go`, regenerate the golden with
`tools/.venv/bin/python tools/savescan.py rows save_files/vanilla_fresh_character.dat > working_copies/rows.golden.jsonl`).

- `New()` loads embedded reference data (`data/embed.go`).
- `LoadSave` sanity-decodes first; failure keeps the prior save.
- `ListMerchants` — browsable = kind `merchant` + the 3 Dragon Communion
  altars; sorted by Twin Maiden Husks' real Bell Bearing Shop 1–5 grouping,
  then the Dragon Communion altars (`MerchantSortKey`).
- `MerchantRows(name)` — enriched `Row` (item name/icon, price nil when -1,
  cost type, quantity, unlock flag, materials, warnings,
  `MaterialLocked = mtrlId != -1`), sorted by stable ShopLineupParam row ID
  for the editor's **Edit layout**. **Game preview** separately applies Elden
  Ring's category-first menu order, then `sortGroupId` and `sortId` ascending
  descending within that category, including staged swaps, so the user can
  inspect the real in-game result on demand.
- `ListItems/ListCategories/ListSubcategories` — items.json catalog;
  `Item.EquipType == nil` means not sellable (no equip-slot mapping).
- `ApplyEdits(edits, outPath)` — validates (empty, out==current, unknown
  row ids, material-locked → typed `EditError`, nothing written), then
  `shopwrite.Apply` **in-process**; success = Save-As semantics (current
  save becomes outPath, row cache invalidated). `shopwrite.Apply` itself
  round-trip-verifies before writing (see `WRITEBACK.md`).

## GUI architecture (`internal/ui/gio`, package gio)

`state.go` owns shared state: the `Catalog`, staged `PendingEdits` (keyed by
row id; staging a field back to its original value unstages it, empty
entries pruned), selection, picking mode, and the load/apply worker
lifecycle.

**Concurrency contract (safety-critical, other code depends on this):**
UI-goroutine-owned fields vs a small mu-guarded set written by workers;
workers never touch UI fields directly — they set flags
(`resetPending`/`applyDone`) that `consumeReset` applies at frame top. Save
loads and edit applies run on worker goroutines (decrypt + zstd of the 53MB
blob takes ~hundreds of ms).

## Catalog panel (`catalog_panel.go`)

Search + category/subcategory cascade, responsive icon grid (`widget.List`
lays out only visible rows), picking-mode banner, unsellable items disabled
while picking. Cell size is user-set via a Settings slider
(`Settings.CatalogGridCellSize`, default 90dp; `catalogCellSize()`).

**Category filter:** raw `items.json` category ids (snake_case, e.g.
`key_items`) shown as friendly labels in Elden Ring's menu order: Tools,
Spirit Ashes, Crafting Materials, Bolstering Materials, Key Items, spells,
Ashes of War, weapons, ammunition, armor, Talismans, then Info Items
(`categoryOrder`/`categoryLabels`/`orderedCategoryOptions`; unknown categories
sort alphabetically after the known ones, never disappear). The empty "no filter" option displays "All
Categories"/"All Subcategories" while the underlying filter value stays `""`
(`Combo.labels`, see widgets).

**Weapon Level control** (`s.PickLevel`, slider + exact-value box) — a
prospective setting, not a filter: it applies to whatever weapon-table item
gets picked or drag-filled next (`stageItemSwapCore`), clamped to that
specific item's own max so a level set for one weapon can't leak onto a
lower-max weapon picked afterward. One control, not per-cell (same reasoning
as the single search/category bar). Lives on the "N items" line (right-
aligned via `layout.Flexed(1, flexSpacer)`), hidden
(`weaponLevelCategoryVisible`) unless the category filter can show weapon-
table items (All Categories, Melee Weapons, Ranged Weapons & Catalysts,
Shields). Slider fixed 0-25 range (`pickLevelMax`, the global max across
weapon-table items; a lower-cap item clamps down silently at stage time).
Slider and box stay in sync both ways: dragging updates `PickLevel` + box
text; typing + Enter updates `PickLevel` + snaps the slider `Value`, but
only while not actively dragging (`Dragging()` guard, else a mid-drag frame
fights the gesture). The box clamps live as you type (`liveClampedInt`,
drains `widget.ChangeEvent` not just `SubmitEvent`); its `Filter` excludes
`-` so no negative can be typed. A hovered weapon-table item's tooltip
(`layoutItemGrid`) appends the "+N" it would be picked at right now
(recomputed fresh every frame). See `docs/ITEM_IDS.md`'s "Weapon
reinforcement levels".

**Layout:** the count row's height is pinned to `filterCountRowHeight` (38dp,
`fixedMinHeight`, content centered) whether or not the Weapon Level control
shows, so switching categories never shifts the grid's start position; the
Merchant panel's stock-count line uses the same fixed height so the two
panels stay symmetric.

**Multi-select** (sellable cells only, ORDERED = replacement order): plain
click = select (re-click of the sole selected item deselects), Ctrl/Cmd-
click = toggle at end, Shift-click = contiguous range in visible order from
the anchor; #5AA0FA border on selected cells. Deselection is press-based
(pass-through Press listeners): any press outside the catalog grid clears
the selection; ending a drag that actually moved clears it too. Selection
cleared on save load.

**Drag-and-drop** (`widget.Draggable` source / `io/transfer` targets, MIME
`application/x-er-items`, payload = comma-separated selected item ids): every
sellable cell is always a drag source. A plain click (press+release, no
movement) only reaches the underlying `Clickable` and keeps its multi-select
semantics (`Draggable` requires movement past its slop threshold before
`Dragging()` latches); a press-and-move starts a drag immediately regardless
of prior selection. Dragging a cell not already in the selection selects
just that cell the moment the drag initiates (`transferInit`) — for correct
border highlight and ghost count; dragging an already-selected cell drags
the whole group. The ghost chip (item icon + xN badge) and amber drop-span
highlight appear only once the drag actually moves (`transfer.InitiateEvent`).
The span marks the cells a drop would fill: from the hovered cell onward
(per-target Enter/Leave works mid-drag for mime-matched targets), skipping
material-locked rows WITHOUT consuming an item, excess ignored; each fill
stages a swap like a pick (shared `stageItemSwapCore`). `applyDrop` stages
directly into `s.PendingEdits` (a standalone action, independent of whether
the row-edit modal is open), then calls `openEditorForDropped` -- a no-op
unless `Settings.OpenEditorAfterDrop` is on, see the Settings section.

Gio DnD gotchas: `gesture.Drag` sets `dragging=true` ON PRESS (keep mid-
flight cells registered or `Dragging()` latches); drag/target areas sit atop
click areas and are input-opaque without `pointer.PassOp`; a drag past slop
grabs the pointer, cancelling the click.

Every staged swap also auto-stages `-1` into the row's non-`-1` display-
override fields (`iconId`/`nameMsgId`/`menuTitleMsgId`/`menuIconId`, carried
on `ItemChange.ClearOverrides` so Undo swap drops them too) — the new item
then shows its own name/icon in-game. Because of this the override warning
is no longer a red-square hazard (GUI filters it via `hazardWarnings`,
catalog output unchanged for oracle parity; popup shows a muted "[i]" note).

## Merchant panel (`merchant_panel.go`)

Merchant combo, subcategory filter, "N items in stock" line, grid. Cell size
is user-set (`Settings.MerchantGridCellSize`, default `merchantCellSizeDefault`
100dp; `merchantCellSize()`). `layoutGrid` (window.go) takes an explicit
`cellSize unit.Dp` param; catalog/pending grids pass `widgets.IconCellSize`,
only the Shop Editor passes the user value.

Border precedence: selected #5AA0FA > pending #F0B450 > warnings #DC5A5A. A
cell with a staged swap shows the NEW item's icon. Unlock-gated rows are NOT
in the border precedence — `widgets.IconCell.Locked` draws a small corner
padlock badge (`drawLockBadge`, art `widgets/assets/lock_badge.png` embedded
+ decoded once in `widgets/lock_badge.go`), independent of `cellBorder`, so
a locked row can show a badge AND a selected/pending/warn border at once.

**On-cell price/quantity overlay** (mimics the in-game shop list):
`widgets.IconCell` has two generic (no domain knowledge) optional fields —
`CornerBadge string` (small number in the icon's own bottom-right corner,
mirrors `Locked`'s top-right positioning) and `Footer layout.Widget` (a strip
rendered BELOW the icon inside the same click/hover region; cell height grows
to fit it, measured once via `op.Record`). `merchant_panel.go` wires
`CornerBadge` to stock quantity (blank for -1/unlimited) and `Footer` to a
price line (`rowPriceFooter`) — always attached even for a priceless row
("-" fallback), so every merchant-grid cell shares one height (a footer only
some cells had would misalign row bottoms). Both read through `rowPriceQty`,
which prefers `effectiveRowEdit`'s staged/drafted value over the committed
one, so a pending price/qty edit shows on the grid cell live. Currency icon
is resolved per row `CostType` (`field_meta.go`'s `currencyIconPath`): named
cost types (Dragon Hearts/Starlight Shards/Heart of Bayle) get their real
vendored icon; costType 0 (runes, the majority) uses `shadow_realm_rune_5.png`.
The overlay is deliberately read-only (editing goes through the row-edit bar).

Gio gotchas learned here: (1) `widgets/iconcell.go`'s `content()` scopes the
background fill/hover tint/disabled veil/Border to the icon square (`iconSz`)
only, NOT the full cell (`sz`) — so a Footer reads as transparent chrome
below a "boxed" icon (the game's item-card look); the Clickable hit region
stays full `sz`. (2) `drawCornerBadge` must loosen the cell's `Exact`
(`Min==Max`) constraints before measuring its label, else the label is forced
to fill the cell and its backing rect paints over it. (3) `rowPriceFooter`
can't use `layout.Direction` for alignment — the incoming Max.Y is the
unbounded `content()`-measurement sentinel; it right-aligns manually
(`op.Record` + offset within the known width) and centers vertically via
symmetric padding.

Cell size scaling: `widgets.BadgeScale(size)` gives a ratio against the 80dp
baseline (1.0 at 80dp), applied to `drawCornerBadge`'s text size and
`rowPriceFooter`'s icon/text so bigger cells enlarge the detail, not just the
artwork. `footerCurrencyIconSize` is 18dp (row-edit bar's `fieldIconSize` is a
separate 14dp). Currency icons carry heavy transparent padding in the source
PNGs (`shadow_realm_rune_5.png`'s rune fills ~half its 256x256 canvas), so
`currency_icons.go`'s `currencyIcon(path)` decodes each once, crops to its
own alpha bounding box (`cropToContent` + small margin), and caches —
separate from the shared `IconCache` (`icons.go`) so the Catalog/pending
views still show these items uncropped.

**Cell-size sliders** (both panels, Settings view): "sticky" via
`stickyCellSize(raw)` — a value within `cellSizeSnapBand` (3dp) of a notch
(`cellSizeSnapStep` 10dp, `cellSizeSnapMin/Max` 60/160dp) pins to that exact
notch; every other value passes through unchanged, so the full continuous
range stays reachable with an easy-to-land-on dead zone at round numbers.
`layoutSettingsPanel` writes the sticky result back into the slider's own
`widget.Float.Value` so the thumb pauses at a notch too (safe — Gio's
`float.go` `Update` recomputes `Value` fresh from the pointer's tracked drag
position each event, never reading `Value` back).

Any press outside the merchant grid deselects the row (exceptions: the
row-edit modal itself -- own scrim/press-listener handles that, see below --
and picking).

**Internal drag-and-drop (row swap, 2026-08-01)**: every merchant cell is
ALSO a drag source now (`rowsMIME`, distinct from the catalog grid's
`itemsMIME`), same press-vs-drag contract as the catalog grid (plain click
keeps `handleRowClick`'s select semantics; a press-and-move starts a drag).
Dropping one merchant row onto another **swaps** their items both
directions (`applyRowSwap`/`effectiveRowIdentity`, staging.go) rather than
the catalog grid's one-way replace — each side hands the other its CURRENT
displayed item (an already-staged swap wins over the on-disk value, so
chaining swaps composes correctly). Single-row only, no multi-select group
semantics (a swap is inherently pairwise). Never touches `SelectedRowIDs`/
`SelectedItems` as a side effect (`dragFromRow` tells `window.go`'s Layout
not to run the catalog-drag's end-of-drag `clearSelection()` for a
row-origin drag, since it never touched that selection). `dropSpan`
highlights just the one hovered target cell for a row-origin drag (a swap
isn't a multi-item fill).

## Row-edit form (`row_edit_form.go` + `row_edit_layout.go` + `bar_chrome.go`)

The per-row edit form: a floating modal (2026-08-03, converted from a bar
docked at the bottom of the Merchant Stock column, to visually match Pending
Edits/Item Info) -- full-window dim scrim + a centered bordered panel, via
the shared `widgets.Backdrop` helper (`layoutRowEditOverlay`, drawn in
`window.go`'s `Layout` alongside the other two modals, NOT inline in
`layoutMerchantPanel` anymore, so the merchant grid no longer shrinks to
make room for it). `rowEditModalTag`/`rowEditModalHit` (whole overlay,
registered in the Backdrop's `afterPanel`) and `rowEditPanelTag`/
`rowEditPanelHit` (the panel only, registered inside `layoutRowEditPanel`
itself, sized to its own measured `dims.Size`) distinguish "clicked the
panel" from "clicked the scrim" — `rowEditModalHit && !rowEditPanelHit`
closes it, same pattern as `itemInfoModalHit`/`itemInfoPanelHit`.

Shown whenever a row is selected (`showRowEditor`) — but HIDDEN entirely
while picking (`layoutRowEditOverlay` returns early on `s.Picking()`): unlike
the old docked bar (confined to the merchant column, off to the side of the
catalog grid), a window-covering modal would otherwise block the catalog
grid with its own scrim during a "Change item" pick. The catalog panel's own
picking banner/Cancel button (`catalog_panel.go`) already covers
cancellation, so nothing is lost; the modal reappears once the pick
completes or is cancelled.

**Draft-then-commit model (load-bearing architecture):** every edit the bar
makes (price/qty/level/item-swap/gate) lands in a DRAFT first
(`State.draftEdits` / `draftGateEdits` map[int64]draftGateEdit;
`draftRowIDs` = the selection snapshot the draft belongs to) instead of
`PendingEdits`/`PendingFlagEdits` directly. `staging.go`'s
`StageField`/`stageItemSwapCore`/`StageItemLevel`/`ClearItemSwap`/
`setRowEntry`/`currentEntry` all take an explicit `edits map[int64]*RowEdit`
param so the SAME functions serve both targets (the bar passes `s.draftEdits`;
drag-and-drop passes `s.PendingEdits`).

`ensureDraft` (state.go), called unconditionally every frame
`layoutMerchantPanel` runs, reconciles `draftRowIDs` against `SelectedRowIDs`:
the moment they differ (different selection, OR the bar closed to none), the
old draft is thrown away and `reseedDraft` deep-copies fresh from
`PendingEdits` for the current selection — one check handling both "switched
selection" and "closed then reopened the same rows" (the latter needs
`ensureDraft` to run even while the bar is CLOSED, else a stale draft could be
shown again on reopen). `applyDraft` commits `draftEdits`/the gate target into
the real `PendingEdits`/`PendingFlagEdits` (via `setRowUnlockForSelectedChar`)
then immediately reseeds the draft from what was just committed — **Apply is a
checkpoint, not a close** (though the Apply button additionally closes the bar
via `clearRowSelection`; the X button discards the draft and closes, same as
clicking outside).

The merchant grid's icon/border/tooltip read `effectiveRowEdit`/
`inDraftSession` (state.go), so a row open in the bar shows its LIVE draft
preview while other rows show only committed state. `RemoveRowEdits`/
`RemoveMerchantEdits` (Pending modal) also drop any matching draft entry,
guarding one edge case: the modal draws over the bar without closing it, so
Removing a row there while its draft is open could otherwise resurrect the
edit on next Apply.

**Layout:** `editingRows()` resolves the selection to `[]*catalog.Row` in
order; the bar renders whenever non-empty. `formHeader` dispatches on count:
1 row = icon + name identity display (`formHeaderSingle`); N>1 = "N items
selected" `countPill` (in the panel title row) + a wrapping ICON GRID preview
(`formMultiItemList`, hand-rolled at `formMultiItemIconSize` = `headerIconSize`
scale, reusing `widgets.IconCell`'s `Size unit.Dp` field for hover/tooltip/
border chrome; the name + a "(pending swap)"/"(pending level change)" tag
moved into the tooltip). The list sizes to real content height and only caps
at `formMultiItemListHeight` for a large scrolling selection, so a 1-row multi-
selection is pixel-identical to a single-row selection's icon. Headers are
pure identity displays — no action buttons near them.

**Fields** (in a bordered `groupBox` card): price/quantity/weapon-level via
`labeledEditor`, right-aligned via trailing `layout.Flexed(1, flexSpacer)` so
all controls share one right edge. Price and quantity **live-clamp as you
type** (`liveClampedInt`): price to `[0, priceMax]` where `priceMax = 999999`
(6 digits — the largest value confirmed to render cleanly; the raw `value`
field's s32 ceiling caused real in-game display corruption well below that,
see `docs/MERCHANT_DATA.md`'s "Price display ceiling"); quantity to
`[qtyMin, qtyMax] = [-1, 255]` (-1 = unlimited; 255 is the largest safe
finite stock because the game retains purchases in an 8-bit counter). Typing
or pasting a larger finite value live-clamps it to `255`. `priceEditor`'s
filter excludes `-`. Leading glyphs: `batchPriceIcon`
(real per-cost-type currency icon) for Price, `stockIcon`
(`assets/stock_sack.png` via `field_icons.go`) for Quantity. Price is labeled
per costType via `field_meta.go`. "Weapon level (max +N)" shown only when
`weaponLevelInfo` says the row's current-or-staged item is weapon-table with a
nonzero max.

**Action row** (`formActionsRow`, unconditional for single AND multi, bottom
of the fields `groupBox`), always exactly 4 buttons in this order: Undo
swap(s), gate action, Change item(s), Apply. All use the `barButton` style.
Laid out by `wrapButtons` (a hand-rolled wrap/flow — Gio has none built in;
same measure-then-`op.Offset` approach as `formMultiItemList`), NOT a plain
horizontal Flex: a Flex of `Rigid`s squeezes its TRAILING children on
overflow, which rendered "Apply" as an unreadable sliver once the form
became a fixed-width modal (2026-08-03 user screenshot). Each line is
right-aligned and every button keeps its natural width, wrapping to a new
line instead of shrinking. The modal is 720dp wide so all 4 fit on one line
even with the longest gate label ("Unlock all (All Characters)"), with
headroom for a 5th button; `TestFormActionsRowFitsModalWidth` guards that
width-vs-label relationship and `TestWrapButtonsNeverSqueezes` the wrap
behavior itself.
Buttons never disappear, only grey out (`actionButton`/`disabledBarButton` —
a filled rounded rect + muted text with NO `widget.Clickable` laid out that
frame, so a greyed button truly can't be clicked; mirrors `pending_edits.go`'s
`disabledButton`). Apply greys when nothing is staged
(`len(draftEdits)+len(draftGateEdits)==0`); Undo swaps when nothing selected
has a staged swap; Change item(s) only while actively picking. The X button
is the only button left in the header (`layoutRowEditPanel`).

**Gate action** (`gateActionSpec`, single-row only vs multi): Enia is checked
FIRST and disables the button unconditionally (her flag ids alias real
boss-defeat flags, see `eniaMerchantName`'s doc comment in
`character_panel.go` — this used to be covered by an informational status
line, removed 2026-08-03, so the button itself must now enforce it). For a
multi-selection it's "Unlock all" (`draftUnlockAll`, drafts an unlock for
every DISTINCT `UnlockFlag` among selected rows); for exactly one row it's
the Unlock/Lock TOGGLE (`toggleDraftGate`, can re-lock, unlike Unlock all).
Both route through `setDraftGate` → on Apply `setRowUnlockForSelectedChar`,
which uses the exact same `PendingFlagEdits` staging a Characters-view
checkbox toggle does (keyed off the char-wide `charFlagState` cache via
`ensureMerchantGated`) — NOT a `eventFlag_forRelease` field edit on the
ShopLineupParam row (that would permanently ungate for EVERY character and
never touch the grid's lock badge). Stages every row in `s.MerchantRows`
sharing the clicked row's `UnlockFlag`, not just that one (Twin Maiden
Husks' bell-bearing purchase releases a batch under one shared flag = one
`flagGroup`/checkbox in the Characters view).

**No character selected** (2026-08-03 user request): the button stays
enabled instead of disabled, labeled "Unlock (All Characters)"/"Unlock all
(All Characters)" — never "Lock", since there's no single per-character
value to toggle off of. `setRowUnlockForSelectedChar` (character_panel.go)
falls through to `setRowUnlockForAllChars` when `SelectedChar < 0`: for
every character in `s.CharList` it independently resolves that character's
own committed state (`charunlock.LockStates`) and stages/clears
`PendingFlagEdits[idx]` accordingly — a real per-character batch unlock, not
a no-op. Explicit `s.invalidate()` after staging (the grid already rendered
this frame with the pre-toggle badge state). `rowLockedForDisplay` reads
`draftEffectiveRowUnlocked` so the grid's lock badge live-previews an
in-progress gate draft.

**Batch field semantics** (`batchCommon`): if every selected row agrees on a
field's value the editor shows it; if they disagree it seeds blank with a
"Mixed" `hint` placeholder (`labeledEditor` has a `hint` param) — committing
any value applies it to every selected row (`handleFormEvents` loops
`StageField`/`StageItemLevel` per row). Weapon-level's shared clamp ceiling is
the TIGHTEST max among eligible selected rows (`batchLevelInfo`) so one number
stays valid everywhere; `StageItemLevel` re-clamps per row regardless (UX
nicety, not a correctness dependency). `commitTypedFieldsBeforeApply`
force-commits whatever's currently typed in price/qty/level (applying the same
`clampRange`) when Apply is clicked without pressing Enter first, so unsubmitted
text isn't dropped.

**Change-item flow:** `StartPicking` snapshots `SelectedRowIDs` into
`PickingForRows`, consumed FRONT-to-back one row per catalog click
(`consumeNextPickingRow`); `Picking()` (`len==0`) flips false once the queue
empties (clicking the same item N times reproduces "same item everywhere" one
click at a time). `window.go`'s `dimOverlay` wraps the header, merchant panel,
and save footer — while picking it paints a translucent scrim and registers an
opaque (no-`PassOp`) input area, leaving only the catalog grid live; the
row-edit modal itself is hidden entirely for the same reason (see above), not
dimmed-in-place. The Cancel button + "Pick a replacement for the N selected
items" banner live in the catalog panel's title row (`titleRow`,
`catalog_panel.go`) — it REPLACES the title content rather than adding a
line, so panel height never changes.
`formMultiItemList` must set `Axis: layout.Vertical` (Gio's `widget.List`
defaults to Horizontal — every other list sets it, see `layoutGrid`).

Material-locked rows: explanation only, no form. (Data-quality warnings and
the display-override note used to render here in Debug mode; that mode was
removed 2026-08-03 and both were developer bookkeeping, so neither is shown.)
`framedIcon` (40dp bordered "inventory slot", reuses the grid-cell look) is
deliberately NOT migrated to `IconCell` — `IconCell`'s background isn't theme-
adaptive like `framedIcon`'s.

`EditingRowID`/`PickingForRow` (singular) are gone; `SelectedRowIDs`
(ordered) + `rowSelAnchor` and `PickingForRows` (plural) are used everywhere.
`handleRowClick` mirrors the catalog's `handleCatalogClicks`.

## Pending Edits modal (`pending_edits.go`)

Header-right "Pending (N)" toggle + Save button
(`layoutFooterPendingControls`). The toggle opens a true blocking modal —
full-window scrim (`pendingModalScrim`) + centered panel capped at 640dp wide
/ 70% window height, gold rule under the title matching the row-edit bar.
"Pending (0)" is a non-interactive `disabledButton` (opening an empty modal is
pointless). A `pendingModalTag`/`pendingModalHit` press-listener (`window.go`)
exempts clicks inside the modal (scrim included) from the global outside-click
deselect logic, else clicking Remove or the filter combo would blow away the
underlying catalog/row selection.

Content is grouped by merchant (`groupPendingByMerchant`): each group has a
"Merchant (N changes)" header + its rows, dividers between groups. A merchant
filter `Combo` appears above the list once there's more than one group,
tracked via `s.pendingMerchantFilter` (a plain string re-applied every frame
via `SetValue`, NOT the Combo's own index-based preservation — the merchant
set can shrink/reorder as edits are Removed, which would point the same INDEX
at a different merchant). Each row shows a 28dp `framedIcon` (the swapped-to
item's icon, or `RowEdit.IconPath` captured at stage time in
`staging.go`'s `setRowEntry` for a field-only edit) + a right-aligned Remove
button. `pendingRow` branches: an actual swap (`FromName != ToName`) uses
`pendingSwapHeader` (old-item block and new-item block as equal
`layout.Flexed(1,...)` shares either side of a `→` glyph — plain BMP text, not
an emoji, so every row shares old-column/new-column widths and icons line up
vertically down the list with no hardcoded pixel widths); anything else uses
the single icon+name `pendingSimpleHeader`.

One `material.List` over a flattened `[]layout.Widget` of headers/rows/
dividers/the character-flag section renders the whole scrollable body (kept a
`Rigid`, not `Flexed`, child of the modal panel — sizes to its own content up
to the 70% cap, so a short list renders as a small readable dialog).

**Remove all** per merchant (`layoutMerchantGroupHeader`'s right-aligned
button, retained state `removeAllBtns` keyed by `pendingMerchantKey`, shared
by `groupPendingByMerchant` and `State.RemoveMerchantEdits` so it discards
EXACTLY the rows its header groups, including the "Unknown merchant" bucket).
Drained in `layoutPendingBody`: process clicks, re-read `pendingRowIDs()`,
THEN compute merchants/groups for rendering (order matters — a Remove-all can
prune multiple ids the same frame). The modal auto-closes once
`combinedPendingCount() == 0` (item AND flag edits empty) with no live
`ApplyErr`, checked right after Remove draining.

**Character-flag section** wrapped in its own bordered `groupBox` card
(reads as a separate block). Each character has a "Remove all" button
(`RemovePendingFlagsForChar`, `character_panel.go` —
`delete(s.PendingFlagEdits, charIndex)`; `removeFlagBtn`/`removeFlagBtns`
mirror the per-merchant pattern), drained in `layoutPendingBody` alongside the
others. A per-row **sell-value** line renders here when
`Settings.ShowSellValueChanges` is on (see the Settings section):
`sellValueChangeText` phrases `computeEquipParamTargets`' output -- the SAME
pass the real write uses, so the preview can't disagree with what gets
written -- in player terms, and says explicitly that a sellValue change
applies to the item at every merchant, since it's an item-wide property, not
a row-local one. Returns "" (line omitted) for an unresolvable item.

Save → native Save-As prefilled `<stem>-edited<ext>` → apply worker.
`EditError` renders inline red; success keeps merchant/row selection and re-
reads rows from the written file (Save-As chaining).

## Item-info popup (`item_info_modal.go`)

Right-click a sellable cell on either grid (`rightClickTarget`, window.go —
`pressListener`'s secondary-button counterpart, same pass-through mechanics)
opens a blocking modal (same `widgets.Backdrop` shape as Pending Edits —
title+X, gold rule, scrollable body; same Phase-1 Press-based scrim-close
fix, its own `itemInfoModalHit`/`itemInfoPanelHit` pair) showing icon,
category/subcategory, in-game description, and — for weapons/armor/spells —
damage/scaling/requirements/negation/FP cost from `catalog.ItemDetails`
(`internal/assets/data/item_details.json`, generated alongside `items.json` by
`tools/itemdb_extract` from SaveForge's own already-computed item text/stat
fields, see `docs/ITEM_IDS.md`). The merchant grid resolves a row's
CURRENTLY DISPLAYED item (`rowEffectiveItemID` — a staged swap's target
wins over the row's on-disk `ItemID`, via the newly-exported
`Catalog.ResolveItemID`), matching the grid's own icon precedence, so
right-clicking a row with a pending swap shows the new item, not the one
being replaced.

`State.itemByID` (state.go) resolves the popup's subject via
`Catalog.ItemByID`, which includes HIDDEN items. It used to index
`ListItems` instead, which omits them — so right-clicking a hidden-but-
really-sold item (Twin Maiden Husks' Flask of Wondrous Physick) resolved
nil and `layoutItemInfoPanel` silently closed itself, opening nothing at
all (user-reported 2026-08-03). Hidden means "not offered in the browsable
catalog grid", never "unresolvable"; guarded by
`TestItemByIDIncludesHidden`.

## Other panels / widgets

- `save_switcher.go` — header Open button, loaded filename (centered),
  busy text, inline load errors.
- `errors.go` + `widgets/modal.go` — unexpected errors append context +
  stack to `<os temp>/er_merchant_editor/editor.log` and raise a blocking
  modal; `cmd/editor` has a last-resort panic logger (no console exists in a
  `-H windowsgui` build). `Modal` supports both `Layout`/`OKClicked` (OK-only
  alert) and `LayoutConfirm`/`CancelClicked`/`ConfirmClicked` (Yes/No).
- `widgets/` — `Combo` (deferred-overlay dropdown; Gio has no native combo.
  `labels` is an optional display-text array parallel to `options` so a value
  like `""` shows as "All Categories" without changing what filtering code
  reads back). `IconCell` (bordered/disabled/tooltip icon button). `Backdrop`/
  `BackdropStyle` (shared scrim/panel mechanics used by `Modal` and the Pending
  overlay). Gio gotcha: `Combo`'s per-row hover/selection fill must NOT use
  `layout.Stack{Expanded(fill), Stacked(row)}` — Gio's `Stack.Layout` zeroes
  `Constraints.Min` before laying out `Stacked` children
  (`gioui.org/layout/stack.go`), so the row's forced full-width `Min.X` never
  survives and the fill shrinks to the label's text width; use the macro-
  record/fill/replay pattern (`layoutHeader`, same file) instead. `Combo`'s
  scrim click-drain loops (like `Modal`'s) to swallow every queued click.
- `icons.go` — lazy icon cache: PNG from the root `assets` embed, downscale
  256→128 RGBA, cached `paint.ImageOp` (UI goroutine only).

## Settings view (`internal/ui/gio/settings.go`)

Header "Settings" tab. Controls grouped with a `settingsGroupDivider` (14dp +
`horizontalDivider`) between clusters: Theme/Font (+ Reset to Vanilla), the
auto-item-swap checkboxes (+ open-editor-after-drop), the display/grid group
(row counts + sell-value changes + both cell-size sliders), then the
ban-risk opt-in last (least-touched). Settings
persist best-effort to `<os user config dir>/er_merchant_editor/config.json` —
never next to the portable exe.

**Theme** (Dark/Light/Elden Ring): `applyEditorPalette`/`widgets.SetPalette`
take a `theme string` 3-way switch and rebuild the material theme at runtime.
`colorContrastFg` is the text color on accent-colored buttons (the Elden
accent is light gold where hardcoded white would be low-contrast).

Both non-dark palettes were retuned 2026-08-03 against user reference shots:
**Light** moved off its blue-grey defaults (#F0F0F2/#C6C6CC read as
"unstyled", and the divider vanished against the panel) onto warm paper tones
with a visibly darker border and darker icon-cell backs, so the grid still
reads as slots. **Elden Ring** was matched to the game's own inventory screen
-- near-BLACK warm ground (#0E0C0A) rather than the previous milky brown
(#171310), panels barely lifted off the window, a dim bronze divider instead
of light tan, desaturated menu gold for the accent, and the game's own
unaffordable-price red for errors. `widgets/theme.go`'s parallel palette
(combo/icon-cell/tooltip/modal) was retuned to match in the same pass -- the
two MUST move together or the grids stop matching their panels.

**Font** (`Settings.Font`, combo next to Theme): "Lora" (default, cleaner,
sturdier serif) or "Cinzel" (inscriptional dark-fantasy titling face matching
the game's own UI font character; its lowercase glyphs are cap-height by
design). Both are Google Fonts (SIL OFL, vendored in
`internal/ui/gio/assets/fonts/` with `*-OFL.txt`), embedded via `fonts.go`'s
`customFontCollection()` into a `text.NewShaper(text.WithCollection(...))`
built once (`applyTheme`, "don't discard the shaper's cache" rule) — additive
to Gio's own system-font fallback, so non-Latin glyphs are unaffected. `th.Face`
(read by every `material.*` constructor) is what switches the typeface;
`applyFont()` swaps it in place, no shaper rebuild.

Gio gotcha (matters if anyone adds a third font): `gioui.org/text.Shaper`
resolves an EMPTY `Font.Typeface` query to the FIRST face passed to
`text.WithCollection`, NOT to real system-font search — so loading a custom
collection silently hijacks every default-styled label. `typefaceFor` never
returns `""`: every input (including unrecognized/legacy values) resolves to
`"Cinzel"` or `"Lora"`, structurally ruling out the hijack (guarded by
`fonts_test.go`).

**Display toggles** (replaced the old catch-all Debug mode, removed
2026-08-03 on user request -- it had bundled five unrelated behaviors, split
here by what each was actually for):
- **`ShowRiskyItems`** (default OFF, last group): `items.json`'s `risky` flag
  (SaveForge's `cut_content`/`ban_risk`, 42 items -- see `ITEM_IDS.md`) is
  filtered out of `filteredItems()` unless this is on, so cut-content/ban-risk
  items can't be dragged/picked at all. Kept as its own deliberate opt-in
  rather than riding on an unrelated setting, precisely because it gates
  ban-risk content. `itemFilterKey` carries it so the memoized filter re-runs
  on toggle; picking mode shares `filteredItems()`/`layoutItemGrid`, so it's
  covered. Risky items always carry the red border + "[!] cut content /
  online-ban risk" tooltip once visible.
- **`ShowSellValueChanges`** (default off): the Pending Edits sell-value line,
  see that section. A normal player-facing toggle -- verifying a price edit
  really lands is an everyday need, not developer bookkeeping.
- **`OpenEditorAfterDrop`** (default off): dropping catalog item(s) onto stock
  cell(s) selects exactly the rows filled and opens the edit modal on them
  (`staging.go`'s `openEditorForDropped`, called from `applyDrop`), saving a
  separate "Edit" click on an item that usually still needs a price. It calls
  `ensureDraft` before returning so the modal's FIRST frame already shows the
  staged swap. Deliberately NOT wired to `applyRowSwapPayload`: a stock-to-
  stock swap carries each row's existing price/quantity with it, so there's
  nothing new to fill in.
- **Dropped entirely** with Debug mode: raw row ids in titles/pending entries
  (the merchant name, `RowEdit.Merchant`, is the meaningful identifier and is
  now always used), event-flag numbers in the Characters view
  (`flagGroupLabel` lost its `debug` param), `hazardWarnings`/display-override
  notes, and the flag id in the merchant tooltip. All developer bookkeeping
  with no player-facing meaning.
- **Merchant row-count suffix** (`Settings.ShowMerchantRowCounts`, default
  off): the Shop Editor's merchant combo shows just the name unless this is
  on, then `"<name> (<N>)"`. Toggling relabels the combo in place without
  resetting the selection (`merchant_panel.go`'s `syncMerchants`/
  `merchantLabel`).

Old configs still carrying `"debug_mode": true` load fine (unknown keys are
ignored) and do NOT re-enable any of the above -- guarded by
`TestOldDebugModeConfigStillLoads`.

**Auto-free/auto-unlimited item swaps** (`Settings.AutoFreeItems`/
`AutoUnlimitedItems`, both off by default): when on, `staging.go`'s
`stageItemSwapCore` (shared by picking and drag-and-drop) stages price 0 and/or
quantity -1 alongside every fresh item swap via `applyAutoItemDefaults`/
`stageFieldInto` (the latter factored out of `StageField` so both share the
from==to unstage rule). Only fires on an actual item swap, not `StageItemLevel`
or a manual field edit; the user can retype either value afterward (overwrites
the auto-staged one same as any `StageField` call).

**Reset to Vanilla** (`s.resetVanillaBtn`, in the Theme/Font row, styled like
`pending_edits.go`'s "Pending (N)" badge — `colorInputBg` background +
`colorError` text): reverts every `ShopLineupParam` row's item/price/quantity/
display-override fields to their original FromSoftware values, undoing drift
from ANY number of past sessions on the loaded save. Clicking opens a Yes/No
confirm (`widgets.Modal.LayoutConfirm`) stating the row count
(`s.resetVanillaDiffCount` via `Catalog.VanillaDiffs()`); confirming calls
`staging.go`'s `ResetToVanilla()`. Disabled (`disabledButton`) when no save is
loaded OR when `Catalog.VanillaDiffs()` is empty — `state.go`'s
`resetVanillaAvailable`, refreshed by `refreshResetVanillaAvailability` from
`consumeReset` after a fresh load and after a completed save/apply (the only
two points the on-disk-backed row values change).

Deliberately **stages rather than writes**: `ResetToVanilla` replaces the
entire `PendingEdits` batch (prior staged-but-unsaved edits superseded, not
merged) via the same `RowEdit`/`FieldChanges`/`ItemChange` staging model and
Save pipeline every other edit uses — no new write code. The user reviews the
(up to ~1,277-row) diff in the Pending Edits modal, then clicks Save.

The one real design subtlety (`staging.go`'s `rowEditFromVanillaDiff`):
whenever a row's item identity (`equipId`/`equipType`) differs from vanilla it
MUST be staged as a real `ItemChange`, NOT raw `FieldChanges` —
`equipParamRefForEdit` (the sellValue-recompute pipeline's touched-item
resolver) only reads the target item ref from `ItemChange`, falling back to
the row's *current* pre-reset item otherwise; staging identity via plain
fields would silently attribute the reset row's new price to the wrong item's
sellValue bookkeeping (guarded by `staging_test.go`).
One deliberate divergence from a normal swap: the generated
`ItemChange.ClearOverrides` stays `nil` — vanilla data sometimes pins a real
non-`-1` display override (e.g. Kale's Physick Note), so the 4 override fields
are restored via ordinary `FieldChanges` to their exact vanilla value instead
of being force-reset to `-1`.

Character-unlock flags already written to disk in a past session CANNOT be
reverted (no baseline tracked); only flag edits staged this session but not yet
saved are discarded (`PendingFlagEdits`/`PendingBellBearingEdits` cleared) —
the confirm dialog says so. See `docs/SHOP_LINEUP.md`'s
"`internal/assets/data/vanilla_shop_lineup.json`" entry for the baseline dataset;
`internal/catalog/vanilla_test.go` (against `BetterPSN.dat`) is the end-to-end
write proof.

## Characters view (`internal/ui/gio/character_panel.go` + `character_panel_tmh.go`)

The app's landing view (`viewCharacters`, `NewState` default), a per-character
merchant-unlock UI over `internal/character`. Full design writeup + history in
`docs/CHAR_UNLOCK.md`. Two bands:

- **Top** (`layoutOpenBar`): a typed-path `Editor` capped at 2/5 of the bar's
  width (`pathEditorWidthNum`/`Den` — it must NOT be the bar's only `Flexed`
  child or it eats all remaining space) + Load (no-op if empty) + Browse...
  (native dialog) + a `Flexed(1)` message area (busy / inline red error;
  `layoutOpenMessage`, `save_switcher.go`) + a `verticalDivider` + a fixed-
  width filename slot (`layoutFilenameSlot`, always visible independent of any
  error). A missing typed path shows a plain "File not found: `<path>`"
  (`friendlyLoadError` in `state.go`), not Go's raw `*fs.PathError`.
- **Middle** (`layoutCharactersColumns`): 3-column drill-down — Characters
  (`charColWidth` 220dp) → Merchants (`merchantColWidth` 340dp) → flexed Flags
  column, one `material.CheckBox` per **flag group** (`groupFlagRows`: rows
  sharing one `UnlockFlag` — e.g. a Twin Maiden Husks bell-bearing purchase —
  collapse to a single checkbox), checked = unlocked, toggle either direction.
  A toggle **stages** (`stageFlag` → `PendingFlagEdits`, applied to every row
  in the group) rather than writing immediately, mirroring the item-edit
  `PendingEdits` model. Rows with an unsaved staged toggle render amber.

**One shared Save footer, identical on every view, one Save button.**
`window.go`'s `Layout` hoists the footer (`layoutFooterPendingControls`,
`pending_edits.go`) ABOVE the Characters/Shop Editor/Settings switch, so it
renders at the same position/size regardless of view (Save on the left,
"Pending (N)" anchored right). The one Save button (`state.go`'s
`startCombinedSave`/`combinedApplyWorker`) commits whatever's staged, of either
kind, through one Save-As dialog: item edits alone via `Catalog.ApplyEdits`;
flag edits alone via `charunlock.ApplyBatchToFile` then a `Catalog.LoadSave`;
**both at once** write flag edits to a `<outPath>.tmp` file first, reload the
Catalog from it, then run `ApplyEdits` on top into the real `outPath` — never
touches the real input path, one physical output file either way. Because the
footer's count is visible from any view, `consumeReset` clears `PendingFlagEdits`
immediately on any successful save/reload.

The header (`window.go`'s `layoutHeader`) is a left-aligned app-name label plus
a persistent 3-tab switcher (Characters / Shop Editor / Settings, top-right).

**Shop Editor grid bound to the selected character** (`merchant_panel.go`'s
`rowLockedForDisplay`): the lock badge/tooltip reflects that character's actual
unlock state (on-disk or staged), not just "does this row have a gate." With no
character selected, **always** locked ("no character" must mean nothing is
known unlocked). Backed by `character_panel.go`'s `charFlagState`/
`charFlagMerchant` (char-wide rowID → {unlocked, merchant}, a byproduct of the
`ensureMerchantGated` loop) and `effectiveRowUnlocked` (prefers a staged edit
over the committed value). `enrich.go`'s `rowWarnings` attaches a "gated behind
event flag" warning to any gated row regardless of character; its prefix is in
`nonHazardWarningPrefixes` (`merchant_panel.go`) so an *unlocked* gated item
doesn't red-square.

Layout gotchas (`window.go`): two full-width single-line bars need `barSurface`,
NOT `panelSurface` — `panelSurface` forces `Min = Max`, correct only inside a
`Flexed(1)` main-content region; using it for a `Rigid` bar ballooned the Open
button to fill nearly the whole window. `barSurface` reads back its content's
natural dimensions. An empty `layout.Flexed` spacer must return
`layout.Dimensions{Size: gtx.Constraints.Min}` (the shared `flexSpacer` helper)
— a bare `layout.Dimensions{}` ignores the `Min=Max=itsShare` constraint Flex
forces on a Flexed child and breaks full-width spanning / right-alignment.
`verticalDivider` trusts `gtx.Constraints.Max.Y` completely, and a `Rigid` child
of a `Vertical` Flex has effectively unbounded `Max.Y`, so using it in a single-
line bar needs an explicit height clamp first (`dividerBarHeight`). All three are
the same lesson (Gio widgets that trust ambient `Constraints` rather than their
own content need the caller to bound those constraints first) and are guarded by
dimension-assertion tests, not just panic-checking.

State machine (`ensureCharList`/`ensureMerchantGated`/`selectFlagMerchant`/
`stageFlag`/`effectiveRowUnlocked`/`displayMerchantUnlocked`/
`combinedPendingCount`/etc.) and headless render/dimension tests live in
`character_panel_test.go`/`merchant_panel_test.go`/`state_test.go`, including an
end-to-end combined-save write-path test (writes a real file and reloads it).
Note `layoutFlagRow`
re-derives each checkbox's `.Value` from `FlagState`/`PendingFlagEdits` on every
render (SSOT) rather than trusting a seed value, so a staged toggle from outside
that merchant's column (e.g. the row-edit bar's Unlock button) doesn't leave a
stale box. GUI covered by extensive real-window use (see CHAR_UNLOCK.md's "GUI
status").

## Known gaps (deferred by user request, not started)

- Responsiveness pass done (async icon decode, filter memoization, redraw
  coalescing, immediate invalidate on selection); WSLg renders on CPU — judge
  performance on the native Windows exe.
- Some subcategory names low-quality, inherited from SaveForge (cosmetic).
- 6 near-identical icon pairs left uncertain, no distinct asset exists either
  way (see `ITEM_IDS.md`); the 2 confirmed-wrong (Golden Beast Crest Shield,
  Hefty Cracked Pot) were fixed.
- mtrlId-gated rows deliberately not editable and hidden entirely (among
  browsable merchants only Enia has them, 51/101, all Remembrance hand-ins tied
  to quest/boss-reward logic — editing the material side would mean writing
  EquipMtrlSetParam, a new corruption surface). Filtered out in
  `fetchMerchantRows`; the merchant combo count uses `EditableRowCount`.
  Defense-in-depth kept: drops skip locked rows, popup explains,
  `catalog.ApplyEdits` still rejects them.
- costType 2 confirmed = Starlight Shards (labeled); only 3/4 remain generic,
  both occurring solely in non-browsable blocks (see `SHOP_LINEUP.md`).
- `widgets/modal.go` (the unexpected-error dialog) never restyled; not
  prioritized (user review found nothing wrong with the GUI).

## Codebase cleanup pass (2026-08-01)

Full-codebase audit (5 parallel read-only reviews across every `app/` package)
+ phased execution plan. Phase 1 bug fixes (committed):
- **Save-As dialog data-loss race**: `startCombinedSave` sets `busy` for the
  whole flow (click through completion/cancel/error), not just once the native
  dialog returns a path — previously, edits staged while the dialog sat open
  were silently discarded on write since `Busy()`-gated staging controls stayed
  interactive. New `State.clearBusy()` resets it on every early-abort path.
- `shopwrite.ApplyWithSchema` bounds-checks an edit's `RowID` before casting to
  `int32` instead of silently truncating (CLI/direct-caller path only).
- `widgets.Combo`'s scrim click-drain now loops like `widgets.Modal`'s.
- `catalog.WarnPrefixNameIconOverride`/`WarnPrefixMaterialExchange`/
  `WarnPrefixEventGated` are the single source of truth for the warning-text
  prefixes `merchant_panel.go`'s `nonHazardWarningPrefixes` matches — previously
  two independently hand-copied literals (the drift class behind an earlier
  cellBorder regression).

**Phase 2/3**: Phase 2 removed dead code (`catalog.items()`/`itemsByID()`,
`state.OpenButton()`/`OpenDialogOpen()`, `charunlock.ApplyToFile()`,
`row_edit_form.go`'s `coinIcon` placeholder — replaced by the real per-cost-type
`batchPriceIcon`). Phase 3 deduplicated near-identical blocks across `state.go`
(selection helpers, `consumeReset`'s common tail — also fixed a missing-
`invalidate()` inconsistency), `window.go` (dark palette no longer hardcoded
twice), `settings.go` (cell-size slider handling), `character_panel.go`
(`flagGroupCheckbox`), `pending_edits.go`/`row_edit_form.go` (disabled-button
chrome, editor seeding), plus a new `widgets.Backdrop`/`BackdropStyle` shared by
`widgets.Modal` and the Pending Edits overlay.

**Phase 4**: pure code-motion file splits, no logic change, each verified by
diffing the complete top-level declaration set before/after.
`internal/savefile/pipeline.go` → `pipeline_schema.go`/`pipeline_crypto.go`/
`pipeline_bnd4.go`/`pipeline_param.go`. `internal/savefile/apply.go`'s
`applyWithSchema` (the function behind both the 2026-07-27 corruption and the
2026-08-02 crash) split into named steps (`patchRows`/`verifyRecompressed`/
`buildRegBlob`/`encryptAndSplice`) with zero reordering; a real write through
the refactored path was user-confirmed in-game. `character_panel.go`
(1255→930 lines) had its Twin Maiden Husks-only block split to
`character_panel_tmh.go`. `row_edit_form.go` (1129→689 lines) split three ways:
logic stays, `row_edit_layout.go` gets the bar's rendering, `bar_chrome.go` gets
primitives shared with other panels. `window.go`'s `Layout` had two blocks
extracted into named methods (`layoutOverlays`/`handleStrictDeselect`), called
at their exact original positions (NOT reordered — a confirm click inside
`layoutOverlays` synchronously stages edits the footer reads the same frame).

**Phase 5**: comment-only trim across 13 files (internal/catalog/{catalog,items,
sellvalue}.go, internal/ui/gio/{character_panel,character_panel_tmh,pending_edits,
row_edit_form,row_edit_layout,settings,state}.go, internal/ui/gio/components/{iconcell,
lock_badge}.go, internal/savefile/recompress.go) — deleted dated "Round N" changelog
narration, kept every genuine non-obvious WHY. See git log for finer-grained
history.
