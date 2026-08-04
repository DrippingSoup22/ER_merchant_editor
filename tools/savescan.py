#!/usr/bin/env python3
"""Read-only Elden Ring PS4 save-file inspection tool.

Pipeline (see docs/MERCHANT_DATA.md for the full writeup + citations):
    save file -> UserData11 (fixed offset) -> AES-256-CBC decrypt
    -> DCX decompress (zstd/deflate) -> BND4 archive of named param files
    -> (per named .param file) its own PARAM container header + row table
    -> (per row, given a schema from data/shop_lineup_schema.json) named
       fields.

No write-back here — this is the discovery-phase tool, not the editor.
"""

import argparse
import json
import struct
import sys
import zlib
from pathlib import Path

import zstandard

DATA_DIR = Path(__file__).resolve().parents[1] / "data"

# PARAM container header flags (Format2D), per SoulsFormats' PARAM.cs —
# this is the *inner* .param file's own header, distinct from the outer
# BND4 archive format above.
FMT1_FLAG01 = 0b0000_0001
FMT1_INT_DATA_OFFSET = 0b0000_0010
FMT1_LONG_DATA_OFFSET = 0b0000_0100
FMT1_OFFSET_PARAM_TYPE = 0b1000_0000

STRUCT_FMT = {
    "s8": "<b", "u8": "<B",
    "s16": "<h", "u16": "<H",
    "s32": "<i", "u32": "<I", "b32": "<i", "angle32": "<f",
    "f32": "<f", "f64": "<d",
}

# ShopLineupParam.equipId is a raw row ID into the per-category EquipParam*
# table named by equipType (SHOP_LINEUP_EQUIPTYPE), NOT the unified item-ID
# used elsewhere (items.json/inventory). Confirmed empirically (2026-07-24)
# against known items across all 5 categories: item_id = equipId + offset,
# matching the item-ID-space high-nibble convention from docs/ITEM_IDS.md.
EQUIP_TYPE_ITEM_ID_OFFSET = {
    0: 0x00000000,  # Weapon
    1: 0x10000000,  # Protector (armor)
    2: 0x20000000,  # Accessory (talisman)
    3: 0x40000000,  # Good
    4: 0x80000000,  # Gem (Ash of War)
    # 5, 6: seen in real data (e.g. weapon-locked unique skills) but undocumented
    # in Paramdex's SHOP_LINEUP_EQUIPTYPE enum (which only lists 0-4) — unresolved.
}

# ShopLineupParam.mtrlId is a row ID into EquipMtrlSetParam.param (a separate
# BND4 entry), not an item id — see docs/SHOP_LINEUP.md's "mtrlId resolution".
# Its materialCateNN fields use the same GAITEM_CATEGORY 0-4 numbering/offsets
# as equipType above, so EQUIP_TYPE_ITEM_ID_OFFSET is reused for both.
MATERIAL_SLOT_COUNT = 6

# Row fields that, when non-(-1), mean "item-swap-inherits-display" doesn't
# hold for this row (confirmed 2026-07-25, see docs/SHOP_LINEUP.md's
# "Data-quality audit"). equipType 5/6 have no known item-id offset at all.
NAME_ICON_OVERRIDE_FIELDS = ("iconId", "nameMsgId", "menuTitleMsgId", "menuIconId")
UNRESOLVED_EQUIP_TYPES = (5, 6)

# Paramdex's own raw Names file labels these 15 rows "Enia - Forging",
# distinct from both her armor rows ("Enia - <Boss>") and the actual reward
# rows ("Remembrance of <Boss>", row_id 101900+, mtrlId-gated -- see
# docs/MERCHANTS.md). item here is the Remembrance itself (equipId resolves
# to the same items.json entry the player carries), price=0, no mtrlId, but
# a unique per-boss eventFlag_forRelease/eventFlag_forStock pair unlike
# every other Enia row. Confirmed via research (2026-07-25) you cannot buy
# back a Remembrance from any merchant after redeeming it -- so this can't
# be a player-visible "for sale" row (matches user's own recollection: "she
# doesn't sell them, she uses them"). Best-supported theory: this is the
# engine's internal trigger for the hand-in dialogue action itself (buying
# it, at cost 0, is what the talk-script does when you choose to redeem),
# never routed through the browsable shop UI -- not confirmed via any
# authoritative source, flagged as a warning rather than hidden/reclassified
# so the underlying data stays fully visible/editable.
ENIA_FORGING_ROW_IDS = frozenset(
    {101775, 101776, 101777, 101778, 101779, 101780, 101781}
    | {101785, 101786, 101787, 101788, 101789, 101790, 101791, 101792}
)

SLOT_SIZE = 0x280000
NUM_SLOTS = 10
PS4_HEADER_SIZE = 0x70
USERDATA10_SIZE = 0x60000
UD11_UNK_HEADER_SIZE = 0x10

REGULATION_KEY = bytes.fromhex(
    "99BFFC366A6BC8C6F5827D093602D676"
    "C42892A01C207FB024D3AF4E493FEF99"
)


def userdata11_bounds(file_size: int) -> tuple[int, int]:
    off = PS4_HEADER_SIZE + NUM_SLOTS * SLOT_SIZE + USERDATA10_SIZE
    return off, file_size


def load_userdata11(path: str) -> bytes:
    data = Path(path).read_bytes()
    off, end = userdata11_bounds(len(data))
    return data[off:end]


def decrypt_regulation(ud11: bytes) -> bytes:
    from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes

    blob = ud11[UD11_UNK_HEADER_SIZE:]
    iv, ciphertext = blob[:16], blob[16:]
    cipher = Cipher(algorithms.AES(REGULATION_KEY), modes.CBC(iv))
    decryptor = cipher.decryptor()
    plaintext = decryptor.update(ciphertext) + decryptor.finalize()
    if plaintext[:4] != b"DCX\x00":
        raise ValueError(f"expected DCX magic after decrypt, got {plaintext[:4]!r}")
    return plaintext


def decompress_dcx(dcx: bytes) -> bytes:
    fmt = dcx[40:44]
    decompressed_size = struct.unpack(">I", dcx[28:32])[0]
    compressed_size = struct.unpack(">I", dcx[32:36])[0]
    if fmt == b"ZSTD":
        payload = dcx[76 : 76 + compressed_size]
        return zstandard.ZstdDecompressor().decompress(payload, max_output_size=decompressed_size)
    if fmt == b"DFLT":
        payload = dcx[76 + 2 : 76 + compressed_size]  # skip 2-byte zlib header
        return zlib.decompress(payload, -15)
    raise ValueError(f"unknown DCX format {fmt!r}")


def list_bnd4_entries(blob: bytes) -> list[dict]:
    # Field offsets verified against er_pvp_mod/core/regulation.go's
    # findNetworkParamInBND4 (entryOff+8/+24/+32), not re-derived from spec.
    if blob[:4] != b"BND4":
        raise ValueError(f"expected BND4 magic, got {blob[:4]!r}")
    count = struct.unpack("<I", blob[0x0C:0x10])[0]
    entries = []
    for i in range(count):
        base = 0x40 + i * 0x24
        entry = blob[base : base + 0x24]
        comp_size = struct.unpack_from("<Q", entry, 8)[0]
        data_off = struct.unpack_from("<I", entry, 24)[0]
        name_off = struct.unpack_from("<I", entry, 32)[0]
        name = _read_utf16le_cstr(blob, name_off)
        entries.append({"name": name, "offset": data_off, "size": comp_size})
    return entries


def _read_utf16le_cstr(blob: bytes, offset: int) -> str:
    end = offset
    while blob[end : end + 2] != b"\x00\x00":
        if end + 2 > len(blob):
            raise ValueError(f"UTF-16LE string at offset {offset} has no terminator before end of blob")
        end += 2
    return blob[offset:end].decode("utf-16-le")


def extract_bnd4_entry(blob: bytes, name: str) -> bytes:
    for entry in list_bnd4_entries(blob):
        if entry["name"] == name or entry["name"].endswith("\\" + name) or entry["name"].endswith("/" + name):
            off, size = entry["offset"], entry["size"]
            return blob[off : off + size]
    raise KeyError(f"no BND4 entry named {name!r}")


def parse_param_header(blob: bytes) -> dict:
    """Parse a single extracted `.param` file's own container header
    (distinct from the outer BND4 archive format). Ported from
    SoulsFormats' PARAM.cs Read(), trimmed to the fields we need — general
    over the Format2D flag combinations, not hardcoded to one offset.
    """
    format2d = blob[0x2D]
    format2e = blob[0x2E]
    long_data_offset = bool(format2d & FMT1_LONG_DATA_OFFSET)
    int_data_offset_expanded = bool(format2d & FMT1_FLAG01) and bool(format2d & FMT1_INT_DATA_OFFSET)

    pos = 0
    strings_offset = struct.unpack_from("<I", blob, pos)[0]
    pos += 4
    pos += 2  # reserved int16 (Flag01&IntDataOffset, or LongDataOffset) or short-format "data start" — unused either way
    unk06 = struct.unpack_from("<h", blob, pos)[0]
    pos += 2
    paramdef_data_version = struct.unpack_from("<h", blob, pos)[0]
    pos += 2
    row_count = struct.unpack_from("<H", blob, pos)[0]
    pos += 2

    if format2d & FMT1_OFFSET_PARAM_TYPE:
        pos += 4  # assert int32(0)
        param_type_offset = struct.unpack_from("<q", blob, pos)[0]
        pos += 8
        pos += 0x14  # asserted-zero pattern
        param_type = _read_ascii_cstr(blob, param_type_offset) if param_type_offset else None
    else:
        param_type = blob[pos : pos + 0x20].split(b"\x00", 1)[0].decode("ascii")
        pos += 0x20

    # "Format" (unused) — these are the same 4 bytes as the BigEndian/Format2D/
    # Format2E/ParamdefFormatVersion quartet at 0x2C-0x2F we already read above
    # by absolute offset; do not also add 4 bytes for that quartet separately.
    pos += 4

    if int_data_offset_expanded:
        pos += 4 + 4 + 4 + 4  # data start (int32) + 3x assert-zero int32
    elif long_data_offset:
        pos += 8 + 8  # data start (int64) + assert-zero int64

    return {
        "strings_offset": strings_offset,
        "unk06": unk06,
        "paramdef_data_version": paramdef_data_version,
        "row_count": row_count,
        "param_type": param_type,
        "format2d": format2d,
        "format2e": format2e,
        "long_data_offset": long_data_offset,
        "row_table_offset": pos,
    }


def _read_ascii_cstr(blob: bytes, offset: int) -> str:
    end = blob.index(b"\x00", offset)
    return blob[offset:end].decode("ascii")


def iter_param_rows(blob: bytes, header: dict) -> list[dict]:
    entry_size = 24 if header["long_data_offset"] else 12
    rows = []
    for i in range(header["row_count"]):
        base = header["row_table_offset"] + i * entry_size
        row_id = struct.unpack_from("<i", blob, base)[0]
        if header["long_data_offset"]:
            data_offset = struct.unpack_from("<q", blob, base + 8)[0]
        else:
            data_offset = struct.unpack_from("<I", blob, base + 4)[0]
        rows.append({"id": row_id, "data_offset": data_offset})
    return rows


def load_schema(name: str) -> dict:
    return json.loads((DATA_DIR / name).read_text())


def decode_row_fields(blob: bytes, data_offset: int, schema: dict) -> dict:
    values = {}
    for field in schema["fields"]:
        if field["type"] == "dummy8":
            continue
        fmt = STRUCT_FMT[field["type"]]
        values[field["name"]] = struct.unpack_from(fmt, blob, data_offset + field["offset"])[0]
    return values


def _decoded_bnd4(save_path: str) -> bytes:
    ud11 = load_userdata11(save_path)
    plaintext = decrypt_regulation(ud11)
    return decompress_dcx(plaintext)


def equip_ref_for_item_id(item_id: int) -> tuple[int, int] | None:
    """Inverse of resolve_item_id: items.json's unified item_id -> the
    (equipType, equipId) pair a ShopLineupParam row needs. The 5 category
    offsets are non-contiguous but never overlap in real data (confirmed
    empirically against all 2772 items.json entries — huge gaps between
    each category's max id and the next offset up), so "largest offset
    <= item_id" always picks the right bucket."""
    applicable = [(equip_type, offset) for equip_type, offset in EQUIP_TYPE_ITEM_ID_OFFSET.items()
                  if item_id >= offset]
    if not applicable:
        return None
    equip_type, offset = max(applicable, key=lambda p: p[1])
    return equip_type, item_id - offset


def find_items_by_name(name: str, items: list[dict]) -> list[dict]:
    """Case-insensitive exact-name lookup. ~0.25% of items.json (7/2772)
    share a name across categories (e.g. a sorcery and an Ash of War both
    called "Glintstone Pebble") — callers must handle >1 result."""
    return [i for i in items if i["name"].lower() == name.lower()]


def resolve_item_id(raw_id: int, category: int, items_by_id: dict) -> int | None:
    """equipId (+equipType) or a single materialId (+materialCate) -> the
    unified item id used by items.json. Trusts `category`'s offset alone —
    fine for equipType, which (unlike materialCate) has no default-value
    ambiguity in real data."""
    offset = EQUIP_TYPE_ITEM_ID_OFFSET.get(category)
    if offset is None:
        return None
    item_id = raw_id + offset
    return item_id if item_id in items_by_id else None


def resolve_material_item_id(material_id: int, material_cate: int, items_by_id: dict) -> int | None:
    """Same conversion as resolve_item_id, but materialCate is not always
    corrected away from its PARAMDEF-shipped default (4/Gem) for
    non-equippable trade materials (confirmed 2026-07-25 empirically: 51/217
    real material slots have cate=4 but actually live at the Good offset —
    see docs/SHOP_LINEUP.md's "mtrlId resolution"). So: try every category
    offset and take the unique items.json hit; only use materialCate to pick
    among multiple hits (rare raw-id collisions across category spaces,
    where materialCate has always been the correct disambiguator so far)."""
    hits = [(off, material_id + off) for off in EQUIP_TYPE_ITEM_ID_OFFSET.values()
            if (material_id + off) in items_by_id]
    if not hits:
        return None
    if len(hits) == 1:
        return hits[0][1]
    cate_off = EQUIP_TYPE_ITEM_ID_OFFSET.get(material_cate)
    for off, item_id in hits:
        if off == cate_off:
            return item_id
    return hits[0][1]


def resolve_materials(mtrl_id: int, mtrl_rows_by_id: dict, mtrl_blob: bytes,
                       mtrl_schema: dict, items_by_id: dict) -> list[dict]:
    """mtrlId -> the list of {item_name, item_id, qty} required to buy the
    row, by decoding its EquipMtrlSetParam row's up-to-6 material slots."""
    if mtrl_id == -1:
        return []
    data_offset = mtrl_rows_by_id.get(mtrl_id)
    if data_offset is None:
        # Confirmed 2026-07-25: the only ids seen missing (901000-901004)
        # belong exclusively to unreachable debug ShopLineupParam rows.
        return [{"unresolved_mtrl_id": mtrl_id}]
    fields = decode_row_fields(mtrl_blob, data_offset, mtrl_schema)
    materials = []
    for i in range(1, MATERIAL_SLOT_COUNT + 1):
        n = f"{i:02d}"
        material_id = fields[f"materialId{n}"]
        if material_id == -1:
            continue
        cate = fields[f"materialCate{n}"]
        item_id = resolve_material_item_id(material_id, cate, items_by_id)
        materials.append({
            "item_name": items_by_id.get(item_id),
            "item_id": item_id,
            "qty": fields[f"itemNum{n}"],
        })
    return materials


def row_warnings(fields: dict, row_id: int | None = None) -> list[str]:
    warnings = []
    if any(fields.get(f, -1) != -1 for f in NAME_ICON_OVERRIDE_FIELDS):
        warnings.append("name/icon override: item-swap-inherits-display assumption doesn't hold")
    if fields.get("equipType") in UNRESOLVED_EQUIP_TYPES:
        warnings.append(f"equipType {fields['equipType']} has no known item-id offset")
    if fields.get("mtrlId", -1) != -1:
        warnings.append("requires a material exchange (see materials) in addition to/instead of the rune price")
    if fields.get("eventFlag_forRelease", 0) != 0:
        warnings.append(
            f"gated behind event flag {fields['eventFlag_forRelease']} "
            "(quest/boss-kill/bell-bearing progress) -- not available from the start"
        )
    if row_id in ENIA_FORGING_ROW_IDS:
        warnings.append(
            "likely an internal hand-in trigger for redeeming this Remembrance, not a "
            "player-visible for-sale row -- see ENIA_FORGING_ROW_IDS in this file"
        )
    return warnings


def decode_shop_row_enriched(row: dict, blob: bytes, schema: dict, row_names: dict,
                              items_by_id: dict, mtrl_rows_by_id: dict, mtrl_blob: bytes,
                              mtrl_schema: dict) -> dict:
    fields = decode_row_fields(blob, row["data_offset"], schema)
    name_entry = row_names.get(str(row["id"]), {})
    item_id = resolve_item_id(fields["equipId"], fields["equipType"], items_by_id)
    return {
        "row_id": row["id"],
        "merchant": name_entry.get("merchant"),
        "label": name_entry.get("item_label"),
        "item_name": items_by_id.get(item_id),
        "price": fields["value"] if fields["value"] != -1 else None,
        "cost_type": fields["costType"],
        "quantity": fields["sellQuantity"],
        "unlock_flag": fields["eventFlag_forRelease"],
        "stock_flag": fields["eventFlag_forStock"],
        "materials": resolve_materials(fields["mtrlId"], mtrl_rows_by_id, mtrl_blob, mtrl_schema, items_by_id),
        "warnings": row_warnings(fields, row["id"]),
        **fields,
    }


def cmd_list(args):
    bnd4 = _decoded_bnd4(args.save)
    for entry in sorted(list_bnd4_entries(bnd4), key=lambda e: e["name"]):
        print(f"{entry['size']:>10}  {entry['name']}")


def cmd_extract(args):
    bnd4 = _decoded_bnd4(args.save)
    data = extract_bnd4_entry(bnd4, args.entry)
    Path(args.outfile).write_bytes(data)
    print(f"wrote {len(data)} bytes to {args.outfile}", file=sys.stderr)


def _load_mtrl_context(bnd4: bytes) -> tuple[bytes, dict, dict]:
    mtrl_blob = extract_bnd4_entry(bnd4, "EquipMtrlSetParam.param")
    mtrl_header = parse_param_header(mtrl_blob)
    mtrl_schema = load_schema("equip_mtrl_set_schema.json")
    mtrl_rows_by_id = {r["id"]: r["data_offset"] for r in iter_param_rows(mtrl_blob, mtrl_header)}
    return mtrl_blob, mtrl_rows_by_id, mtrl_schema


def cmd_rows(args):
    bnd4 = _decoded_bnd4(args.save)
    blob = extract_bnd4_entry(bnd4, args.entry)
    header = parse_param_header(blob)
    schema = load_schema("shop_lineup_schema.json")
    row_names = load_schema("shop_row_names.json")
    items_by_id = {item["id"]: item["name"] for item in load_schema("items.json")}
    mtrl_blob, mtrl_rows_by_id, mtrl_schema = _load_mtrl_context(bnd4)

    print(f"# {header['param_type']}  rows={header['row_count']}  "
          f"paramdef_data_version={header['paramdef_data_version']}", file=sys.stderr)

    rows = iter_param_rows(blob, header)
    if args.limit:
        rows = rows[: args.limit]
    for row in rows:
        print(json.dumps(decode_shop_row_enriched(
            row, blob, schema, row_names, items_by_id, mtrl_rows_by_id, mtrl_blob, mtrl_schema)))


def cmd_merchants(args):
    bnd4 = _decoded_bnd4(args.save)
    blob = extract_bnd4_entry(bnd4, args.entry)
    header = parse_param_header(blob)
    schema = load_schema("shop_lineup_schema.json")
    row_names = load_schema("shop_row_names.json")
    items_by_id = {item["id"]: item["name"] for item in load_schema("items.json")}
    mtrl_blob, mtrl_rows_by_id, mtrl_schema = _load_mtrl_context(bnd4)

    rows = iter_param_rows(blob, header)
    enriched = [decode_shop_row_enriched(
        row, blob, schema, row_names, items_by_id, mtrl_rows_by_id, mtrl_blob, mtrl_schema)
        for row in rows]

    merchant_count = len({r["merchant"] for r in enriched if r["merchant"]})
    print(f"# {header['param_type']}  rows={header['row_count']}  merchants={merchant_count}",
          file=sys.stderr)

    enriched.sort(key=lambda r: (r["merchant"] is None, r["merchant"] or "", r["row_id"]))
    for r in enriched:
        print(json.dumps(r))


def cmd_find_item(args):
    items = load_schema("items.json")
    matches = find_items_by_name(args.name, items)
    if not matches:
        print(f"no item named {args.name!r}", file=sys.stderr)
        sys.exit(1)
    for item in matches:
        ref = equip_ref_for_item_id(item["id"])
        equip_type, equip_id = ref if ref else (None, None)
        print(json.dumps({**item, "equipType": equip_type, "equipId": equip_id}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="cmd", required=True)

    p_list = sub.add_parser("list", help="list every named entry in the regulation BND4 archive")
    p_list.add_argument("save")
    p_list.set_defaults(func=cmd_list)

    p_extract = sub.add_parser("extract", help="dump one named BND4 entry's raw bytes to a file")
    p_extract.add_argument("save")
    p_extract.add_argument("entry")
    p_extract.add_argument("outfile")
    p_extract.set_defaults(func=cmd_extract)

    p_rows = sub.add_parser("rows", help="decode every row of a shop-lineup-shaped param file, one JSON object per line")
    p_rows.add_argument("save")
    p_rows.add_argument("entry", nargs="?", default="ShopLineupParam.param")
    p_rows.add_argument("--limit", type=int, default=None)
    p_rows.set_defaults(func=cmd_rows)

    p_merchants = sub.add_parser("merchants", help="decode ShopLineupParam rows grouped by merchant, one JSON object per line")
    p_merchants.add_argument("save")
    p_merchants.add_argument("entry", nargs="?", default="ShopLineupParam.param")
    p_merchants.set_defaults(func=cmd_merchants)

    p_find_item = sub.add_parser("find-item", help="look up an item by name, print its equipType/equipId (for edits.json); no save file needed")
    p_find_item.add_argument("name")
    p_find_item.set_defaults(func=cmd_find_item)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
