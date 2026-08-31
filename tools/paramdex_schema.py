"""Shared PARAMDEF schema builder for soulsmods/Paramdex Defs XML, used by
every tools/*_extract generator that needs to compute a table's row layout.
Stdlib only. See docs/SHOP_LINEUP.md for the field-offset-from-field-order
approach and docs/ITEM_IDS.md for the bitfield-grouping gotcha this fixed.
"""

import re
import os
import urllib.request
import xml.etree.ElementTree as ET
from pathlib import Path

TYPE_SIZES = {
    "s8": 1, "u8": 1, "dummy8": 1,
    "s16": 2, "u16": 2,
    "s32": 4, "u32": 4, "f32": 4, "b32": 4, "angle32": 4,
    "f64": 8,
}

FIELD_DEF_RE = re.compile(
    r"^(?P<type>\w+)\s+(?P<name>\w+)(\[(?P<arraylen>\d+)\])?(\s*=\s*(?P<default>-?[\d.]+))?$"
)
BITFIELD_DEF_RE = re.compile(r"^(?P<type>\w+)\s+(?P<name>\w+):(?P<bits>\d+)(\s*=\s*-?[\d.]+)?$")


def fetch(url: str) -> str:
    # Prefer the workspace's pinned checkout when supplied. This keeps data
    # refreshes reproducible and avoids silently following Paramdex master.
    root = os.environ.get("ER_PARAMDEX_ROOT")
    marker = "/Paramdex/master/"
    if root and marker in url:
        relative = url.split(marker, 1)[1]
        return (Path(root) / relative).read_text(encoding="utf-8-sig")
    with urllib.request.urlopen(url) as resp:
        return resp.read().decode("utf-8-sig")


def build_schema(xml_text: str) -> dict:
    """Field offsets computed by us from field order + type sizes (not
    scraped). Bitfields (`type name:bits`) aren't individually addressable
    fields we need, so consecutive ones sharing one storage byte are
    collapsed into a single `dummy8`-typed placeholder sized to the whole-
    byte space they occupy, so offset math for the fields after them stays
    correct.

    A bitfield group's real "anchor" type is whatever non-`dummy8` type
    started it (`u8 realField:1`); reserved/unused trailing bits are
    conventionally declared as `dummy8 someReserve:N` regardless of the
    anchor type (confirmed 2026-07-28 against EquipParamWeapon's
    `u8 disableParam_NT:1` + `dummy8 disableParamReserve1:7`, which
    together fill exactly one byte on disk — grouping strictly by exact
    type match mis-split this into two separate padding bytes, a 2-byte
    misalignment confirmed via real row spacing in our own fixture's
    EquipParamWeapon.param, 664 bytes vs. the wrongly-computed 666). So
    `dummy8` is treated as compatible with any group, joining it as long as
    the group's bit-width capacity (8 * the anchor type's byte size) isn't
    exceeded; a differently-typed non-`dummy8` bitfield starts a new group.
    """
    root = ET.fromstring(xml_text)
    fields = []
    offset = 0
    group_type = None
    group_width = 0  # bits
    bits_used = 0

    def flush():
        nonlocal offset, group_type, group_width, bits_used
        if group_type is None:
            return
        size = group_width // 8
        fields.append({
            "name": f"_bitfield_pad@{offset}",
            "type": "dummy8",
            "array_length": size,
            "offset": offset,
            "size": size,
        })
        offset += size
        group_type, group_width, bits_used = None, 0, 0

    for field_el in root.find("Fields"):
        def_str = field_el.get("Def").strip()
        bm = BITFIELD_DEF_RE.match(def_str)
        if bm:
            btype = bm.group("type")
            bits = int(bm.group("bits"))
            this_width = TYPE_SIZES[btype] * 8
            compatible = group_type is not None and (
                btype == "dummy8" or group_type == "dummy8" or btype == group_type
            )
            if compatible and bits_used + bits <= group_width:
                bits_used += bits
            else:
                flush()
                group_type, group_width, bits_used = btype, this_width, bits
            continue
        flush()
        m = FIELD_DEF_RE.match(def_str)
        if not m:
            raise ValueError(f"unparsed field def: {def_str!r}")
        type_ = m.group("type")
        name = m.group("name")
        array_len = int(m.group("arraylen") or 1)
        size = TYPE_SIZES[type_] * array_len
        fields.append({
            "name": name,
            "type": type_,
            "array_length": array_len,
            "offset": offset,
            "size": size,
        })
        offset += size
    flush()
    return {"param_type": root.find("ParamType").text, "row_size": offset, "fields": fields}
