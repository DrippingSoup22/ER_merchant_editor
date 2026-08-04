// Package catalog is the read side of the Elden Ring merchant save editor:
// decode a save's ShopLineupParam rows, resolve item/merchant/material context,
// and expose merchant/item queries plus an in-process edit-apply. Ported from
// app/savescan.py + app/catalog.py; write-back is delegated to shopwrite.Apply
// (called in-process, not as a subprocess).
package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"er_merchant_editor/app/shopwrite"
	"er_merchant_editor/data"
)

const shopEntry = "ShopLineupParam.param"

// browsableSpecialExchanges: of the special_exchange pseudo-shops, only the
// 3 Dragon Communion altars (see docs/MERCHANTS.md) are real browsable lists.
var browsableSpecialExchanges = map[string]bool{
	"Church of Dragon Communion":      true,
	"Cathedral of Dragon Communion":   true,
	"Grand Altar of Dragon Communion": true,
}

// tmhShop1Rank and tmhShop2Rank keep the named base-game sellers in the two
// Twin Maiden Husks submenus. Their values follow the in-game submenu order.
// Miriel and Gowry each appear twice there (their Sorcery and Incantations
// wares are separate entries), but have one merchant identity in the editor,
// so each occupies its first in-game position here.
var tmhShop1Rank = map[string]int{
	"Sorceress Sellen":     1,
	"Preceptor Seluvis":    2,
	"Sorcerer Thops":       3,
	"Brother Corhyn":       4,
	"Miriel":               5,
	"D Hunter of The Dead": 6,
	"Gowry":                7,
	"Sorcerer Rogier":      8,
	"Knight Bernahl":       9,
	"Iji":                  10,
}

var tmhShop2Rank = map[string]int{
	"Gatekeeper Gostoc":      1,
	"Pidia Carian Servant":   2,
	"Patches":                3,
	"Blackguard Big Boggart": 4,
}

// tmhShop5Rank keeps the Shop 5 stock together. Thiollier has no separate
// Twin Maiden Husks checkbox, but Moore's bearing carries Thiollier's
// progressed stock into the same Shop 5 inventory.
var tmhShop5Rank = map[string]int{
	"Moore":      0,
	"Thiollier":  1,
	"Count Ymir": 2,
}

// ErrNoSaveLoaded is returned by operations that need a loaded save.
var ErrNoSaveLoaded = errors.New("no save loaded")

// EditError is an edit-validation failure (unknown row_id, material-gated row,
// bad out path) or a shopwrite failure — nothing was written. errors.As-able.
type EditError struct{ Msg string }

func (e *EditError) Error() string { return e.Msg }

func editErrorf(format string, a ...any) *EditError {
	return &EditError{Msg: fmt.Sprintf(format, a...)}
}

// canonical is one merchant_catalog.json entry (reconciled merchant identity).
type canonical struct {
	Kind     string `json:"kind"`
	Merchant string `json:"merchant"`
}

// Merchant is one entry of ListMerchants. EditableRowCount excludes
// material-locked rows (deliberately not editable, hidden by the GUI).
type Merchant struct {
	Name             string
	RowCount         int
	EditableRowCount int
}

// Catalog holds the currently loaded save plus cached decoded data. One
// instance per app run.
type Catalog struct {
	savePath string

	itemList     []*Item
	itemByID     map[int64]*Item
	weaponMaxLvl map[int64]int
	reinforce    *reinforceRates             // per-level attack/scaling/guard curves, see weapon_reinforce_rates.go
	sortOrder    map[int64]int64             // item id -> real in-game sortId, see item_sort_order.json
	sortGroup    map[int64]int64             // item id -> real in-game sortGroupId (menu "Type" group), the primary order key
	subCatRank   map[string]map[string]int64 // category -> subCategory -> game-order rank of its members (see buildSubCategoryRank)
	itemDetails  map[int64]ItemDetail        // item id -> popup detail data, see item_details.json
	merchantCat  map[int64]canonical
	rowNames     map[int64]rowName
	shopSchema   *shopwrite.ShopSchema
	mtrlSchema   *shopwrite.ShopSchema
	vanillaByID  map[int64]map[string]int64 // see vanilla.go

	rows []*Row // decoded rows for the current save; nil = not yet decoded

	// sellValueByEquipRef is lazily decoded on the first SellValue call for
	// the current save (nil = not yet decoded); see sellvalue.go.
	sellValueByEquipRef map[equipRef]int64
}

// New builds a Catalog, loading all embedded reference data once.
func New() (*Catalog, error) {
	c := &Catalog{}
	var err error
	if c.itemList, c.itemByID, err = loadItems(); err != nil {
		return nil, fmt.Errorf("load items.json: %w", err)
	}
	if c.weaponMaxLvl, err = loadWeaponReinforce(); err != nil {
		return nil, fmt.Errorf("load weapon_reinforce.json: %w", err)
	}
	if c.reinforce, err = loadReinforceRates(); err != nil {
		return nil, fmt.Errorf("load weapon_reinforce_rates.json: %w", err)
	}
	if c.sortOrder, c.sortGroup, err = loadSortOrder(); err != nil {
		return nil, fmt.Errorf("load item_sort_order.json: %w", err)
	}
	applySortOverrides(c.sortOrder, c.sortGroup)
	c.subCatRank = buildSubCategoryRank(c.itemList, c.sortOrder, c.sortGroup)
	if c.itemDetails, err = loadItemDetails(); err != nil {
		return nil, fmt.Errorf("load item_details.json: %w", err)
	}
	if c.merchantCat, err = loadMerchantCatalog(); err != nil {
		return nil, fmt.Errorf("load merchant_catalog.json: %w", err)
	}
	if c.rowNames, err = loadRowNames(); err != nil {
		return nil, fmt.Errorf("load shop_row_names.json: %w", err)
	}
	if c.shopSchema, err = shopwrite.LoadShopSchema(); err != nil {
		return nil, err
	}
	if c.mtrlSchema, err = loadMtrlSchema(); err != nil {
		return nil, fmt.Errorf("load equip_mtrl_set_schema.json: %w", err)
	}
	if c.vanillaByID, err = loadVanillaShopLineup(); err != nil {
		return nil, fmt.Errorf("load vanilla_shop_lineup.json: %w", err)
	}
	return c, nil
}

// loadMerchantCatalog converts merchant_catalog.json's native string row-id
// keys to int64 once at load time (mirrors loadVanillaShopLineup), so the
// per-row canonicalFor lookup on every ShopRows/ListMerchants/MerchantRows
// call is a direct int64 map hit rather than an fmt.Sprintf allocation.
func loadMerchantCatalog() (map[int64]canonical, error) {
	raw, err := data.FS.ReadFile("merchant_catalog.json")
	if err != nil {
		return nil, err
	}
	var m map[string]canonical
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := make(map[int64]canonical, len(m))
	for k, v := range m {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			continue
		}
		out[id] = v
	}
	return out, nil
}

// --- save lifecycle ---

// Loaded reports whether a save is currently loaded.
func (c *Catalog) Loaded() bool { return c.savePath != "" }

// SavePath returns the currently loaded save path ("" if none).
func (c *Catalog) SavePath() string { return c.savePath }

// LoadSave sanity-decodes the file before committing — a bad file leaves the
// previously loaded save untouched. Invalidates the row cache.
func (c *Catalog) LoadSave(path string) error {
	if _, err := shopwrite.LoadRegulation(path); err != nil {
		return err
	}
	c.savePath = path
	c.rows = nil
	c.sellValueByEquipRef = nil
	return nil
}

func (c *Catalog) requireSave() (string, error) {
	if c.savePath == "" {
		return "", ErrNoSaveLoaded
	}
	return c.savePath, nil
}

// --- rows ---

// ShopRows returns every ShopLineupParam row from the current save, enriched,
// in param-table order. Cached until the next LoadSave or successful ApplyEdits.
func (c *Catalog) ShopRows() ([]*Row, error) {
	if c.rows != nil {
		return c.rows, nil
	}
	path, err := c.requireSave()
	if err != nil {
		return nil, err
	}
	reg, err := shopwrite.LoadRegulation(path)
	if err != nil {
		return nil, err
	}
	_, blob, _, paramRows, err := shopwrite.LoadParamRows(reg.BND4, shopEntry)
	if err != nil {
		return nil, err
	}
	mtrl, err := loadMtrlContext(reg.BND4, c.mtrlSchema)
	if err != nil {
		return nil, err
	}

	rows := make([]*Row, 0, len(paramRows))
	for _, pr := range paramRows {
		r, err := c.enrichRow(pr, blob, c.shopSchema, mtrl)
		if err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	c.rows = rows
	return rows, nil
}

// RowsByID maps row_id -> enriched Row for the current save.
func (c *Catalog) RowsByID() (map[int64]*Row, error) {
	rows, err := c.ShopRows()
	if err != nil {
		return nil, err
	}
	m := make(map[int64]*Row, len(rows))
	for _, r := range rows {
		m[r.RowID] = r
	}
	return m, nil
}

// --- merchant identity ---

func (c *Catalog) canonicalFor(rowID int64) canonical {
	if cc, ok := c.merchantCat[rowID]; ok {
		return cc
	}
	return canonical{Kind: "unnamed"}
}

func isBrowsable(cc canonical) bool {
	return cc.Kind == "merchant" ||
		(cc.Kind == "special_exchange" && browsableSpecialExchanges[cc.Merchant])
}

// dragonCommunionAltarRank fixes the 3 Dragon Communion altars (see
// browsableSpecialExchanges) in real-world altar order (earliest-reachable
// first: Church/Limgrave, Cathedral/Caelid, Grand Altar/DLC) rather than
// alphabetical, and keeps them as one contiguous block rather than scattered
// by name.
var dragonCommunionAltarRank = map[string]int{
	"Church of Dragon Communion":      0,
	"Cathedral of Dragon Communion":   1,
	"Grand Altar of Dragon Communion": 2,
}

// bellBearingMerchantRank keeps the Shop Editor's generic merchant filters
// in the exact order of the Bell Bearings that unlock those shops at Twin
// Maiden Husks. The display names remain location-based; only the sort key
// encodes the game's bearing number.
var bellBearingMerchantRank = map[string]int{
	"Nomadic Merchant - North Limgrave":            1,
	"Nomadic Merchant - East Limgrave":             2,
	"Nomadic Merchant - Coastal Cave":              3,
	"Nomadic Merchant - East Weeping Peninsula":    4,
	"Nomadic Merchant - Liurnia of the Lakes":      5,
	"Nomadic Merchant - North Liurnia":             6,
	"Nomadic Merchant - Altus Plateau":             7,
	"Nomadic Merchant - Mt. Gelmir":                8,
	"Nomadic Merchant - Caelid (Aeonia Swamp)":     9,
	"Nomadic Merchant - South Caelid":              10,
	"Isolated Merchant - Weeping Peninsula":        1,
	"Isolated Merchant - Academy of Raya Lucaria":  2,
	"Isolated Merchant - Dragonbarrow":             3,
	"Hermit Merchant - Leyndell":                   1,
	"Hermit Merchant - Mountaintops of the Giants": 2,
	"Hermit Merchant - Ainsel River":               3,
}

func merchantFamilySortKey(name string) string {
	if rank, ok := bellBearingMerchantRank[name]; ok {
		return fmt.Sprintf("%02d:%s", rank, name)
	}
	return "99:" + name
}

// tmhShop4SortKey orders the combined Bell Bearing Shop 4 family. Unlike the
// old per-family blocks, the game puts Kalé and every non-Nomadic wandering
// merchant in this one shop. The family order follows the shop's bearing
// sequence; numbered families retain their bearing number.
func tmhShop4SortKey(name string) string {
	switch {
	case name == "Merchant Kale":
		return "00:" + name
	case strings.HasPrefix(name, "Isolated Merchant - "):
		return "01:" + merchantFamilySortKey(name)
	case strings.HasPrefix(name, "Hermit Merchant - "):
		return "02:" + merchantFamilySortKey(name)
	case strings.HasPrefix(name, "Abandoned Merchant - "):
		return "03:" + name
	case strings.HasPrefix(name, "Imprisoned Merchant - "):
		return "04:" + name
	default:
		return "99:" + name
	}
}

// MerchantSortKey uses Twin Maiden Husks' actual five Bell Bearing shop
// groups everywhere the app presents merchant names: Shop 1's specialist
// base-game NPCs (including Iji), Shop 2's Gostoc/Pidia/Patches/Blackguard group, Shop 3's Nomadic
// merchants, Shop 4's Kalé/Isolated/Hermit/Abandoned/Imprisoned group, and
// Shop 5's DLC sellers. The game merges each shop into an item menu rather
// than preserving a seller sequence, so Shop 1/2 use their bearing-list
// sequence; numbered wandering merchants use their bearing number.
// Twin Maiden Husks stays first and the three Dragon Communion altars remain
// a final real-world-order block. Shared by ListMerchants and the Characters
// view's merchant column.
func MerchantSortKey(name string) (int, string) {
	if altarRank, ok := dragonCommunionAltarRank[name]; ok {
		return 7, fmt.Sprintf("%d", altarRank)
	}
	switch {
	case name == "Twin Maiden Husks":
		return 0, name
	case tmhShop1Rank[name] != 0:
		return 1, fmt.Sprintf("%02d:%s", tmhShop1Rank[name], name)
	case tmhShop2Rank[name] != 0:
		return 2, fmt.Sprintf("%02d:%s", tmhShop2Rank[name], name)
	case strings.HasPrefix(name, "Nomadic Merchant - "):
		return 3, merchantFamilySortKey(name)
	case name == "Merchant Kale", strings.HasPrefix(name, "Isolated Merchant - "),
		strings.HasPrefix(name, "Hermit Merchant - "), strings.HasPrefix(name, "Abandoned Merchant - "),
		strings.HasPrefix(name, "Imprisoned Merchant - "):
		return 4, tmhShop4SortKey(name)
	case name == "Moore", name == "Thiollier", name == "Count Ymir":
		return 5, fmt.Sprintf("%02d:%s", tmhShop5Rank[name], name)
	default:
		return 6, name
	}
}

// ListMerchants returns each browsable merchant with its row count, sorted by
// MerchantSortKey. Ports catalog.py list_merchants.
func (c *Catalog) ListMerchants() ([]Merchant, error) {
	rows, err := c.ShopRows()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	editable := map[string]int{}
	for _, r := range rows {
		cc := c.canonicalFor(r.RowID)
		if isBrowsable(cc) {
			counts[cc.Merchant]++
			if !r.MaterialLocked {
				editable[cc.Merchant]++
			}
		}
	}
	out := make([]Merchant, 0, len(counts))
	for name, n := range counts {
		out = append(out, Merchant{Name: name, RowCount: n, EditableRowCount: editable[name]})
	}
	sort.Slice(out, func(i, j int) bool {
		gi, ni := MerchantSortKey(out[i].Name)
		gj, nj := MerchantSortKey(out[j].Name)
		if gi != gj {
			return gi < gj
		}
		return ni < nj
	})
	return out, nil
}

// MerchantRows returns one browsable merchant's rows in their stable
// ShopLineupParam order. This is the editor's editable slot layout: putting
// items into consecutive cells stays predictable while a batch is being
// assembled. The separate Game Preview view applies the item's global menu
// sort, which is how Elden Ring ultimately displays the same rows. Returns an
// error if no rows match (unknown/empty merchant name). Ports catalog.py
// merchant_rows (the enriched Row already carries every field it returned).
func (c *Catalog) MerchantRows(name string) ([]*Row, error) {
	rows, err := c.ShopRows()
	if err != nil {
		return nil, err
	}
	var out []*Row
	for _, r := range rows {
		cc := c.canonicalFor(r.RowID)
		if !isBrowsable(cc) || cc.Merchant != name {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no rows for merchant %q", name)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RowID < out[j].RowID })
	return out, nil
}

// --- write-back ---

// ApplyEdits validates the edits, then calls shopwrite.Apply in-process. On
// success outPath becomes the new current save (Save-As semantics) and the row
// cache is invalidated. Ports catalog.py apply_edits (same rejections/order).
func (c *Catalog) ApplyEdits(edits []shopwrite.Edit, outPath string) (*shopwrite.Summary, error) {
	if len(edits) == 0 {
		return nil, editErrorf("no edits given")
	}
	path, err := c.requireSave()
	if err != nil {
		return nil, err
	}
	outAbs, err := filepath.Abs(outPath)
	if err != nil {
		return nil, err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if outAbs == pathAbs {
		return nil, editErrorf("out_path must differ from the currently loaded save")
	}

	rowsByID, err := c.RowsByID()
	if err != nil {
		return nil, err
	}
	var unknown []int64
	for _, e := range edits {
		if _, ok := rowsByID[e.RowID]; !ok {
			unknown = append(unknown, e.RowID)
		}
	}
	if len(unknown) > 0 {
		return nil, editErrorf("unknown row_id(s): %s", formatIDs(unknown))
	}

	var materialGated []int64
	for _, e := range edits {
		if rowsByID[e.RowID].MaterialLocked {
			materialGated = append(materialGated, e.RowID)
		}
	}
	if len(materialGated) > 0 {
		return nil, editErrorf(
			"row_id(s) %s require a material trade (mtrlId != -1); editing these rows isn't supported yet",
			formatIDs(materialGated))
	}

	summary, err := shopwrite.Apply(path, outAbs, shopEntry, edits)
	if err != nil {
		return nil, &EditError{Msg: err.Error()}
	}

	c.savePath = outAbs
	c.rows = nil
	c.sellValueByEquipRef = nil
	return summary, nil
}

// formatIDs renders an int64 slice as Python's list repr ("[a, b, c]").
func formatIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
