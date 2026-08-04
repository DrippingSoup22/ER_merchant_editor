#!/usr/bin/env python3
"""Derive internal/assets/data/merchant_catalog.json: a curated row_id -> canonical-merchant
mapping, built on top of internal/assets/data/shop_row_names.json (Paramdex's raw Names
file, left untouched for reference/debugging) plus manual research findings
(see docs/MERCHANTS.md). Paramdex's raw `merchant` field conflates three
different things under similar-looking "Name - Suffix" strings: one NPC's
multiple unlock tiers (merge), genuinely distinct NPCs that happen to share
a naming convention (don't merge), and non-NPC mechanic pseudo-shops (not a
real merchant at all). This script is what reconciles that; nothing here is
re-derived from Paramdex automatically, it encodes decisions made in
docs/MERCHANTS.md.

Regenerate: python3 generate.py  (run from this directory; writes ../../internal/assets/data/)
"""

import json
from pathlib import Path

DATA_DIR = Path(__file__).resolve().parents[2] / "internal" / "assets" / "data"

# One NPC across every unlock tier -- Paramdex splits each into "Name - tier"
# per prayerbook/scroll/quest-stage. Collapse to the bare name.
MERGE_PREFIX_WHITELIST = [
    "Brother Corhyn", "Miriel", "Sorceress Sellen",
    "Preceptor Seluvis", "Twin Maiden Husks",
]

# Enia's remembrance-exchange counter is split three ways in Paramdex:
# "Enia - <Boss>" (armor sold outright), "Enia - Forging" (hand-in rows for
# the remembrance itself, row_id 101775-101792), and "Remembrance of <Boss>"
# / "Elden Remembrance" (the actual weapon/ash reward rows, row_id
# 101900-101948, confirmed contiguous with no gap/overlap against the Enia
# range). All one NPC.
def _is_enia(raw: str) -> bool:
    return (
        raw == "Enia"
        or raw.startswith("Enia - ")
        or raw.startswith("Remembrance of")
        or raw == "Elden Remembrance"
    )


# Confirmed via the game's own mechanics (Fextralife) -- these are places/
# mechanics, not people, so they're not "merchants" in the same sense as the
# rest of this file even though they're real, live, in-game trades.
SPECIAL_EXCHANGE_NAMES = {"Alteration", "Reversion"}

# Dragon Communion is 3 physically distinct altars, not one merchant --
# split 2026-07-31 after in-game testing showed the Grand Altar sells only
# its own 3 items, none of the base game's, contradicting the merged
# "Dragon Communion" bucket this used to be. Fextralife-confirmed
# item-for-item, zero overlap between altars (see docs/MERCHANTS.md):
# Church (3 incantations, Limgrave), Cathedral (13 incantations -- the
# Church's 3 plus more, Caelid), Grand Altar (Ghostflame Breath + Bayle's
# Tyranny/Flame Lightning, DLC). All 16 Church+Cathedral rows carry a raw
# Paramdex "Dragon Communion" label (matched by row id below, not by that
# label, since the label alone can't tell the two apart); the Grand Altar's
# 3 rows are mislabeled "Incantation" by Paramdex entirely.
DRAGON_COMMUNION_CHURCH_ROW_IDS = {101975, 101976, 101980}
DRAGON_COMMUNION_CATHEDRAL_ROW_IDS = set(range(101950, 101963))  # 101950-101962
DRAGON_COMMUNION_GRAND_ALTAR_ROW_IDS = {102350, 102351, 102355}

# Confirmed via Fextralife, item by item: every item these specific rows
# sell is actually obtained as a drop/pickup elsewhere in the game (Flame,
# Fall Upon Them: mini-boss reward; Frenzied Burst: hidden scarab; Stars of
# Ruin: a corpse pickup in Sellia Hideaway) and the remaining row just
# duplicates an already-real Sellen sale. No real NPC's talk-script opens
# this row range in actual play -- debug/test leftovers, not reachable.
#
# Corrected 2026-07-25 (previously misclassified as Dragon Communion, see
# docs/MERCHANTS.md): four identical 5-incantation blocks (Dragonfire/
# Agheel's Flame/Magma Breath/Dragonice/Rotten Breath, symbolic 1-5 rune
# prices, row_id blocks 900000/900010/900020/900030), all four sharing the
# exact same eventFlag_forStock per item (i.e. literally the same
# underlying stock state rendered 4 times, not 4 independent altars).
# Fextralife confirms only 3 real Dragon Communion altars exist (Church:
# 3 items, Cathedral: 13 items, Grand Altar: 3 items -- all already
# accounted for by the DRAGON_COMMUNION_*_ROW_IDS sets above), and two of
# these five items (Agheel's Flame,
# Magma Breath) show as unconditionally available here despite being
# confirmed dragon-kill-gated at the real Cathedral -- a direct
# contradiction if this were live stock. Stale/superseded duplicate rows
# from an earlier shop-table revision, not reachable in the current game.
_STALE_DRAGON_COMMUNION_DUPLICATE_ROW_IDS = {
    900000, 900001, 900002, 900003, 900004,
    900010, 900011, 900012, 900013, 900014,
    900020, 900021, 900022, 900023, 900024,
    900030, 900031, 900032, 900033, 900034,
}
EXCLUDED_ROW_IDS = {1600404, 1600405, 1600407, 1600408} | _STALE_DRAGON_COMMUNION_DUPLICATE_ROW_IDS

# SotE shop NPCs (mapped 2026-07-28) -- Paramdex's Names file gives DLC rows
# no "[Merchant]" prefix at all (base-game rows have one; DLC rows are bare
# item names or generic "[Sorcery]"), so these are attributed by row-id
# range. Stock verified item-for-item against eldenring.wiki.gg: Moore's 17
# items and Count Ymir's 9 sorceries (incl. his two bell-quest tiers) match
# exactly; Thiollier's block is self-identifying (Thiollier's Concoction).
DLC_MERCHANT_ROW_RANGES = [
    (102250, 102266, "Moore"),
    (102270, 102282, "Thiollier"),
    (102300, 102308, "Count Ymir"),
]

# Real, currently-purchasable rows that we can't confidently attribute to
# one specific NPC. Kept visible/editable, just honestly unlabeled rather
# than guessed. (Empty in practice since the Count Ymir range above claimed
# the ex-"Sorcery" rows -- kept as the fallback for future surprises.)
UNKNOWN_MERCHANT_RAW_NAMES = {"Incantation", "Sorcery"}

# Confirmed real attribution bugs in Paramdex's raw Names file (2026-07-26),
# found via a systematic per-merchant Fextralife cross-check (see
# docs/MERCHANTS.md's "Row-level misattributions" section). Applied before
# every other rule so the raw label never wins for these specific rows.
ROW_REASSIGNMENTS = {
    # DLC "Remembrance of the Dancing Lion" reward pair -- reused free row
    # slots at the tail of Twin Maiden Husks' block (101800-101899) instead
    # of extending Enia's own 101900-101948 reward range. Same price=0 +
    # mtrlId=a-Remembrance signature as every other Enia reward row.
    101898: "Enia",  # Enraged Divine Beast (talisman)
    101899: "Enia",  # Divine Beast Frost Stomp (Ash of War)
    # Sorcerer Thops's own trio (Glintstone Pebble/Arc/Starlight, 1000/1500/
    # 2500 runes) -- Paramdex has no "Sorcerer Thops" entries anywhere and
    # mislabels these as Preceptor Seluvis. Fextralife's Starlight page
    # states outright it is not sold by Seluvis; prices match Thops's known
    # lineup exactly. Sits in its own row block (100250-100252) with a gap
    # before Seluvis's real stock (100275+) -- a reused slot range, same
    # failure mode as the TMH case above.
    100250: "Sorcerer Thops",  # Glintstone Pebble
    100251: "Sorcerer Thops",  # Glintstone Arc
    100252: "Sorcerer Thops",  # Starlight
}

# Enia's "Forging" rows (Paramdex's own raw label) sell the Remembrance
# item itself at price 0/no material, each with a unique per-boss unlock
# flag unlike every other Enia row -- confirmed (2026-07-25, see
# docs/SHOP_LINEUP.md) you cannot buy back a Remembrance from any merchant
# after redeeming it, so these can't be real player-visible for-sale rows;
# best-supported theory is they're the engine's internal trigger for the
# hand-in dialogue action itself. Kept out of the browsable grid (`kind`
# "internal", not "merchant") but NOT excluded/hidden from the underlying
# data -- still fully resolvable via savescan.py directly. Keep in sync
# with ENIA_FORGING_ROW_IDS in app/savescan.py (duplicated rather than
# imported -- both scripts are meant to stay standalone/dependency-free).
ENIA_FORGING_ROW_IDS = {
    101775, 101776, 101777, 101778, 101779, 101780, 101781,
    101785, 101786, 101787, 101788, 101789, 101790, 101791, 101792,
}

# Paramdex's generic "Merchant - <Location>" labels omit the actual merchant
# archetype. Use the game's stock identity in every user-facing list: 10 are
# Nomadic, Ainsel River and Mountaintops are Hermit, Siofra River is Abandoned,
# and Raya Lucaria is Isolated (matched item-for-item to its real shop).
MERCHANT_RENAMES = {
    "Merchant - Academy of Raya Lucaria": "Isolated Merchant - Academy of Raya Lucaria",
    "Merchant - Ainsel River": "Hermit Merchant - Ainsel River",
    "Merchant - Altus Plateau": "Nomadic Merchant - Altus Plateau",
    "Merchant - Caelid (Aeonia Swamp)": "Nomadic Merchant - Caelid (Aeonia Swamp)",
    "Merchant - Coastal Cave": "Nomadic Merchant - Coastal Cave",
    "Merchant - East Limgrave": "Nomadic Merchant - East Limgrave",
    "Merchant - East Weeping Peninsula": "Nomadic Merchant - East Weeping Peninsula",
    "Merchant - Liurnia of the Lakes": "Nomadic Merchant - Liurnia of the Lakes",
    "Merchant - Mountaintops of the Giants": "Hermit Merchant - Mountaintops of the Giants",
    "Merchant - Mt. Gelmir": "Nomadic Merchant - Mt. Gelmir",
    "Merchant - North Limgrave": "Nomadic Merchant - North Limgrave",
    "Merchant - North Liurnia": "Nomadic Merchant - North Liurnia",
    "Merchant - Siofra River": "Abandoned Merchant - Siofra River",
    "Merchant - South Caelid": "Nomadic Merchant - South Caelid",
}


def canonicalize(row_id: int, raw_merchant: str | None) -> dict:
    if row_id in ROW_REASSIGNMENTS:
        return {"merchant": ROW_REASSIGNMENTS[row_id], "kind": "merchant"}
    # Before the None check: DLC rows have no raw merchant name at all.
    for lo, hi, name in DLC_MERCHANT_ROW_RANGES:
        if lo <= row_id <= hi:
            return {"merchant": name, "kind": "merchant"}
    if raw_merchant is None:
        return {"merchant": None, "kind": "unnamed"}
    if row_id in EXCLUDED_ROW_IDS:
        return {"merchant": None, "kind": "excluded"}
    if row_id in ENIA_FORGING_ROW_IDS:
        return {"merchant": "Enia", "kind": "internal"}
    if row_id in DRAGON_COMMUNION_CHURCH_ROW_IDS:
        return {"merchant": "Church of Dragon Communion", "kind": "special_exchange"}
    if row_id in DRAGON_COMMUNION_CATHEDRAL_ROW_IDS:
        return {"merchant": "Cathedral of Dragon Communion", "kind": "special_exchange"}
    if row_id in DRAGON_COMMUNION_GRAND_ALTAR_ROW_IDS:
        return {"merchant": "Grand Altar of Dragon Communion", "kind": "special_exchange"}
    if raw_merchant in SPECIAL_EXCHANGE_NAMES:
        return {"merchant": raw_merchant, "kind": "special_exchange"}
    if _is_enia(raw_merchant):
        return {"merchant": "Enia", "kind": "merchant"}
    for prefix in MERGE_PREFIX_WHITELIST:
        if raw_merchant == prefix or raw_merchant.startswith(prefix + " - "):
            return {"merchant": prefix, "kind": "merchant"}
    if raw_merchant in UNKNOWN_MERCHANT_RAW_NAMES:
        return {"merchant": None, "kind": "unknown_merchant"}
    canonical = MERCHANT_RENAMES.get(raw_merchant, raw_merchant)
    return {"merchant": canonical, "kind": "merchant"}


def main():
    names = json.loads((DATA_DIR / "shop_row_names.json").read_text())

    out = {}
    for row_id_str, entry in names.items():
        out[row_id_str] = canonicalize(int(row_id_str), entry["merchant"])

    (DATA_DIR / "merchant_catalog.json").write_text(json.dumps(out, indent=2, sort_keys=True) + "\n")

    kinds = {}
    merchants = set()
    for v in out.values():
        kinds[v["kind"]] = kinds.get(v["kind"], 0) + 1
        if v["kind"] == "merchant":
            merchants.add(v["merchant"])
    print(f"wrote {len(out)} row entries, {len(merchants)} canonical merchants")
    print("by kind:", kinds)


if __name__ == "__main__":
    main()
