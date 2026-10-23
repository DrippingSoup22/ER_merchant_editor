"""One PARAM table of a save, for the generators.

The save itself is read by ER_shared through `go run ./tools/paramdump`, so the
generators need nothing beyond the standard library.
"""
import base64
import json
import struct
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DATA_DIR = ROOT / "internal" / "assets" / "data"

STRUCT_FMT = {
    "s8": "<b", "u8": "<B",
    "s16": "<h", "u16": "<H",
    "s32": "<i", "u32": "<I", "b32": "<i", "angle32": "<f",
    "f32": "<f", "f64": "<d",
}


def load_param(save_path, entry):
    """The table's bytes and its rows, [{"id", "data_offset"}], in table order."""
    out = subprocess.run(
        ["go", "run", "./tools/paramdump", "-save", str(Path(save_path).resolve()), "-entry", entry],
        cwd=ROOT, capture_output=True, text=True, check=True)
    table = json.loads(out.stdout)
    return base64.b64decode(table["blob"]), table["rows"]


def load_schema(name):
    return json.loads((DATA_DIR / name).read_text())


def decode_row_fields(blob, data_offset, schema):
    values = {}
    for field in schema["fields"]:
        if field["type"] == "dummy8":
            continue
        fmt = STRUCT_FMT[field["type"]]
        values[field["name"]] = struct.unpack_from(fmt, blob, data_offset + field["offset"])[0]
    return values
