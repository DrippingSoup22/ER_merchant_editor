package savefile

// JSON row-schema loading: the paramdex-extracted field layout for
// ShopLineupParam and the 5 EquipParam* tables, plus the type-width table
// encode/decode (pipeline_param.go / decode.go) share.

import (
	"encoding/json"
	"fmt"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/assets/data"
)

// STRUCT_FMT equivalent: type -> (byte width). Endianness is little-endian for
// all inner PARAM field storage (mirrors savescan.py STRUCT_FMT).
var typeWidth = map[string]int{
	"s8": 1, "u8": 1,
	"s16": 2, "u16": 2,
	"s32": 4, "u32": 4, "b32": 4, "angle32": 4,
	"f32": 4, "f64": 8,
}

type SchemaField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	ArrayLength int    `json:"array_length"`
	Offset      int    `json:"offset"`
	Size        int    `json:"size"`
}

type ShopSchema struct {
	ParamType string        `json:"param_type"`
	RowSize   int           `json:"row_size"`
	Fields    []SchemaField `json:"fields"`
	byName    map[string]SchemaField
}

// Field looks up a schema field by name.
func (s *ShopSchema) Field(name string) (SchemaField, bool) {
	f, ok := s.byName[name]
	return f, ok
}

// ParseSchema parses a paramdex-extracted schema JSON (shop_lineup_schema.json
// or equip_mtrl_set_schema.json — same shape).
func ParseSchema(raw []byte) (*ShopSchema, error) {
	var s ShopSchema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	s.byName = make(map[string]SchemaField, len(s.Fields))
	for _, f := range s.Fields {
		s.byName[f.Name] = f
	}
	return &s, nil
}

// LoadShopSchema loads the embedded ShopLineupParam row schema.
func LoadShopSchema() (*ShopSchema, error) {
	raw, err := data.FS.ReadFile("shop_lineup_schema.json")
	if err != nil {
		return nil, err
	}
	s, err := ParseSchema(raw)
	if err != nil {
		return nil, fmt.Errorf("shop_lineup_schema.json: %w", err)
	}
	return s, nil
}

// equipParamSchemaFiles/equipParamEntryNames key an EquipParam* table by
// equipType (0-4), the exact same convention ShopLineupParam's own
// equipType field already uses -- see docs/MERCHANT_DATA.md's 2026-07-30
// "cost=0" entry for why these are needed: a row's item has its own
// sellValue in a completely separate table from ShopLineupParam.
var equipParamSchemaFiles = map[int64]string{
	0: "equip_param_weapon_schema.json",
	1: "equip_param_protector_schema.json",
	2: "equip_param_accessory_schema.json",
	3: "equip_param_goods_schema.json",
	4: "equip_param_gem_schema.json",
}

var equipParamEntryNames = map[int64]string{
	0: "EquipParamWeapon.param",
	1: "EquipParamProtector.param",
	2: "EquipParamAccessory.param",
	3: "EquipParamGoods.param",
	4: "EquipParamGem.param",
}

// LoadEquipParamSchema loads the embedded row schema for one of the 5
// EquipParam* tables, keyed by equipType.
func LoadEquipParamSchema(equipType int64) (*ShopSchema, error) {
	name, ok := equipParamSchemaFiles[equipType]
	if !ok {
		return nil, fmt.Errorf("unknown equipType %d", equipType)
	}
	raw, err := data.FS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	s, err := ParseSchema(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return s, nil
}

// EquipParamEntryName resolves the BND4 entry name for an EquipParam*
// table, keyed by equipType.
func EquipParamEntryName(equipType int64) (string, bool) {
	name, ok := equipParamEntryNames[equipType]
	return name, ok
}
