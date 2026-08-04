# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Elden Ring save-file editor targeting merchant shop contents, on **PS**
(PS4/PS5). Goal: a free editor to change any merchant's existing
`ShopLineupParam` rows (item/price/quantity/unlock gating). Not "every
item on one merchant" — confirmed architecturally impossible, see
`docs/PROJECT.md`'s "Row-count ceiling".

Read `docs/PROJECT.md` first (current status/open problems), then as
needed: `docs/MERCHANT_DATA.md` (USERDATA11 pipeline + known constants),
`docs/ITEM_IDS.md` (item ID<->name mapping), `docs/SHOP_LINEUP.md`
(ShopLineupParam row schema + row-to-merchant mapping), `docs/MERCHANTS.md`
(canonical merchant identity — read this before trusting any merchant
name), `docs/WRITEBACK.md` (the write-back engine), `docs/EDITOR.md` (the
Gio editor UI), `docs/PACKAGING.md` (single-binary build + CI),
`docs/CHAR_UNLOCK.md` (per-character merchant-unlock feature, code
complete but not yet visually GUI-tested — separate `.sl2` character-slot
region, not `regulation.bin`), `docs/SAVEFORGE_REFERENCE.md`
and `docs/ER_PVP_MOD_REFERENCE.md` (prior-art, now mostly superseded by
our own code — kept only for non-obvious gotchas/attribution). All kept
short on purpose — read before re-deriving context, don't skip because it
"looks solved already."

Confirmed: `USERDATA11` = AES-256-CBC + DCX/zstd encrypted/compressed
embedded `regulation.bin`, itself a BND4 archive of named param tables.
Pipeline and byte offsets verified against our own fixture save. On PS4
there's no real re-compression on write — edits must reuse existing
row/block structure, not grow it (see `docs/MERCHANT_DATA.md`).

## Environment

Ubuntu on WSL. Go: apt go1.22 + `GOTOOLCHAIN=auto` (fetches the go.mod-pinned
toolchain once, cached after). Python 3.12 for dev-side `tools/` only.
Linux GUI dev builds need cgo + X11/EGL packages (see `docs/PACKAGING.md`);
the shipped Windows target is CGO-free.

## Working conventions (must follow)

- **Backups**: `save_files/` holds the user's originals — treat as read-only,
  never edit in place. For any in-game test, copy the file into
  `working_copies/` (gitignored) first and edit the copy. Do not create
  additional backup copies of `save_files/` itself — the user already
  maintains those.
- **Tests are expensive**: each in-game test load is slow, so batch edits and
  maximize information gained per test rather than testing one small change
  at a time. Aggressive/unsafe edits are fine — testing is offline against a
  disposable copy with the original always intact.
- **Subagents**: for substantial *implementation* chunks, dispatch Opus
  subagents and always double-check their output before relying on it.
  **Never skimp on `app/shopwrite`** — it's the corruption-risk half; being
  frugal there is pound-foolish. For *research/navigation*, follow the
  token-tiered agent policy below.
- **Docs**: keep everything in `docs/` concise. These files get re-read at the
  start of future sessions — bloat costs tokens every time, so log findings
  tersely (offsets, byte values, terse conclusions) rather than narratively.
  Read a `docs/*.md` only with a specific reason, not "to be safe."
- **Token economy (must follow)** — the real cost is bulk content entering
  the main context, so keep it out:
  - **Never `Read` a `data/*.json` or `data/icons/*` file wholesale** —
    they're large generated artifacts (`item_details.json` ~811KB,
    `items.json` ~481KB, etc.). Extract with `grep`/`jq` or a bounded
    `Read` (`offset`/`limit`). Same for any file over ~1000 lines.
  - **Tiered build/check** (don't full-build per change — `app/build.sh` is
    ~24s + a 90MB exe, and CI rebuilds on push): iterate with
    `app/check.sh ./app/<pkg>/` (vet + scoped test, no artifacts) or a bare
    `go build ./app/<pkg>/` (sub-second compile-check). Run `app/build.sh`
    **only** to produce the shippable exe — i.e. when handing off an editor
    change for the user to click-test. Scope test output to the touched
    package; surface failures, not full pass logs.
  - **Agent policy (token-tiered):** trivial lookup (a symbol / one fact) →
    do it inline (Grep/Read), no agent. Broad *local* code search → the
    Haiku `info-gatherer` agent (or built-in `Explore`); conclusion only
    (`file_path:line` + terse answer), no dumps. *Online* research → the
    Haiku `web-researcher` agent; distilled facts + source URLs, never raw
    pages — but re-verify correctness-critical claims (ban-risk, event-flag
    collisions, FromSoft container-format facts) against the primary source
    yourself, since Haiku can lose nuance.

## Layout

- `README.md` — user-facing setup/run instructions (public-release facing,
  kept accurate for someone who isn't me).
- `LICENSE` — GPLv3 (relicensed from MIT 2026-07-28 to permit adapting
  event-flag data/logic from GPLv3 EldenRing-SaveForge; see
  `docs/PROJECT.md` for attribution).
- `save_files/` — user-provided original saves (read-only, gitignored).
  Two fixtures: `vanilla_fresh_character.dat` (clean baseline) and
  `BetterPSN.dat` (third-party-edited, cross-check only — see `docs/PROJECT.md`).
- `working_copies/` — disposable copies used for actual test edits (gitignored).
- `docs/` — living status + research notes (see above).
- `data/` — generated runtime lookups, all embedded via `data/embed.go`
  (10 JSONs: `items`, `item_details`, `item_sort_order`, `shop_lineup_schema`,
  `shop_row_names`, `equip_mtrl_set_schema`, `merchant_catalog`,
  `weapon_reinforce`, `weapon_reinforce_rates`, `vanilla_shop_lineup`). Exact
  shapes/provenance live in
  the docs (`ITEM_IDS.md`, `SHOP_LINEUP.md`, `MERCHANTS.md`). Guardrails: use
  `merchant_catalog.json` for merchant identity, **not** the raw
  `shop_row_names.json`; `vanilla_shop_lineup.json` is the "Reset to Vanilla"
  baseline. Large generated files — never `Read` them whole (Token economy).
- `data/icons/` — vendored SaveForge item-icon PNGs (**committed**,
  ~85MB/2752 files; go:embed needs them present for CI/fresh clones),
  embedded via root-level `icons.go` (package `assets`; root because embed
  patterns can't contain `..`). **Never import `assets` outside the GUI** —
  embed vars are not dead-code-eliminated.
- `tools/savescan.py` — Python, read-only save-file inspection (dev-only;
  no longer a runtime dependency of anything shipped): BND4 list/extract,
  `rows` (decodes ShopLineupParam, one JSON object/line), `merchants`
  (grouped/sorted, mtrlId resolved, warnings inline), `find-item <name>`.
  Run via `tools/.venv/bin/python tools/savescan.py ...` (venv:
  `pip install -r tools/requirements.txt`, gitignored). Kept deliberately
  as the **independent oracle** for `app/catalog`'s golden test — do not
  port it to Go or delete it; regenerate the golden with
  `tools/.venv/bin/python tools/savescan.py rows save_files/vanilla_fresh_character.dat > working_copies/rows.golden.jsonl`.
- `tools/itemdb_extract/` — Go, regenerates `data/items.json` from
  SaveForge's item DB (see `docs/ITEM_IDS.md` for the exact command).
- `tools/paramdex_extract/` — Python (stdlib only), regenerates
  `data/shop_lineup_schema.json` + `data/shop_row_names.json` +
  `data/equip_mtrl_set_schema.json` from `soulsmods/Paramdex` (see
  `docs/SHOP_LINEUP.md`).
- `tools/merchant_catalog/` — Python (stdlib only, no network), regenerates
  `data/merchant_catalog.json` from `data/shop_row_names.json` + this repo's
  own research rules (see `docs/MERCHANTS.md`).
- `tools/paramdex_schema.py` — Python (stdlib only), the shared PARAMDEF
  schema builder (`build_schema`/`fetch`) used by `paramdex_extract` and
  `weapon_reinforce_extract`. Bitfield-grouping gotcha: a differently-typed
  `dummy8`-vs-`u8` consecutive bitfield pair sharing one real on-disk byte
  must group together, not split into two (see `docs/ITEM_IDS.md`).
- `tools/weapon_reinforce_extract/` — Python (needs `tools/.venv`, reuses
  `savescan.py`'s decrypt/BND4/param decode directly), regenerates
  `data/weapon_reinforce.json` from Paramdex's `EquipParamWeapon`/
  `ReinforceParamWeapon` schemas + our own fixture save's regulation.bin
  (see `docs/ITEM_IDS.md`).
- `tools/weapon_reinforce_rates_extract/` — Python (same venv/pattern),
  regenerates `data/weapon_reinforce_rates.json`: each weapon's own
  `reinforceTypeId` + that type's per-level attack/scaling/guard multiplier
  curve (`ReinforceParamWeapon` rate columns), so the item-info popup shows a
  weapon's stats at its actual "+N" level. Embedded. See `docs/ITEM_IDS.md`.
- `tools/vanilla_shop_lineup_extract/` — Python (needs `tools/.venv`,
  reuses `savescan.py`'s decrypt/BND4/param decode directly, no network),
  regenerates `data/vanilla_shop_lineup.json` from our fixture save's own
  regulation.bin (see `docs/SHOP_LINEUP.md`).
- `tools/spell_stats_extract/` — Python (needs `tools/.venv`, reuses
  `savescan.py`'s decode + Paramdex `MagicParam` schema), regenerates
  `data/spell_stats.json` (sorcery/incantation FP/slots/INT-FAI-ARC reqs)
  from the fixture's regulation.bin. Read by `itemdb_extract` as the
  authoritative spell source (supersedes SaveForge's DLC-incomplete curated
  table); **not embedded** — a generation input only. Regen it *before*
  `itemdb_extract` (see `docs/ITEM_IDS.md`).
- `tools/consumable_scaling_extract/` — Python (same venv/pattern),
  regenerates `data/consumable_scaling.json` (damage throwables' attribute
  scaling) from the fixture's `EquipParamGoods.refVirtualWepId` -> virtual
  `EquipParamWeapon` chain. Read by `itemdb_extract` (regen *before* it);
  **not embedded**. See `docs/ITEM_IDS.md`.
- `app/shopwrite/` — Go **package `shopwrite`**, the write-back engine
  (`apply.go`, `decode.go`, `recompress.go`, `pipeline_*.go`): edits existing
  `ShopLineupParam` rows in place (decrypt/decompress/patch/**fully
  recompress**/re-encrypt/splice) with a **round-trip self-check before any
  write** (`Apply` never writes without it). Full recompress → value edits
  have no row-count ceiling. **Critical**: the output zstd frame must match
  SoulsFormats' `ZstdHelper.WriteZstd` shape exactly (no checksum, no
  explicit content size, 64KB window) — klauspost's defaults don't, and a
  wrong frame passes every offline check yet crashes the game on load, so
  trust `soulsmods/SoulsFormatsNEXT` over guessing on any FromSoft
  container-format question. Real-hardware confirmed. See `docs/WRITEBACK.md`.
  GUI calls `Apply` in-process; `app/cmd/shopwrite` is the CLI wrapper.
- `app/catalog/` — Go package, the read side: enriched row decode
  (item/merchant/material/warnings), item queries, `ApplyEdits` validation
  (material-locked rows rejected). Golden-tested against
  `tools/savescan.py` — the tests in here are the safety net for any
  decode change; `go test ./...` runs them (fixture tests self-skip when
  `save_files/` is absent).
- `app/charflags/` — resolves an event-flag ID (the IDs
  `eventFlag_forRelease` references) to its byte/bit in a character slot's
  event-flags bitfield and gets/sets it. Algorithm+data (`bst.go`,
  `event_flags_exceptions.go`) ported from EldenRing-SaveForge under
  **GPLv3** — why the project relicensed from MIT; attribution in
  `docs/SAVEFORGE_REFERENCE.md`. See `docs/CHAR_UNLOCK.md`.
- `app/charslot/` — locates a slot's event-flags region (own independent
  byte-anchor) and reads identity/name/level; byte-exact tested vs both
  fixtures' 15 real slots. See `docs/CHAR_UNLOCK.md`.
- `app/charunlock/` — cross-references `catalog.Row.UnlockFlag` against a
  character's flags and writes releases either direction, single or
  mixed-direction batch (`SetReleaseBatch`; `ApplyBatchToFile` writes
  several characters' edits to one file), round-trip self-checked, no
  checksum needed. GUI wiring: `app/editor/character_panel.go`;
  `app/cmd/charunlock` is the CLI wrapper. See `docs/CHAR_UNLOCK.md`.
- `app/editor/` + `app/cmd/editor/` — the Gio GUI (one file per panel;
  custom widgets in `app/editor/widgets/`; native dialogs via
  ncruces/zenity). Three tabs via a header switcher (`window.go`):
  **Characters** (landing, `NewState` default), **Shop Editor**,
  **Settings**. Dev run: `go run ./app/cmd/editor`. Full architecture, view
  details, and round-by-round history in `docs/EDITOR.md` and
  `docs/CHAR_UNLOCK.md` — read those before changing a view. Load-bearing
  gotchas worth keeping in mind even without opening the docs:
  - **One shared Save button** commits staged item + flag edits together
    (`state.go`'s `startCombinedSave`); mixed edits route through an
    `<outPath>.tmp` intermediate and never touch the input path.
  - Shop Editor locked-row display (`rowLockedForDisplay`,
    `merchant_panel.go`) is bound to the Characters-view selection and is
    **always locked when no character is selected** (deselect must clear to
    "locked," not keep the previous character's state).
  - **Sync requirement**: `enrich.go`'s `rowWarnings` adds a
    save-independent "gated behind event flag" warning to every gated row,
    so it must stay in `nonHazardWarningPrefixes` (`merchant_panel.go`) or
    it red-squares *unlocked* gated items.
  - **Gio layout**: widgets that trust ambient `Constraints` must be bounded
    by the caller (`panelSurface`/`barSurface`, `flexSpacer`,
    `dividerBarHeight`, `Combo` row width) — details + assertion tests in
    `docs/EDITOR.md`.
  - Respect the UI-goroutine vs worker concurrency contract in `state.go`.
  - No GUI automation in this dev env — real-window checks rely on the
    user's click-through; current state IS user-verified
    (`docs/CHAR_UNLOCK.md`'s "GUI status"), but re-check for brand-new views.
- `app/winres/` + `app/cmd/editor/rsrc_windows_amd64.syso` — Windows exe
  icon/manifest/version; the .syso is committed, regenerate only after
  editing `app/winres/*` (`go tool go-winres make --in app/winres/winres.json
  --out app/cmd/editor/rsrc --arch amd64`).
- `app/build.sh` — builds everything shipped: 87MB single-file Windows GUI
  exe + 4-target shopwrite CLI into `app/dist/` (gitignored). Pure
  `go build`, works from WSL/CI identically. ~24s — for the shippable exe
  only, not per-edit (see "Tiered build/check").
- `app/check.sh [pkgs...]` — fast iteration-loop check: `go vet` + `go test`
  on the given packages (default all), no artifacts. Scope it to the package
  touched; this is the per-edit command, `build.sh` is not.
- `.github/workflows/release.yml` — CI on ubuntu-latest: vet + test +
  `app/build.sh`; `v*` tag push attaches the exe to a draft GitHub
  Release, `workflow_dispatch` uploads a workflow artifact instead.

## Tooling strategy

Everything shipped is **Go, one binary** (Gio GUI + `app/catalog` reads +
`app/shopwrite` writes, all in-process; go:embed for icons/data). The
engine was *moved* into a package, never rewritten — byte-identity-tested
against the pre-refactor tool, preserving the trust it earned against real
saves; treat `app/shopwrite/` as the corruption-risk half and keep changes
minimal and test-guarded. Python survives only in dev-side `tools/`:
`savescan.py` (independent golden-test oracle — keep it), plus the
`paramdex_extract`/`merchant_catalog` generators; `itemdb_extract` is Go.
One-off generators, not the hot path. No Node/Rust/PyInstaller anywhere.
