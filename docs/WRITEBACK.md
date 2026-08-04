# Write-back engine (shopwrite)

Go library + CLI that edits existing `ShopLineupParam` rows in place in a PS4
save and writes a new save. Value edits only, same row count (see PROJECT.md
"Row-count ceiling"). Read decode originally ported from savescan (now
`tools/savescan.py`, kept as the golden-test oracle); write side originally
ported from `er_pvp_mod`'s zstd raw-block patch, replaced 2026-08-02 by full
recompression (see "Recompression" below; ER_PVP_MOD_REFERENCE.md keeps the
raw-block-patch history for attribution).

## Location / build

`app/shopwrite/` — **package `shopwrite`** (importable; the GUI calls
`Apply` in-process): `apply.go` (Apply/Edit/Summary + encrypt/splice +
round-trip self-check), `pipeline_crypto.go` (load/decrypt/DCX/zstd-decode
to a BND4 blob), `pipeline_bnd4.go` (BND4 archive), `pipeline_param.go`
(inner PARAM header/row table + field-encode), `pipeline_schema.go` (JSON
row-schema loading — 2026-08-01, split from one `pipeline.go` by pure code
motion, see docs/EDITOR.md's cleanup-pass entry), `decode.go` (field
decode, inverse of encode), `recompress.go` (fresh full zstd re-encode of
the patched blob). `app/cmd/shopwrite` is the thin CLI wrapper (same
flags/exit codes as the pre-refactor tool).

Root module, one dep: `klauspost/compress` (zstd — decode, encode, and the
self-check). Pure Go, CGO-free. The schema JSON is embedded (`data/embed.go`),
so built binaries are standalone.

## Usage

```
go run ./app/cmd/shopwrite -save <in.dat> -out <out.dat> -entry ShopLineupParam.param -edits <edits.json>
```

`-out` must differ from `-save` (never writes the input). Summary (rows
touched, stream growth vs. capacity, output path) prints to stderr; any
validation/capacity/self-check error exits non-zero and writes nothing.
Prebuilt CLI binaries: `app/build.sh` → `app/dist/shopwrite/<goos>-<goarch>/`.

## Recompression (2026-08-02 — replaced the Raw-block-patch write path)

**Problem this fixed**: the original write path (ported from `er_pvp_mod`,
see ER_PVP_MOD_REFERENCE.md) never re-encoded the zstd stream — it replaced
only the 64KB decompressed-cadence block(s) an edit's bytes fell into with
uncompressed Raw blocks, verbatim-copying everything else. Each replaced
block cost a fixed ~55-65KB of growth (Raw is bigger than the Compressed
block it replaces) against a fixed ~323KB total capacity slack — a hard
ceiling of ~5-6 touched blocks *per save file, ever* (growth is cumulative).
Editing many distinct items at once (e.g. price-0-ing a big chunk of a
merchant's stock, each item's own `EquipParamGoods/Weapon/...` row usually
landing in a different scattered block) blew this immediately — real case:
zeroing half of Enia's ~100-item stock needed 49 distinct `EquipParamProtector`
blocks alone, 147KB over budget before even touching `ShopLineupParam`.

**Root cause of the original design choice**: `er_pvp_mod`'s PS4 branch
avoids recompression entirely, per its own comment: "PS4 saves have no MD5
prefix... so any recompression of the ZSTD frame produces different
ciphertext that PS4 rejects." That claim was made for `NETWORK_PARAM_ST` (a
small, different param block) and never verified against `regulation.bin`.
Direct evidence it doesn't hold here: `save_files/BetterPSN.dat`, a real
third-party-edited PS4 save confirmed working in-game (edits all 93 Twin
Maiden Husks rows + 79 items' sellValue with no reported problems), has a
regulation.bin stream that is **824/824 Compressed blocks, zero Raw** — a
genuine full recompression, and its stream is 2,027,465 bytes, *smaller*
than vanilla's own 2,036,176-byte stream despite the edits. Confirmed by
directly parsing both fixtures' zstd block streams.

**Fix (first attempt, 2026-08-02, later found incomplete)**: `recompress.go`'s
`buildRecompressedStream` fully re-encodes the patched BND4 blob with
`klauspost/compress/zstd` (`zstd.SpeedBestCompression`, via `EncodeAll`)
instead of patching individual blocks. `apply.go`'s block-walk/Treeless-
successor/iterative-repair machinery is gone entirely — there's no block
concept anymore, so the whole class of "verbatim-kept Compressed block
LZ-references into an edited range" bug (the 2026-07-27 corruption that
motivated the original round-trip self-check) can't occur; the self-check
itself stays (decompress the new stream, byte-compare against the intended
patched blob, refuse to write on any mismatch) as cheap insurance against a
plain encoder/decoder bug, non-iterative now.

**Real in-game test crashed on load (2026-08-02, same day)**: offline
verification (capacity, round-trip self-check, full test suite) all passed,
but the game crashed immediately on load of the actual output. Root-caused
by directly comparing our stream's zstd frame header against vanilla's own
and `save_files/BetterPSN.dat`'s (both confirmed working in-game):

| | vanilla | BetterPSN | ours (first attempt) |
|---|---|---|---|
| Content_Checksum_flag | false | false | **true** |
| Frame_Content_Size field | absent | absent | **present** |
| Window size | 64MB | **64KB** | 8MB |

The `WithEncoderCRC defaults false` claim above was simply wrong —
klauspost's actual default is `crc: true` (checked directly in
`encoder_options.go`). `EncodeAll` also always knows and writes the exact
content size up front, which neither reference stream does. Confirmed
authoritative via **SoulsFormats** (`soulsmods/SoulsFormatsNEXT`, the
trusted .NET library the whole FromSoftware modding ecosystem — WitchyBND,
Smithbox, etc. — uses to read/write these exact DCX-ZSTD files; **treat it
as the most trusted source for any FromSoft container-format question**):
its own write path, `Utilities/Compression/ZstdHelper.cs`'s `WriteZstd`,
sets exactly `ZSTD_c_contentSizeFlag = 0` and `ZSTD_c_windowLog = 16` — the
windowLog=16 matches BetterPSN's own measured window exactly, confirming
this is the real, deliberate requirement, not one tool's accident.

**Fix (corrected)**: switched from `EncodeAll` (always sets Frame_Content_Size)
to the streaming `Write`+`Close` API, with `zstd.WithWindowSize(65536)` and
`zstd.WithEncoderCRC(false)` explicitly. Our output's frame header now
matches BetterPSN's exactly (checksum off, window 64KB, content size absent)
— verified directly, not just decode-tested. Regression test:
`TestBuildRecompressedStreamMatchesRequiredFrameShape`.

**Result**: value-only edits essentially never hit the capacity ceiling
again — a real re-run of the Enia scenario above (50 rows, price 0) that
used to fail by 147KB now succeeds with 274KB of slack *remaining* even at
the smaller, correct 64KB window (was 366KB at the incorrect 8MB window —
the smaller mandated window compresses slightly worse, still comfortably
enough). Regression test: `TestApplyManyDistinctRowsInOneWrite` (all 93 TMH
rows in one `Apply` call).
`Summary.TouchedBlocks`/`ReplacedBlocks` fields removed (no block concept
left); `TestApplyBaseline` no longer asserts sha256-identity against a
captured pre-refactor baseline (exact bytes now depend on the zstd encoder,
not just the edits) — it asserts every edited field landed correctly and
every other row is byte-for-byte unchanged, which is the invariant that
actually matters.

## Other param tables (2026-07-30 — EquipParam* sellValue)

`Apply` is a byte-identical thin wrapper (still hardcoded to
`ShopLineupParam` — the corruption-tested, trusted path, left untouched)
around unexported `applyWithSchema`, which everything else (recompression,
round-trip self-check, capacity check, splice/encrypt) is shared with.
`ApplyWithSchema(savePath, outPath, entryName, edits, schema)` is the same
function open to ANY embedded schema/entry pair — added for the cost=0 fix
(see `docs/MERCHANT_DATA.md`), which needs to clear an item's own
`sellValue` in its `EquipParamWeapon/Protector/Accessory/Goods/Gem` row, a
completely separate table/row-id-space from `ShopLineupParam`.
`LoadEquipParamSchema(equipType)`/`EquipParamEntryName(equipType)`
(`pipeline_schema.go`) resolve the right schema/BND4-entry-name for one of those 5
tables, keyed 0-4 the same way `ShopLineupParam`'s own `equipType` field
already is. `ParseParamHeader`/`IterParamRows` (row lookup) derive row
count/offset from the target file's own binary header, not the schema, so
they needed no changes — the schema is only consulted for the specific
field's computed byte offset. 5 new embedded schemas
(`data/equip_param_*_schema.json`, generated by
`tools/equip_param_schema_extract`, mirrors `tools/paramdex_extract`
exactly — no new schema-computation logic). `app/editor/state.go`'s
`combinedApplyWorker` chains multiple `ApplyWithSchema`/`ApplyEdits`/
`charunlock.ApplyBatchToFile` passes through numbered `.tmpN` files when
more than one kind of edit is staged at once (flags, sellValue, item
edits) — same never-overwrite-the-input discipline every individual write
already enforces on its own, just generalized from a fixed 2-stage switch
to an ordered N-stage pipeline.

`edits.json` — array of `{row_id, fields:{name:value}}`; field names/types per the
schema. Example:
```json
[
  {"row_id": 100000, "fields": {"equipId": 12345, "equipType": 0, "value": 0, "sellQuantity": -1}},
  {"row_id": 101802, "fields": {"eventFlag_forRelease": 0}}
]
```

## Validation / guards (error out, write nothing)

- row_id must exist in the param's row table; field must exist and not be `dummy8`.
- value must fit the field's declared type width (e.g. `s16` rejects 40000).
- zstd frame `Content_Checksum_flag` set on the INPUT -> error (we don't
  maintain the xxhash64 trailer). Cleared in our fixtures; irrelevant to our
  own output, which never sets it (see "Recompression" above).
- DCX format must be ZSTD; UserData11 must be PS4 (`" GER"` magic).

## Mechanics

- Edits are applied to the mutable decompressed BND4 blob at
  `entry.offset + row.data_offset + field.offset` (little-endian).
- The whole patched blob is then re-encoded as a fresh zstd stream (see
  "Recompression" above) — no block-boundary bookkeeping.
- **Round-trip self-check**: decompress the new stream and byte-compare
  against the intended patched blob; any mismatch refuses to write. Cheap
  insurance against an encoder/decoder bug. Regression:
  `TestApplySingleBlockEditRoundTrips`, `TestApplyManyDistinctRowsInOneWrite`.
- DCX header `compressed_size` (BE @0x20 within the 76-byte header) patched to the
  new stream length; `decompressed_size` unchanged.

## Capacity check

Capacity = `len(UserData11) - 0x10 (unk) - 16 (IV)`, computed from the actual
input file (not hardcoded). New DCX blob (76-byte header + new stream) must fit;
otherwise error with bytes-over-budget. Vanilla fixture: capacity 2,359,280 B.
Under recompression, value-only edits rarely grow the stream at all (often
shrink it — see "Recompression" above), so this essentially never fires now;
it stays as a hard guard for the pathological case. Plaintext padded with
zeros to the fixed capacity, then AES-256-CBC re-encrypted with the **same
IV** (capacity is 16-aligned; no padding scheme).

Independently re-verified against external sources (official zstd spec,
real SoulsFormats source, AES-key provenance) — full detail:
[ER_PVP_MOD_REFERENCE.md](ER_PVP_MOD_REFERENCE.md)'s "Independent
verification" section.

## Verification history

- 2026-07-25 (Raw-block-patch era, superseded): edited 4 rows across 4
  merchants spanning a real block boundary (row `111105`). Full-content byte
  diff of both decompressed BND4 blobs: 23 differing bytes, 0 outside the
  intentionally-edited field ranges.
- 2026-07-27 (Raw-block-patch era, superseded): found+fixed a real
  corruption (verbatim-kept Compressed block LZ-referenced into an edited
  range) — motivated the round-trip self-check, which is still in place
  today under recompression.
- 2026-08-02 (first recompression attempt): `TestApplyManyDistinctRowsInOneWrite`
  — all 93 TMH rows edited in one `Apply` call, impossible under
  Raw-block-patch (see the Enia 147KB-overflow case above), now succeeds
  with capacity to spare. Full offline test suite (`go test ./app/...`)
  green. **All offline checks passed but the game crashed on load** —
  offline verification alone isn't sufficient for this class of change; see
  "Real in-game test crashed on load" above.
- 2026-08-02 (corrected, real crash test, **user-confirmed in-game**): a
  genuine stress test — every row of all 39 real merchants (608 rows)
  swapped to one item, price 0, quantity unlimited, in a single write,
  verified row-by-row against the original offline (gate flags untouched,
  material-locked rows untouched, all 1277 rows present). **Loaded
  in-game successfully, no crash, every item edited correctly** — the
  first real hardware confirmation of the corrected recompression write
  path, not just offline verification. This is now the strongest evidence
  this project has that full recompression is safe for `regulation.bin`
  on PS4.
