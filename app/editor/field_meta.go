package editor

// costType -> price-field label mapping plus the shared change-description
// strings, used by the row edit form and the pending-edits bar so both
// describe a changed field identically. Ported from app/gui/field_meta.py.

import "fmt"

// fieldLabels maps a raw ShopLineupParam field name to its display label.
var fieldLabels = map[string]string{
	"value":                "Price",
	"sellQuantity":         "Quantity",
	"eventFlag_forRelease": "Unlock gate",
}

// namedCostTypes: costType 0 (runes) is the overwhelming majority and needs no
// callout. 2 = Starlight Shards, confirmed by all four browsable rows (Seluvis's
// puppets, values 2/5/3/5 = their exact in-game Starlight Shard prices).
// costTypes 3/4 occur only in excluded non-browsable blocks and keep the
// generic "costType N" fallback. See docs/SHOP_LINEUP.md's "costType" section.
var namedCostTypes = map[int64]string{
	1: "Dragon Hearts",
	2: "Starlight Shards",
	5: "Heart of Bayle",
}

// priceFieldLabel renders the price-input label for a given cost type.
func priceFieldLabel(costType int64) string {
	if costType == 0 {
		return "Price"
	}
	if name, ok := namedCostTypes[costType]; ok {
		return fmt.Sprintf("Price (%s)", name)
	}
	return fmt.Sprintf("Price (costType %d)", costType)
}

// runeIconPath is the merchant grid's price-footer icon for costType 0
// (runes, the overwhelming majority) -- visually near-identical to the
// game's own rune glyph (user: "the icon used for the runes is pretty
// similar to shadow_realm_rune_5.png, try to use this for it").
const runeIconPath = "items/tools/shadow_realm_rune_5.png"

// currencyIconPathByCostType maps a rare named cost type to its own real
// item icon (already vendored under data/icons, same as every other item
// icon) instead of a generic fallback -- used by the merchant grid's
// on-cell price footer.
var currencyIconPathByCostType = map[int64]string{
	1: "items/key_items/dragon_heart.png",   // Dragon Hearts
	2: "items/tools/starlight_shards.png",   // Starlight Shards
	5: "items/key_items/heart_of_bayle.png", // Heart of Bayle
}

// currencyIconPath resolves the price-footer icon for a row's cost type.
func currencyIconPath(costType int64) string {
	if p, ok := currencyIconPathByCostType[costType]; ok {
		return p
	}
	return runeIconPath
}

// describeChange renders one field change for the pending-edits bar, mirroring
// field_meta.describe_change's exact strings.
func describeChange(fieldName string, from, to, costType int64) string {
	if fieldName == "eventFlag_forRelease" {
		return fmt.Sprintf("Unlock gate: cleared (was flag %d)", from)
	}
	var label string
	if fieldName == "value" {
		label = priceFieldLabel(costType)
	} else if l, ok := fieldLabels[fieldName]; ok {
		label = l
	} else {
		label = fieldName
	}
	// The price field's -1 sentinel means "no price" (row.Price == nil), which
	// the Python renders as "-" rather than the raw -1.
	fromDisplay := fmt.Sprintf("%d", from)
	if fieldName == "value" && from == -1 {
		fromDisplay = "-"
	}
	return fmt.Sprintf("%s: %s -> %d", label, fromDisplay, to)
}
