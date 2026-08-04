# Per-character merchant unlocks

`ShopLineupParam.eventFlag_forRelease` gates a row for each character. The
actual bit lives in that character's `.sl2` slot, separate from the embedded
`regulation.bin`. The Characters view reads and stages these flags; item edits
never change them.

## Packages

- `internal/character/slot` splits the ten fixed PS4 slots, reads identity,
  and locates the event-flag region.
- `internal/character/flags` maps flag IDs to byte/bit positions.
- `internal/character` relates catalog rows and bell bearings to flags and
  performs verified batch mutations.

Verified PS4 slot constants: header `0x70`, slot size `0x280000`, ten slots.
The event-flags bitfield is `0x1BF99F` bytes. Its start is 1,061 bytes after
the unique TutorialData marker `AE 00 01 00 00 04 00 00`; fixture tests verify
the location across all populated slots.

The flag-ID mapping's exception and BST tables were ported from GPLv3
EldenRing-SaveForge because public data was insufficient to derive them.
Character identity offsets were also adapted from SaveForge. The slot anchor,
mutation checks, and merchant integration were developed here. Attribution is
in [SAVEFORGE_REFERENCE.md](SAVEFORGE_REFERENCE.md).

## Mutation contract

`SetReleaseBatch` accepts independent target values, so one batch can unlock
and relock different flags. It stages on a copy, verifies that only intended
bytes changed, reads every target back, and updates the caller's buffer only
after all checks pass. `ApplyBatchToFile` supports several characters in one
new output and never overwrites its input.

No slot checksum covers the event-flag region, but the explicit byte and
read-back checks remain mandatory. Unresolvable flags are skipped on reads and
rejected on writes.

## Characters view

The view drills from character to merchant to shared-flag groups. Rows sharing
one release flag render as one checkbox. Staging back to the on-disk value
removes the pending edit. Selecting a character also controls lock badges in
the Shop Editor; with no selection, gated rows display as locked.

Twin Maiden Husks shows both row-backed release flags and talk-script bell
bearing flags from `bell_bearings.go`. Scroll and prayerbook groups for Corhyn,
Miriel, and Sellen use the same staged flag path. All flag edits merge with
item and price edits in one application save plan.

## Enia safety rule

Never expose or write Enia's release flags. Her armor/reward flags overlap the
global `9100-9199` boss-defeat signals; changing them has triggered boss
cutscenes and teleports on real hardware. Enia is therefore excluded from the
Characters view, her row-level gate action is disabled, and write staging has
a defensive name check.

A game-wide audit found the dangerous boss-flag overlap only on Enia. Other
merchant release flags represent their own quest progression. Item swaps are
safe from this class of collision because neither `eventFlag_forRelease` nor
`eventFlag_forStock` is modified.

## Tests

Fixture-backed tests cover slot identity, flag position parity with the
SaveForge oracle, lock-state reads, mixed-direction writes, multi-character
outputs, bell bearings, and alias detection. UI tests cover grouping, staging,
selection, and combined-save behavior. Keep the Enia guards and shared-flag
grouping explicitly tested whenever this feature changes.
