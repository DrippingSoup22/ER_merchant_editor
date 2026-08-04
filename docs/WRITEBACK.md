# Write-back engine

`internal/savefile` mutates PARAM fields inside the save's embedded
`regulation.bin`. `cmd/shopwrite` is a thin diagnostic CLI; the desktop app
calls the package through `internal/application.SavePlan`.

```sh
go run ./cmd/shopwrite \
  -save input.dat -out output.dat \
  -entry ShopLineupParam.param -edits edits.json
```

Input and output paths must differ. An edits file is an array of objects such
as:

```json
[{"row_id":100000,"fields":{"equipId":12345,"equipType":0,"value":0,"sellQuantity":-1}}]
```

## Pipeline

1. Load and decrypt `USERDATA11`.
2. Decompress DCX/zstd and locate the named BND4 PARAM entry.
3. Validate row IDs, field names, types, and widths before mutation.
4. Patch the decompressed BND4 bytes.
5. Fully recompress with the required zstd frame shape.
6. Decompress the new stream and compare it byte-for-byte with the intended
   patched BND4.
7. Patch the DCX compressed size, enforce fixed-region capacity, encrypt with
   the original IV, and write the new output.

Any failure writes nothing.

## Required zstd shape

The game-compatible frame matches SoulsFormats' `ZstdHelper.WriteZstd`:

- no content checksum;
- no explicit frame content size;
- 64 KiB window.

`recompress.go` therefore uses the streaming encoder with
`WithEncoderCRC(false)` and `WithWindowSize(65536)`. Do not replace it with
`EncodeAll`; that produces a frame which passes offline decompression but has
crashed on real hardware. `TestBuildRecompressedStreamMatchesRequiredFrameShape`
guards this contract.

## APIs and guards

- `Apply` uses the embedded `ShopLineupParam` schema.
- `ApplyWithSchema` supports the five `EquipParam*` tables used for
  `sellValue` synchronization.
- row and field must exist; padding fields cannot be edited;
- values must fit the declared PARAM type;
- input must be the expected PS4/DCX-zstd form;
- the rebuilt DCX must fit the fixed `USERDATA11` capacity;
- output is accepted only after round-trip verification.

Character flags use a separate verified path in `internal/character`; the
application layer chains flag, equip-param, and shop-row stages through
temporary files and removes intermediates afterward.

The full-recompression path has been real-hardware tested with hundreds of
row changes in one output. Earlier raw-block-patch history and attribution are
summarized in [ER_PVP_MOD_REFERENCE.md](ER_PVP_MOD_REFERENCE.md).
