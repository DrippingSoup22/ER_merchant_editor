# Gio desktop editor

Run from the repository root:

```sh
go run ./cmd/ermerchanteditor
```

Linux development requires the native Gio packages listed in
[PACKAGING.md](PACKAGING.md). The app starts without a save and never writes
the loaded input path.

## Ownership

`internal/ui/gio` owns rendering, widget state, interaction, and presentation
only. `internal/application.Session` owns the catalog and all staged edits;
`SavePlan` snapshots and applies them. Save-format writes belong to
`internal/savefile` and `internal/character`.

The UI may consume catalog/character read models, but new mutation or save
sequencing logic belongs in the application layer.

## Views

- **Characters**: character selection and merchant-release/bell-bearing flag
  staging. This is the initial view.
- **Shop Editor**: searchable/filterable item catalog and one merchant's fixed
  row grid, with item swap, price, quantity, `+N`, drag-and-drop, and item info.
- **Settings**: theme/font and editing defaults, cell sizing, risky-item
  visibility, and Reset to Vanilla.

One shared footer shows action results, contextual guidance, and the live staged-change count.
Pending lists/removes edits; Save File writes them together through Save As.

## Staging invariants

- Edits are overlays on the loaded save, never immediate writes.
- Returning a field or flag to its on-disk value removes that pending edit.
- Item swaps preserve release/stock flags, clear row-specific name/icon
  overrides, validate the target category, and stage any required
  `EquipParam.sellValue` reduction.
- Material-priced rows are visible for context but cannot be edited.
- A prepared save plan is immutable, so UI changes made while a native dialog
  is open cannot silently enter the write.
- A successful save reloads the output and clears all pending maps; a failure
  keeps the loaded input and pending state.

Reset to Vanilla diffs shop-row fields against the embedded baseline. It does
not restore character flags already written in an earlier session because no
per-character vanilla baseline exists.

## Character-aware gates

The selected character determines lock badges and gate actions. With no
character selected, gated rows are shown as locked; an explicit gate action
may target all characters. Enia is always disabled because her release flags
overlap boss progression. The character-independent “gated behind event flag”
warning must stay classified as informational so an already-unlocked row does
not become a hazard.

## Concurrency

Gio widget and view state belongs to the frame goroutine. File loading,
decoding, saving, and native dialogs run outside it. Workers return immutable
results; the frame loop consumes them, updates state, and invalidates the
window. Do not mutate maps, widget state, or the loaded catalog concurrently.

## Layout invariants

- `panelSurface` is for bounded flex regions; use `barSurface` for naturally
  sized rigid bars.
- Flex spacers must return their assigned minimum dimensions.
- Dividers inside unbounded rigid bars need an explicit height clamp.
- The Twin Maiden Husks sections share one scrollable list; do not place a
  trailing rigid section after that list.
- Drag payloads are IDs, not pointers to transient grid cells. Multi-item drops
  fill consecutive editable rows and preview the exact targets.

Dimension, staging, modal, and interaction tests guard these rules. Real-window
checks are still required for new layouts because unit tests cannot validate
native dialogs, font rendering, or platform window behavior.

Every dimmed overlay follows one input rule: pointer-down outside its bright
panel cancels the active flow; presses inside the bright area do not dismiss it.
