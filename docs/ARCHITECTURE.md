# Architecture

ER Merchant Editor is split into platform-independent core packages and a
thin desktop shell. Dependencies point inward: operating-system and Gio code
may use the application core, but the core must never import Gio, native
dialogs, or platform packaging code.

## Package boundaries

```text
cmd
  -> internal/ui/gio, or directly to the core for CLI commands

internal/ui/gio
  -> internal/application (session state and save orchestration)
  -> internal/catalog, internal/character (domain/read models)
  -> internal/platform (desktop services)

internal/application
  -> internal/catalog
  -> internal/character
  -> internal/savefile

internal/catalog   -> internal/savefile, internal/assets/data
internal/character -> internal/character/flags, internal/character/slot
internal/savefile  -> internal/assets/data
```

- `internal/savefile` owns the Elden Ring container format: crypto, DCX/zstd,
  BND4, PARAM schemas, decoding, mutation, and verification.
- `internal/catalog` owns items, merchants, shop rows, metadata enrichment,
  reinforcement rules, and vanilla comparisons.
- `internal/character` owns character slots, identities, event flags, unlocks,
  and bell bearings.
- `internal/application` owns an editing session: loading, staged changes,
  mutation translation, and save orchestration. It contains no GUI widgets.
- `internal/ui/gio` owns window layout and retained widget/view state only.
  It may consume core domain/read-model types for presentation, but does not
  own save-format mutation or write orchestration.
- `internal/platform` owns replaceable operating-system services such as file
  dialogs and application paths.
- `internal/assets` contains embedded runtime data and item icons. Fonts and
  view artwork live beside the Gio shell. Command-line tools import data
  assets only, never the icon package.
- `cmd` contains composition roots. A command may wire packages together but
  must not contain business rules.
- `packaging` and `scripts` contain target-specific release metadata and build
  orchestration; build output belongs in ignored `dist/`.

## Rules

1. Core packages accept data or interfaces, not Gio widgets or native paths
   selected by a dialog.
2. Interfaces live with the consumer that needs them. Platform packages
   implement those interfaces.
3. A loaded save is represented by one application session. UI state may
   select or render session data but may not become the source of truth for
   pending edits.
4. The application layer owns save orchestration and translates staged editor
   operations into low-level save mutations; the UI does not write save
   containers itself.
5. Platform builds are native CI jobs. Windows resources, Linux desktop
   metadata and future macOS bundle metadata remain outside Go application
   logic.
6. Generated binaries, virtual environments, user saves, test copies, and
   release artifacts are never tracked.

## Change policy

Preserve observable behavior, keep imports flowing inward, and test the
smallest affected core package while iterating. Run `scripts/check.sh` before
handoff; use native packaging jobs for current Windows and Linux release
verification. macOS verification is deferred until it becomes a release
target.
