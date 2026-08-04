# Findings mined from er_pvp_mod (prior art)

Source: `/mnt/c/Users/danie/Desktop/er_pvp_mod` (external, the user's own
prior tool, not part of this repo — read-only). Edits `NETWORK_PARAM_ST`
via a decrypt/decompress/parse/edit/recompress/re-encrypt/write cycle,
confirmed working on real PS4 saves. Its PS4 zstd raw-block-patch
technique is what `internal/savefile` adapts and generalizes — that tool's
code is now the canonical implementation; this doc keeps only the
non-obvious facts/gotchas that aren't visible just by reading it.

## What we reused

- PS4 `UserData11` pipeline: fixed offsets, AES-256-CBC decrypt (fixed key,
  same for PC/PS4), DCX/zstd decompress, BND4 archive of named `.param`
  files. Offsets confirmed against our own fixture
  (`save_files/vanilla_fresh_character.dat`, 28,967,040 bytes):
  `ud11Off = 0x70 + 10*0x280000 + 0x60000 = 0x1960070`,
  `ud11End = ud11Off + 0x240010 = 0x1ba0080` (== file size). Byte 0 at
  `ud11Off` is `" GER"`, confirming the no-MD5 (PS4) branch.
- PS4 raw-block-patch write technique (superseded 2026-08-02): er_pvp_mod's
  PS4 branch avoids real zstd re-encoding, per its own comment: "PS4 saves
  have no MD5 prefix... so any recompression of the ZSTD frame produces
  different ciphertext that PS4 rejects." `internal/savefile` originally
  generalized er_pvp_mod's raw-block-patch (single-row/single-block case)
  to multiple, non-contiguous edited rows on that same assumption. Directly
  disproven for `regulation.bin` specifically: `save_files/BetterPSN.dat`,
  a real third-party-edited PS4 save confirmed working in-game, carries a
  genuinely fully-recompressed regulation.bin stream (824/824 Compressed
  blocks, zero Raw) smaller than vanilla's own. `internal/savefile` now fully
  recompresses too — see `docs/WRITEBACK.md`'s "Recompression" section.
  (er_pvp_mod's own PC branch already used real recompression via
  `compressDCX`/`zstd.NewWriter` — the constraint was PS4-specific by that
  tool's own design, just apparently overcautious for this particular file.)

## Gotchas from the Raw-block-patch era (superseded 2026-08-02, kept for history)

`internal/savefile` no longer patches individual blocks (see "Recompression" in
`docs/WRITEBACK.md`), so none of these gotchas apply to its current code —
kept here as the record of what the old technique required and why.

- **Growth capacity is the fixed UD11 ciphertext region size, not the
  previously-used stream length.** Confirmed ~323KB (vanilla) / ~332KB
  (BetterPSN) of real spare capacity in both fixtures — a structural
  property of the PS4 format's fixed-size allocation, not an accident of
  one file. Covers roughly 5-6 touched 64KB blocks per write (~57KB growth
  each, since a Raw block is bigger than the compressed block it
  replaces). Does **not** enable adding rows: inserting bytes would shift
  every byte after the insertion point, requiring the entire remainder of
  the stream to be re-encoded as Raw — and `ShopLineupParam.param` sits at
  ~41MB of a ~54MB decompressed archive, so ~13MB of trailing content would
  need it, vastly exceeding the slack. Growth is cumulative across
  successive writes to the same file; `internal/savefile` computes needed
  growth and checks against actual remaining capacity at write time.
- zstd's `Content_Checksum_flag` is never read/handled in `regulation.go`;
  harmless since it's cleared in both our real fixtures, but
  `internal/savefile` adds an explicit guard (errors out) rather than
  inheriting the silent assumption.
- A real row (`111105`) straddles a 64KB block boundary in
  `ShopLineupParam.param` — confirms the multi-block patch path is
  exercised in practice, not just a hypothetical edge case.
- Treeless_Literals successor blocks (Compressed blocks that reuse the
  previous block's Huffman table) must also convert to Raw when their
  predecessor is patched, chained transitively if needed — er_pvp_mod
  already handles this for the single-block case; `internal/savefile` ports
  it.
- RLE blocks (type 1): not expected in regulation.bin: guarded as a hard
  error rather than silently mishandled (none seen in either fixture).

## Independent verification against external sources (2026-07-25)

Re-checked our AES-key/pipeline understanding against sources outside the
SaveForge/er_pvp_mod lineage (both trace to the same origin, not two
independent confirmations):

- **zstd block/frame parsing** (`internal/savefile/recompress.go`/
  `pipeline_crypto.go`): checked bit-for-bit against the official Zstandard
  Compression Format spec. Full pass, zero discrepancies.
- **PARAM/BND4 container parsing** (`tools/savescan.py`,
  `internal/savefile/pipeline_bnd4.go`/`pipeline_param.go`): re-derived field-by-field against
  `soulsmods/SoulsFormatsNEXT`'s actual source. Full pass — the earlier
  off-by-4 fix independently re-confirmed. Two harmless nits fixed:
  Python's `_read_utf16le_cstr` could infinite-loop on a malformed blob
  (now bounds-checked); Go read two `int64`-typed offsets as unsigned
  before casting (now cast through `int64` first, matching the real
  source's signedness). Confirmed narrower-than-spec-but-fine-for-ER: our
  BND4 parser hardcodes little-endian/Unicode-names/no-hash-table and the
  0x24-byte entry stride rather than computing it from header flags — fine
  for ER's regulation.bin specifically, would misparse a differently-
  flagged archive.
- **AES-256 key + PS4 offsets**: the identical 32-byte key appears in
  `SoulsFormatsNEXT`, `Meowmaritus/SoulsAssetPipeline` (predates ER's
  release), `Grimrukh/ParamCrypt`, and `ClayAmore/ER-Save-Editor`/
  `ER-Save-Lib` (predates SaveForge by ~2 years, one of SaveForge's own 3
  cited reference editors — so the lineage is ClayAmore -> SaveForge -> us,
  not circular). Spans ~4 years, multiple unrelated authors, no PC/PS4/PS5
  key split, no evidence the key changed across patches/DLC (only the
  compression format changed, DFLT->ZSTD at patch 1.12, already
  format-sniffed). `ClayAmore/ER-Save-Lib` independently corroborates the
  PS4 offset math byte-for-byte.
