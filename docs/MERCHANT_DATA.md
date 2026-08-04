# Merchant data in `USERDATA11`

Merchant data is stored in the save's embedded `regulation.bin`; the outer
PlayStation encryption/signature is not handled by this project.

## Container

For the verified PS4 fixture:

- `USERDATA11`: `0x1960070` through `0x1ba0080`;
- layout: 16-byte unknown header, 16-byte IV, AES-256-CBC ciphertext;
- AES key: `99 BF FC 36 6A 6B C8 C6 F5 82 7D 09 36 02 D6 76 C4 28 92 A0 1C 20 7F B0 24 D3 AF 4E 49 3F EF 99`;
- plaintext: DCX/zstd-compressed BND4 archive of PARAM files.

`internal/savefile` derives capacity from the input, preserves the IV, fully
recompresses the modified BND4, patches the DCX compressed size, zero-pads to
the fixed region size, and re-encrypts. See [WRITEBACK.md](WRITEBACK.md).

## Merchant mechanics

`ShopLineupParam.param` contains 1,277 fixed rows. Important behavior:

- `sellQuantity`: `-1` unlimited, `0` unavailable, `1..255` safe finite
  stock. Larger values wrap through an 8-bit in-game counter.
- `value`: displayed price. The editor caps it at `999999`; seven-digit
  values have caused UI corruption in game.
- `eventFlag_forRelease`: per-character visibility gate. Item edits preserve
  it; the Characters view changes the corresponding slot flag separately.
- `mtrlId`: extra material cost through `EquipMtrlSetParam`; such rows are
  intentionally not editable.
- Item visibility requires shop price to be at least the item's
  `EquipParam*.sellValue`. Item swaps therefore stage any required sell-value
  reduction before the shop-row edit.

The application save plan—not `catalog.ApplyEdits` alone—is the correct path
for realistic item-swap tests because it includes sell-value and character
flag stages.

## Verification

The Python `tools/savescan.py` decoder is retained as an independent oracle.
Fixture-backed Go tests compare decoded rows and mutation results against it.
Both files under `save_files/` are read-only; tests write only temporary or
explicit test copies.

Schema and merchant attribution details live in [SHOP_LINEUP.md](SHOP_LINEUP.md)
and [MERCHANTS.md](MERCHANTS.md).
