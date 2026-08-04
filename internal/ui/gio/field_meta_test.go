package gio

// Table tests for describeChange and priceFieldLabel (currencyIconPath is
// covered by TestCurrencyIconPath in merchant_panel_test.go).

import "testing"

// TestPriceFieldLabel: costType 0 is the plain "Price", a named cost type
// gets its "(Name)" suffix, and an unknown cost type falls back to the
// generic "(costType N)" rather than guessing.
func TestPriceFieldLabel(t *testing.T) {
	cases := []struct {
		costType int64
		want     string
	}{
		{0, "Price"},
		{1, "Price (Dragon Hearts)"},
		{2, "Price (Starlight Shards)"},
		{5, "Price (Heart of Bayle)"},
		{3, "Price (costType 3)"},   // occurs only in excluded blocks -> generic fallback
		{99, "Price (costType 99)"}, // unknown -> generic fallback
	}
	for _, c := range cases {
		if got := priceFieldLabel(c.costType); got != c.want {
			t.Errorf("priceFieldLabel(%d) = %q, want %q", c.costType, got, c.want)
		}
	}
}

// TestDescribeChange covers each branch: the eventFlag_forRelease special
// case, the price -1 sentinel rendering as "-", the named/unknown cost-type
// price labels, the known non-price field label, and the unknown-field
// fallback to the raw field name.
func TestDescribeChange(t *testing.T) {
	cases := []struct {
		name              string
		fieldName         string
		from, to, costTyp int64
		want              string
	}{
		{"unlock gate cleared", "eventFlag_forRelease", 12345, 0, 0, "Unlock gate: cleared (was flag 12345)"},
		{"plain rune price", "value", 100, 200, 0, "Price: 100 -> 200"},
		{"named cost-type price", "value", 5, 10, 2, "Price (Starlight Shards): 5 -> 10"},
		{"unknown cost-type price", "value", 1, 2, 7, "Price (costType 7): 1 -> 2"},
		{"price -1 sentinel shows as dash", "value", -1, 500, 0, "Price: - -> 500"},
		{"price -1 sentinel with named cost type", "value", -1, 3, 1, "Price (Dragon Hearts): - -> 3"},
		{"known non-price field", "sellQuantity", 5, -1, 0, "Quantity: 5 -> -1"},
		{"unknown field falls back to raw name", "someField", 1, 2, 0, "someField: 1 -> 2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := describeChange(c.fieldName, c.from, c.to, c.costTyp); got != c.want {
				t.Errorf("describeChange(%q, %d, %d, %d) = %q, want %q",
					c.fieldName, c.from, c.to, c.costTyp, got, c.want)
			}
		})
	}
}
