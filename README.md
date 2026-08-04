# ER Merchant Editor

A free save-file editor for **Elden Ring on PS4/PS5** that rewrites what
merchants sell. Pick any of the game's 43 merchants, swap the item in a
stock slot for any of 2596 items, set its price, quantity and upgrade
level, and unlock stock that's still gated behind progression — then write
it all out to a new save in one batch.

Single portable `.exe`, no installer, nothing else to download.

**[Download the latest release →](../../releases/latest)**

## What it does

### Shop Editor

The whole catalog on the left, the selected merchant's
stock on the right.

<p align="center">
  <img src="docs/images/shop-editor.png" alt="Shop Editor — item catalog on the left, merchant stock on the right" width="100%">
</p>

- **Swap what a merchant sells.** Click a stock slot and pick a
  replacement, or drag an item straight from the catalog. Drag several at
  once (Ctrl/Shift-select) and they fill consecutive slots, with a live
  preview of exactly which cells will change.
- **Set price and quantity** per slot (quantity `-1` = unlimited), and set
  a weapon or shield's reinforcement `+N`, clamped to that item's real
  maximum (+10 somber, +25 standard).
- **Rearrange stock** by dragging one slot onto another; the item, price
  and quantity move together.
- **Work in the view that fits the job.** *Edit layout* keeps stable save
  slots for predictable drag-and-drop; *Game preview* shows the category and
  item order Elden Ring will use in-game.
- **Right-click any item** for an Elden Ring-style info card: weapon stats
  at that slot's actual `+N`, spell FP cost, slots and requirements,
  scaling, Ash of War skills.
- **Search and filter** by category and sub-category, in the same order
  the game itself uses.

### Characters

Every character slot in the save, and what each one has actually unlocked.

<p align="center">
  <img src="docs/images/characters.png" alt="Characters view: per-character merchant unlock state, with bell-bearing stock listed" width="100%">
</p>

- Tick a merchant's still-gated items to unlock them for that character —
  bell bearings, quest rewards, boss-kill stock, anything progression-gated.
- With no character selected, unlocking applies to **all** characters at
  once.
- Twin Maiden Husks bell bearings and merchants follow their in-game shop
  groups, making it clear which stock each flag unlocks.
- Stock stays locked in the editor until the selected character can
  actually buy it, so you always see that character's real shop.

### Settings

Dark, Light and Elden Ring themes, font choice, defaults for
how swapped items are priced, cell-size controls, visibility of cut-content /
online-ban-risk entries, and **Reset to Vanilla**, which diffs the save
against a built-in vanilla baseline and stages every difference for review.

<p align="center">
  <img src="docs/images/settings.png" alt="Settings — display themes, editing defaults, cell sizes, and the cut-content visibility option" width="100%">
</p>

## What it can't do

It edits **existing** merchant rows. It cannot add new rows to a merchant's
inventory, so "one merchant sells every item in the game" is out of reach —
that's a hard limit of the save format, not a missing feature. See
`docs/PROJECT.md`'s "Row-count ceiling".

## Before you start: your save must already be decrypted

Elden Ring saves on PlayStation sit inside an outer, platform-level
encryption layer (PS4/PS5 signing). That layer is **completely separate**
from anything this tool touches and is out of scope here — you need a save
that's already past it, the same starting point tools like SaveForge
expect. Get there with a third-party PS4/PS5 save decrypt/encrypt utility
first.

**Always work on a copy, never your only save.**

## Using it

1. Run `ERMerchantEditor.exe` — that's the whole install. Windows only.
2. Paste or browse to your decrypted `.dat` save and hit **Load**.
3. Edit. Every change is staged, not applied — **Pending (N)** shows
   exactly what will be written, and anything can be removed before saving.
4. **Save File...** opens a picker for the destination.

Your original file is never modified. Nothing is written at all unless the
fully rebuilt save passes a byte-exact round-trip check first — if it
doesn't verify, you get an error instead of a file.

## Building from source

Optional; skip it if you downloaded a release. Requires Go 1.22+ (the build
pulls the pinned newer toolchain itself via `GOTOOLCHAIN=auto`; the first
build needs network access for the module cache).

```
bash app/build.sh        # Windows exe + shopwrite CLI, from any OS
go run ./app/cmd/editor  # or run the GUI directly
                         # (Linux needs X11/EGL dev packages — see docs/PACKAGING.md)
```

The Windows build is pure Go with no cgo, so it cross-compiles from Linux
or macOS; that's exactly what GitHub Actions does for each release tag.

## Under the hood

The editor decrypts the save, extracts and decompresses the embedded
`regulation.bin`, patches the relevant param tables, fully recompresses,
re-encrypts and splices the result back — all in-process, in one Go binary.
`docs/` is the living source of truth: `PROJECT.md` for status and the
pipeline, `MERCHANT_DATA.md` for the save format, `SHOP_LINEUP.md` and
`MERCHANTS.md` for the param/merchant data, `WRITEBACK.md` for the write
engine, `EDITOR.md` for the GUI.

## License

GPLv3 — see `LICENSE`. Relicensed from MIT on 2026-07-28 to permit adapting
event-flag save-format data and logic from the GPLv3-licensed
[EldenRing-SaveForge](https://github.com/oisis/EldenRing-SaveForge) project;
see `docs/SAVEFORGE_REFERENCE.md` for attribution. Item icons are vendored
from the same project. Param definitions come from
[soulsmods/Paramdex](https://github.com/soulsmods/Paramdex).

Not affiliated with FromSoftware or Bandai Namco.
