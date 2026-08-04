// Package data embeds the JSON data files the editor and shopwrite need at
// runtime. Embedding (instead of locating a data/ directory on disk) is what
// makes every built binary standalone. Icons are deliberately NOT here: they
// live in the root-level assets package so the shopwrite CLI doesn't carry
// ~80MB of PNGs it never uses (go:embed variables are never dead-code
// eliminated).
package data

import "embed"

//go:embed shop_lineup_schema.json shop_row_names.json equip_mtrl_set_schema.json items.json merchant_catalog.json weapon_reinforce.json weapon_reinforce_rates.json vanilla_shop_lineup.json item_sort_order.json item_details.json
//go:embed equip_param_weapon_schema.json equip_param_protector_schema.json equip_param_accessory_schema.json equip_param_goods_schema.json equip_param_gem_schema.json
var FS embed.FS
