# Merchant identity

`shop_row_names.json` mirrors Paramdex labels and is not a reliable merchant
model: it splits one NPC by quest tier, conflates similarly named NPCs, and
includes mechanics or unreachable rows. Runtime code must use the generated
`merchant_catalog.json` mapping instead.

Each mapped row has one kind:

- `merchant`: stock owned by a real NPC;
- `special_exchange`: a non-NPC trade such as Dragon Communion;
- `unknown_merchant`: real stock without a defensible owner;
- `excluded`: debug, duplicate, or unreachable content;
- `unnamed`: no Paramdex name entry.

Only real merchants, unknown real stock, and the three supported Dragon
Communion altars are browsable.

## Normalization rules

- Quest/scroll/prayerbook suffixes for Corhyn, Miriel, Sellen, Seluvis, and
  Twin Maiden Husks collapse to one NPC.
- Location suffixes remain when they distinguish wandering merchants, such as
  Nomadic, Hermit, Isolated, Imprisoned, and Abandoned merchants.
- Enia armor, forging, and Remembrance reward rows share one merchant identity.
  Her material-priced rows cannot be edited, and her release flags cannot be
  changed; see [CHAR_UNLOCK.md](CHAR_UNLOCK.md).
- Alteration/Reversion rows are armor mechanics and stay hidden.
- Church, Cathedral, and Grand Altar of Dragon Communion are separate
  browsable exchanges because their inventories and currencies differ.
- DLC ranges absent from Paramdex naming are explicitly assigned to Moore,
  Thiollier, and Count Ymir.

Known row corrections live as data-generation rules, not runtime exceptions:

- `101898-101899`: Enia, not Twin Maiden Husks;
- `100250-100252`: Sorcerer Thops, not Seluvis;
- Raya Lucaria's generic merchant block: Isolated Merchant.

Do not infer ownership from row proximity or labels alone. Update
`tools/merchant_catalog/generate.py`, cite the evidence in its comments, and
regenerate:

```sh
python3 tools/merchant_catalog/generate.py
```

The generator reads only local `shop_row_names.json`; it performs no network
access. `MerchantSortKey` in `internal/catalog` owns display grouping and is
covered by ordering tests.
