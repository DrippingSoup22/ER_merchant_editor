# `er_pvp_mod` prior art

The user's external `er_pvp_mod` project supplied the initial PS4
`USERDATA11` decrypt/decompress/re-encrypt pipeline. It edits a different PARAM
table and is not part of this repository.

Reused and independently checked facts:

- PS4 `USERDATA11` fixed offsets and `" GER"` marker;
- AES-256-CBC key, IV placement, and fixed ciphertext capacity;
- DCX/zstd containing a BND4 archive of named PARAM entries.

The key and format were cross-checked against SoulsFormatsNEXT,
SoulsAssetPipeline, ParamCrypt, and ER-Save-Lib. BND4/PARAM field parsing was
checked against SoulsFormatsNEXT and fixture row spacing. The local parser is
intentionally limited to Elden Ring's little-endian, Unicode-name BND4 shape.

## Superseded raw-block writer

`er_pvp_mod` avoided PS4 zstd recompression by replacing touched compressed
blocks with raw blocks. This project initially followed that design, then
removed it because:

- output grew roughly one raw 64 KiB block per touched region;
- growth accumulated within the fixed save allocation;
- preserved compressed blocks could reference changed predecessor data;
- multi-table merchant edits quickly exceeded capacity.

A real, working third-party PS4 save proved that `regulation.bin` can be fully
recompressed. The current writer does so and matches SoulsFormats' required
frame shape. See [WRITEBACK.md](WRITEBACK.md). None of the old block-successor
or raw-growth rules apply to current code.

The independent Python decoder remains useful as a second implementation for
fixture tests, but `internal/savefile` is the runtime authority.
