# EldenRing-SaveForge attribution

External source: `EldenRing-SaveForge` (GPLv3), used read-only during
development. This repository adapts:

- PS4 item data and icons through `tools/itemdb_extract`;
- character-slot identity offsets;
- event-flag ID-to-byte/bit mapping and its exception/BST data;
- bell-bearing flag/name data, originally credited by SaveForge to
  `er-save-manager`.

These parts support `internal/assets` and `internal/character`. The project was
relicensed from MIT to GPLv3 before incorporating them; see `LICENSE`.

SaveForge represents merchant availability as per-character event flags. Those
flags gate pre-existing `ShopLineupParam.eventFlag_forRelease` rows; they do
not replace the shop table. Full integration and safety rules are documented
in [CHAR_UNLOCK.md](CHAR_UNLOCK.md).

SaveForge does not edit `ShopLineupParam`; the merchant schema, canonical
merchant mapping, write-back implementation, and Gio UI were developed in this
repository. This file records attribution without treating the external tool
as runtime architecture.

SaveForge marks Spectral Steed Attire goods `no_database` because its dedicated
World view owns both inventory visibility and active appearance flags. That is
a routing/read-only marker, not a ban-risk classification. This merchant editor
offers the three legitimate ownership items and leaves appearance selection to
the game's hub menu. An in-game 1.17 test confirmed all three purchase and
activate correctly. Duplicate shop delivery may automatically overflow one copy
into repository storage despite manual deposit being disabled; that behavior is
game-side and produced no observed fault.
