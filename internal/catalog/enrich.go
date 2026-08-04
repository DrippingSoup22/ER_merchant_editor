package catalog

// Row enrichment: decode a ShopLineupParam row and resolve item name, price,
// materials (via EquipMtrlSetParam), and data-quality warnings. Ported field-
// for-field from savescan.py (decode_shop_row_enriched, resolve_materials,
// row_warnings) and catalog.py (the category/icon/material_locked additions).

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/assets/data"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/savefile"
)

const materialSlotCount = 6

// nameIconOverrideFields: non-(-1) here means item-swap can't inherit display.
var nameIconOverrideFields = []string{"iconId", "nameMsgId", "menuTitleMsgId", "menuIconId"}

// unresolvedEquipTypes have no known item-id offset.
var unresolvedEquipTypes = map[int64]bool{5: true, 6: true}

// eniaForgingRowIDs: Enia "Forging" hand-in rows (see savescan.py). 15 rows,
// 101775-101792 minus gaps.
var eniaForgingRowIDs = func() map[int64]bool {
	m := map[int64]bool{}
	for _, id := range []int64{
		101775, 101776, 101777, 101778, 101779, 101780, 101781,
		101785, 101786, 101787, 101788, 101789, 101790, 101791, 101792,
	} {
		m[id] = true
	}
	return m
}()

// Material is one required material to buy a row. UnresolvedMtrlID != 0 marks
// savescan's {"unresolved_mtrl_id": N} case (a mtrlId with no EquipMtrlSetParam
// row); ItemName/ItemID/Qty are then unset.
type Material struct {
	ItemName         string `json:"item_name"`
	ItemID           int64  `json:"item_id"`
	Qty              int64  `json:"qty"`
	UnresolvedMtrlID int64  `json:"unresolved_mtrl_id,omitempty"`
}

// Row is a fully enriched ShopLineupParam row. Merchant/Label come from the raw
// Paramdex Names file (shop_row_names.json); canonical merchant identity is
// applied separately (merchant_catalog.json) in ListMerchants/MerchantRows.
type Row struct {
	RowID          int64
	Merchant       string
	Label          string
	ItemName       string
	ItemID         *int64
	IconPath       string
	Category       string
	SubCategory    string
	Price          *int64 // nil when the raw value field == -1
	CostType       int64
	Quantity       int64
	UnlockFlag     int64
	StockFlag      int64
	Materials      []Material
	Warnings       []string
	MaterialLocked bool
	Level          int64 // real reinforcement level ("+N") resolved from the raw equipId, 0 if unleveled or not a weapon
	Fields         map[string]int64
}

// CharacterGate exposes only the row identity and event flag needed by the
// character-unlock core. The structural interface keeps the character package
// independent from catalog models.
func (r *Row) CharacterGate() (rowID, flagID int64) {
	return r.RowID, r.UnlockFlag
}

// DisplayName is ItemName with a "+N" reinforcement suffix when Level > 0 --
// mirrors ItemChange.DisplayName()'s convention for staged edits, so a row's
// displayed name doesn't change look depending on whether the level edit is
// still staged or already saved and reloaded.
func (r *Row) DisplayName() string {
	if r.Level > 0 {
		return fmt.Sprintf("%s +%d", r.ItemName, r.Level)
	}
	return r.ItemName
}

// rowName is one shop_row_names.json entry.
type rowName struct {
	Merchant  string `json:"merchant"`
	ItemLabel string `json:"item_label"`
}

// mtrlContext bundles the EquipMtrlSetParam table needed to resolve materials.
type mtrlContext struct {
	blob     []byte
	rowsByID map[int64]int // row id -> data offset within blob
	schema   *savefile.ShopSchema
}

// loadRowNames converts shop_row_names.json's native string row-id keys to
// int64 once at load time (mirrors loadVanillaShopLineup), so enrichRow's
// per-row name lookup is a direct int64 map hit rather than an fmt.Sprintf
// allocation on every decoded row.
func loadRowNames() (map[int64]rowName, error) {
	raw, err := data.FS.ReadFile("shop_row_names.json")
	if err != nil {
		return nil, err
	}
	var m map[string]rowName
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := make(map[int64]rowName, len(m))
	for k, v := range m {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			continue
		}
		out[id] = v
	}
	return out, nil
}

func loadMtrlSchema() (*savefile.ShopSchema, error) {
	raw, err := data.FS.ReadFile("equip_mtrl_set_schema.json")
	if err != nil {
		return nil, err
	}
	return savefile.ParseSchema(raw)
}

// loadMtrlContext extracts and indexes EquipMtrlSetParam.param from a decoded
// BND4 archive. Ported from savescan.py _load_mtrl_context.
func loadMtrlContext(bnd4 []byte, schema *savefile.ShopSchema) (*mtrlContext, error) {
	_, blob, _, rows, err := savefile.LoadParamRows(bnd4, "EquipMtrlSetParam.param")
	if err != nil {
		return nil, err
	}
	rowsByID := make(map[int64]int, len(rows))
	for _, r := range rows {
		rowsByID[int64(r.ID)] = r.DataOffset
	}
	return &mtrlContext{blob: blob, rowsByID: rowsByID, schema: schema}, nil
}

// resolveMaterials decodes a mtrlId's up-to-6 material slots. Ported from
// savescan.py resolve_materials.
func resolveMaterials(mtrlID int64, mtrl *mtrlContext, itemsByID map[int64]*Item) ([]Material, error) {
	if mtrlID == -1 {
		return nil, nil
	}
	dataOffset, ok := mtrl.rowsByID[mtrlID]
	if !ok {
		return []Material{{UnresolvedMtrlID: mtrlID}}, nil
	}
	fields, err := savefile.DecodeRowFields(mtrl.blob, dataOffset, mtrl.schema)
	if err != nil {
		return nil, err
	}
	var out []Material
	for i := 1; i <= materialSlotCount; i++ {
		n := fmt.Sprintf("%02d", i)
		materialID := fields["materialId"+n]
		if materialID == -1 {
			continue
		}
		cate := fields["materialCate"+n]
		m := Material{Qty: fields["itemNum"+n]}
		if itemID, ok := resolveMaterialItemID(materialID, int(cate), itemsByID); ok {
			m.ItemID = itemID
			if it := itemsByID[itemID]; it != nil {
				m.ItemName = it.Name
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// DisplayOverrides returns the row's name/icon override fields that are set
// (non--1): fields that pin the shop-menu display regardless of equipId. An
// item swap must reset these to -1 or the menu keeps showing the old item's
// name/icon (the editor stages that reset automatically with every swap).
func (r *Row) DisplayOverrides() []string {
	var out []string
	for _, f := range nameIconOverrideFields {
		if r.Fields[f] != -1 {
			out = append(out, f)
		}
	}
	return out
}

// Warning-text prefixes, exported so a caller classifying warnings (e.g.
// the editor deciding which ones still matter once its own character-aware
// state already covers the same condition -- see merchant_panel.go's
// hazardWarnings) can match against the literal text rowWarnings below
// actually generates, instead of hand-copying a second literal that could
// silently drift out of sync with it.
const (
	WarnPrefixNameIconOverride = "name/icon override"
	WarnPrefixMaterialExchange = "requires a material exchange"
	WarnPrefixEventGated       = "gated behind event flag"
)

// rowWarnings ports savescan.py row_warnings (same order, exact strings).
func rowWarnings(fields map[string]int64, rowID int64) []string {
	var warnings []string
	for _, f := range nameIconOverrideFields {
		if fields[f] != -1 {
			warnings = append(warnings, WarnPrefixNameIconOverride+": item-swap-inherits-display assumption doesn't hold")
			break
		}
	}
	if unresolvedEquipTypes[fields["equipType"]] {
		warnings = append(warnings, fmt.Sprintf("equipType %d has no known item-id offset", fields["equipType"]))
	}
	if fields["mtrlId"] != -1 {
		warnings = append(warnings, WarnPrefixMaterialExchange+" (see materials) in addition to/instead of the rune price")
	}
	if fields["eventFlag_forRelease"] != 0 {
		warnings = append(warnings, fmt.Sprintf(
			WarnPrefixEventGated+" %d (quest/boss-kill/bell-bearing progress) -- not available from the start",
			fields["eventFlag_forRelease"]))
	}
	if eniaForgingRowIDs[rowID] {
		warnings = append(warnings,
			"likely an internal hand-in trigger for redeeming this Remembrance, not a "+
				"player-visible for-sale row -- see ENIA_FORGING_ROW_IDS in this file")
	}
	return warnings
}

// enrichRow decodes and enriches one ShopLineupParam row. Ported from
// savescan.py decode_shop_row_enriched, with catalog.py's item_id/category/
// subCategory/iconPath/material_locked additions folded in.
func (c *Catalog) enrichRow(row savefile.ParamRow, blob []byte, schema *savefile.ShopSchema, mtrl *mtrlContext) (*Row, error) {
	fields, err := savefile.DecodeRowFields(blob, row.DataOffset, schema)
	if err != nil {
		return nil, err
	}
	rid := int64(row.ID)
	name := c.rowNames[rid]

	var itemID *int64
	var itemName, iconPath, category, subCategory string
	var level int64
	if id, lvl, ok := c.resolveItemIDWithLevel(fields["equipId"], int(fields["equipType"])); ok {
		idCopy := id
		itemID = &idCopy
		level = lvl
		if it := c.itemByID[id]; it != nil {
			itemName, iconPath = it.Name, it.IconPath
			category, subCategory = it.Category, it.SubCategory
		}
	}

	var price *int64
	if v := fields["value"]; v != -1 {
		vCopy := v
		price = &vCopy
	}

	materials, err := resolveMaterials(fields["mtrlId"], mtrl, c.itemByID)
	if err != nil {
		return nil, err
	}

	return &Row{
		RowID:          rid,
		Merchant:       name.Merchant,
		Label:          name.ItemLabel,
		ItemName:       itemName,
		ItemID:         itemID,
		IconPath:       iconPath,
		Category:       category,
		SubCategory:    subCategory,
		Price:          price,
		CostType:       fields["costType"],
		Quantity:       fields["sellQuantity"],
		UnlockFlag:     fields["eventFlag_forRelease"],
		StockFlag:      fields["eventFlag_forStock"],
		Materials:      materials,
		Warnings:       rowWarnings(fields, rid),
		MaterialLocked: fields["mtrlId"] != -1,
		Level:          level,
		Fields:         fields,
	}, nil
}
