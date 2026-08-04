package catalog

// Item DB (items.json) loading, item queries, and the equipId<->item-id
// conversion. Ported from savescan.py (equip_ref_for_item_id, resolve_item_id,
// resolve_material_item_id) and catalog.py (list_items/categories/subcategories).

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/assets/data"
)

// Item is one items.json entry. EquipType/EquipID are nil for a non-sellable
// item (no resolvable equip ref).
type Item struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	SubCategory string `json:"subCategory"`
	IconPath    string `json:"iconPath"`
	EquipType   *int   `json:"equipType"`
	EquipID     *int64 `json:"equipId"`
	// Risky marks cut-content / online-ban-risk items (SaveForge's curated
	// flags, see itemdb_extract). Listed normally; the GUI only offers them
	// in Debug mode.
	Risky bool `json:"risky"`
}

// rawItem is the on-disk items.json shape (no equip ref; that's derived).
// Hidden entries are same-name/same-category internal variants of another id
// (see itemdb_extract's hiddenItemIDs): resolvable by id so shop rows still
// enrich, but excluded from the browsable item list.
type rawItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	SubCategory string `json:"subCategory"`
	IconPath    string `json:"iconPath"`
	Hidden      bool   `json:"hidden"`
	Risky       bool   `json:"risky"`
}

// equipTypeItemIDOffset mirrors savescan.py EQUIP_TYPE_ITEM_ID_OFFSET: the
// per-category offset added to a ShopLineupParam equipId (or a single
// materialId) to reach the unified items.json id. 0x80000000 exceeds int32, so
// int64 throughout.
var equipTypeItemIDOffset = map[int]int64{
	0: 0x00000000, // Weapon
	1: 0x10000000, // Protector (armor)
	2: 0x20000000, // Accessory (talisman)
	3: 0x40000000, // Good
	4: 0x80000000, // Gem (Ash of War)
}

// EquipRefForItemID is the inverse of resolveItemID: a unified item id -> the
// (equipType, equipId) a ShopLineupParam row needs, or ok=false if none applies
// (only when itemID < the smallest offset, i.e. negative). The 5 category
// offsets never overlap in real data, so "largest offset <= itemID" always
// picks the right bucket. Ported from savescan.py equip_ref_for_item_id.
func EquipRefForItemID(itemID int64) (equipType int, equipID int64, ok bool) {
	bestOffset := int64(-1)
	for et, off := range equipTypeItemIDOffset {
		if itemID >= off && off > bestOffset {
			bestOffset, equipType = off, et
		}
	}
	if bestOffset < 0 {
		return 0, 0, false
	}
	return equipType, itemID - bestOffset, true
}

// resolveItemID converts an equipId (+equipType) or a single materialId
// (+materialCate) to the unified items.json id, or ok=false if the offset is
// unknown or the result isn't a real item. Trusts category's offset alone.
// Ported from savescan.py resolve_item_id.
func resolveItemID(rawID int64, category int, itemsByID map[int64]*Item) (int64, bool) {
	offset, known := equipTypeItemIDOffset[category]
	if !known {
		return 0, false
	}
	itemID := rawID + offset
	if _, ok := itemsByID[itemID]; !ok {
		return 0, false
	}
	return itemID, true
}

// resolveItemIDWithLevel wraps resolveItemID with a weapon-reinforcement
// fallback resolveItemID itself doesn't have (and shouldn't -- it mirrors
// savescan.py resolve_item_id exactly, the golden-test oracle). For weapons
// (category 0) whose raw id doesn't resolve directly, falls back to treating
// it as a real reinforced variant of a base (+0) item -- see
// weaponReinforceLevel. Without this, a leveled weapon row can't resolve at
// all, since items.json only indexes +0 base ids. Safe to add without
// touching the golden-tested function: only rows this app itself writes via
// the "+N" leveling feature exercise this path, so the golden test doesn't.
// ResolveItemID is resolveItemIDWithLevel's exported wrapper (the level
// return is dropped -- callers needing it, e.g. weapon-table row decode,
// stay on the private form): equipId+equipType -> the unified items.json id,
// for callers outside this package that need to resolve a raw ItemChange's
// EquipID/EquipType back to a browsable Item (e.g. the item-info popup
// resolving a staged swap's target item).
func (c *Catalog) ResolveItemID(equipID int64, equipType int) (itemID int64, ok bool) {
	id, _, ok := c.resolveItemIDWithLevel(equipID, equipType)
	return id, ok
}

// ItemByID resolves an items.json id to its Item, INCLUDING hidden ones
// (ListItems deliberately omits those -- they're excluded from the
// browsable catalog grid but still need to resolve for name/icon/details
// wherever a real shop row references one). nil if the id is unknown.
//
// This is deliberately the permissive lookup: "is it browsable" is
// ListItems' job (it filters hidden out), and using ListItems as an
// id->Item index instead silently dropped real, sellable rows' items --
// exactly the bug this exists to prevent (user-reported 2026-08-03:
// right-clicking Flask of Wondrous Physick, a hidden item TMH really
// sells, opened no info popup at all).
func (c *Catalog) ItemByID(id int64) *Item { return c.itemByID[id] }

func (c *Catalog) resolveItemIDWithLevel(rawID int64, category int) (itemID int64, level int64, ok bool) {
	if id, ok := resolveItemID(rawID, category, c.itemByID); ok {
		return id, 0, true
	}
	if category == 0 {
		if base, lvl, ok := c.weaponReinforceLevel(rawID); ok {
			return base, lvl, true
		}
	}
	return 0, 0, false
}

// weaponReinforceLevel checks whether rawID is a real reinforced ("+N")
// variant of some base (+0) weapon item: base ids are always exact
// multiples of 10000, and a reinforced id is base+N (see docs/ITEM_IDS.md's
// "Weapon reinforcement levels"). Validated against weapon_reinforce.json's
// real per-item max level (via MaxUpgradeLevel), not just a plausible-range
// guess, so this can't false-positive on an id that merely happens to be
// close to a real base id.
func (c *Catalog) weaponReinforceLevel(rawID int64) (base int64, level int64, ok bool) {
	base = rawID - (rawID % 10000)
	level = rawID - base
	if _, exists := c.itemByID[base]; !exists {
		return 0, 0, false
	}
	max, isWeapon := c.MaxUpgradeLevel(base)
	if !isWeapon || level > int64(max) {
		return 0, 0, false
	}
	return base, level, true
}

// resolveMaterialItemID mirrors savescan.py resolve_material_item_id: try every
// category offset, take the unique items.json hit, and only use materialCate to
// disambiguate when >1 offset hits (materialCate's shipped default is an
// unreliable primary key for trade materials).
func resolveMaterialItemID(materialID int64, materialCate int, itemsByID map[int64]*Item) (int64, bool) {
	type hit struct {
		off, id int64
	}
	// Deterministic order matters only for the >1-hit tiebreak fallback below;
	// iterate the offsets in ascending category order to match Python's dict
	// insertion order (0..4).
	var hits []hit
	for cat := 0; cat <= 4; cat++ {
		off := equipTypeItemIDOffset[cat]
		id := materialID + off
		if _, ok := itemsByID[id]; ok {
			hits = append(hits, hit{off, id})
		}
	}
	if len(hits) == 0 {
		return 0, false
	}
	if len(hits) == 1 {
		return hits[0].id, true
	}
	if cateOff, ok := equipTypeItemIDOffset[materialCate]; ok {
		for _, h := range hits {
			if h.off == cateOff {
				return h.id, true
			}
		}
	}
	return hits[0].id, true
}

// --- catalog Item queries ---

// loadItems decodes items.json from the embedded data.FS and derives each
// item's equip ref (nil when unresolvable). Called once at construction.
func loadItems() ([]*Item, map[int64]*Item, error) {
	raw, err := data.FS.ReadFile("items.json")
	if err != nil {
		return nil, nil, err
	}
	var rawItems []rawItem
	if err := json.Unmarshal(raw, &rawItems); err != nil {
		return nil, nil, err
	}
	list := make([]*Item, 0, len(rawItems))
	byID := make(map[int64]*Item, len(rawItems))
	for _, ri := range rawItems {
		it := &Item{
			ID: ri.ID, Name: ri.Name, Category: ri.Category,
			SubCategory: ri.SubCategory, IconPath: ri.IconPath, Risky: ri.Risky,
		}
		if et, eid, ok := EquipRefForItemID(ri.ID); ok {
			etCopy, eidCopy := et, eid
			it.EquipType, it.EquipID = &etCopy, &eidCopy
		}
		if !ri.Hidden {
			list = append(list, it)
		}
		byID[it.ID] = it
	}
	return list, byID, nil
}

// ListItems mirrors catalog.py list_items: filter by category/subCategory/
// case-insensitive name search/equipType. A nil filter pointer means "any".
// Result is ordered so the unfiltered grid reads as the sub-category filter
// list in order, one contiguous block per sub-category: primary key is the
// sub-category's own rank (itemGroupRank -> subCategoryRank, the SAME rank
// ListSubcategories orders the filter dropdown by), secondary the item's
// real in-game position within that block (gameOrderKey, i.e.
// (sortGroupId, sortId)). Items with no real sort data sort last, by id.
//
// Sub-category is the primary key rather than the game's raw sortGroupId
// (which it was until 2026-08-03) because a sub-category can span several
// sortGroupIds -- e.g. key items' "Quest Items" straddles the group holding
// "Great Runes", so raw game order interleaved them and the grid stopped
// matching the filter list's order (user-reported: only Memory of Grace
// appeared under Quest Items, the rest landed after Great Runes).
func (c *Catalog) ListItems(category, subCategory, search string, equipType *int) []*Item {
	out := make([]*Item, 0)
	for _, it := range c.itemList {
		if category != "" && it.Category != category {
			continue
		}
		if subCategory != "" && it.SubCategory != subCategory {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(it.Name), strings.ToLower(search)) {
			continue
		}
		if equipType != nil {
			if it.EquipType == nil || *it.EquipType != *equipType {
				continue
			}
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := c.itemGroupRank(a), c.itemGroupRank(b); ra != rb {
			return ra < rb
		}
		sa, sb := c.itemGameOrder(a.ID), c.itemGameOrder(b.ID)
		if sa != sb {
			return sa < sb
		}
		return a.ID < b.ID
	})
	return out
}

// itemGroupRank is the primary ordering key: the item's sub-category rank,
// so the grid groups by sub-category in exactly the order
// ListSubcategories shows in the filter dropdown (both read subCatRank).
// Items whose sub-category carries no rank sort last (noSortRank), by their
// own game order then id.
func (c *Catalog) itemGroupRank(it *Item) int64 {
	return c.subCategoryRank(it)
}

// itemGameOrder is the within-sub-category ordering key: the item's absolute
// position in the game's own menu order, (sortGroupId, sortId) collapsed via
// gameOrderKey. Uses the full key rather than sortId alone because a
// sub-category can span several sortGroupIds (see ListItems), where sortId
// alone would not be comparable across the boundary.
func (c *Catalog) itemGameOrder(id int64) int64 {
	sortID, ok := c.sortOrder[id]
	if !ok {
		return noSortRank
	}
	return gameOrderKey(c.sortGroup[id], sortID)
}

// ItemComesBeforeInMerchantMenu reports whether item a is shown before item b
// in Elden Ring's merchant menu. The menu advances sort groups in ascending
// order, then advances sortId in ascending order within each group. This is
// notably the same direction as the catalog's browse order: Rune Arc (520)
// is before Starlight Shards (530), which is before Blessing of Marika (540).
//
// Items without a usable sort key go after known items. Equal keys deliberately
// compare equal so the caller can retain ShopLineupParam row-ID order as the
// stable tie-breaker (including multiple rows selling the same item).
func (c *Catalog) ItemComesBeforeInMerchantMenu(a, b int64) bool {
	aSort, aOK := c.sortOrder[a]
	bSort, bOK := c.sortOrder[b]
	aOK = aOK && aSort != 0
	bOK = bOK && bSort != 0
	if aOK != bOK {
		return aOK
	}
	if !aOK { // neither item has sort data
		return false
	}
	if aGroup, bGroup := c.sortGroup[a], c.sortGroup[b]; aGroup != bGroup {
		return aGroup < bGroup
	}
	return aSort < bSort
}

// noSortRank sorts after every item carrying a real sortId, rather than
// before (a bare zero value would wrongly sort first).
const noSortRank = int64(1) << 62

func (c *Catalog) subCategoryRank(it *Item) int64 {
	if ranks, ok := c.subCatRank[it.Category]; ok {
		if r, ok := ranks[it.SubCategory]; ok {
			return r
		}
	}
	return noSortRank
}

// ListCategories mirrors catalog.py list_categories: sorted distinct categories.
func (c *Catalog) ListCategories() []string {
	set := map[string]struct{}{}
	for _, it := range c.itemList {
		set[it.Category] = struct{}{}
	}
	return sortedKeys(set)
}

// ListSubcategories returns the distinct non-empty subCategories within one
// category, ordered to match the item grid (ListItems' subCategoryRank), so
// the filter dropdown and the grid agree instead of the filter being
// alphabetical while the grid follows the game's menu order. Ties (or
// sub-categories with no real sort data) fall back to alphabetical.
func (c *Catalog) ListSubcategories(category string) []string {
	set := map[string]struct{}{}
	for _, it := range c.itemList {
		if it.Category == category && it.SubCategory != "" {
			set[it.SubCategory] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	ranks := c.subCatRank[category]
	rank := func(sub string) int64 {
		if ranks != nil {
			if r, ok := ranks[sub]; ok {
				return r
			}
		}
		return noSortRank
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := rank(out[i]), rank(out[j]); ri != rj {
			return ri < rj
		}
		return out[i] < out[j]
	})
	return out
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// weaponReinforceEntry is one internal/assets/data/weapon_reinforce.json entry.
type weaponReinforceEntry struct {
	ItemID   int64 `json:"itemId"`
	MaxLevel int   `json:"maxLevel"`
}

// loadWeaponReinforce decodes weapon_reinforce.json (item id -> real max
// reinforcement/"+N" level, generated by tools/weapon_reinforce_extract from
// EquipParamWeapon/ReinforceParamWeapon -- see docs/ITEM_IDS.md). Called
// once at construction, same as loadItems.
func loadWeaponReinforce() (map[int64]int, error) {
	raw, err := data.FS.ReadFile("weapon_reinforce.json")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Weapons []weaponReinforceEntry `json:"weapons"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make(map[int64]int, len(doc.Weapons))
	for _, w := range doc.Weapons {
		out[w.ItemID] = w.MaxLevel
	}
	return out, nil
}

// loadSortOrder decodes item_sort_order.json (item id -> real in-game
// sortId + sortGroupId, generated by tools/sort_order_extract from
// EquipParamWeapon/Protector/Accessory/Gem/Goods -- see docs/ITEM_IDS.md).
// Keyed by string in the JSON (Go's encoding/json always stringifies map
// keys on output; this file was written by Python, same convention). Both
// columns are kept: sortGroupId is the game's real menu "Type" group (the
// primary ordering key), sortId the position within it -- see ListItems and
// buildSubCategoryRank for why the game order is (sortGroupId, sortId).
func loadSortOrder() (sortID, sortGroup map[int64]int64, err error) {
	raw, err := data.FS.ReadFile("item_sort_order.json")
	if err != nil {
		return nil, nil, err
	}
	var doc map[string]struct {
		SortID    int64 `json:"sortId"`
		SortGroup int64 `json:"sortGroupId"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, err
	}
	sortID = make(map[int64]int64, len(doc))
	sortGroup = make(map[int64]int64, len(doc))
	for k, v := range doc {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("item_sort_order.json: bad key %q: %w", k, err)
		}
		sortID[id] = v.SortID
		sortGroup[id] = v.SortGroup
	}
	return sortID, sortGroup, nil
}

// sortOverrides patches a handful of item ids whose REAL EquipParamGoods
// sortId/sortGroupId (verified against our fixture's own regulation.bin,
// same method as loadSortOrder) sorts them somewhere a player wouldn't
// expect, as a deliberate, narrow exception to "always match game order"
// (ListItems' own rule) -- not an extraction bug, just cosmetic browsing
// convenience for a handful of known-janky rows.
//
// Great Rune of the Unborn (item id 1073751904, real goods id 10080)
// ships with sortId=200060/sortGroupId=10 (verified via
// tools/savescan.py + tools/paramdex_schema.py against
// save_files/vanilla_fresh_character.dat) -- alone among the 7 Great
// Runes, whose other 6 members (Godrick's/Radahn's/Morgott's/Rykard's/
// Mohg's/Malenia's) all share sortGroupId=40, sortId 203030-203080 (next
// to Stonesword Key's 203090). Almost certainly a Shadow of the Erdtree
// DLC-addition quirk in FromSoft's own data, not ours (user-spotted
// 2026-08-03). Re-slotted at the end of that cluster, just before
// Stonesword Key.
var sortOverrides = map[int64]struct{ sortID, sortGroup int64 }{
	1073751904: {sortID: 203085, sortGroup: 40}, // Great Rune of the Unborn
}

func applySortOverrides(sortID, sortGroup map[int64]int64) {
	for id, ov := range sortOverrides {
		sortID[id] = ov.sortID
		sortGroup[id] = ov.sortGroup
	}
}

// explicitSubCatOrder overrides the sortId-derived sub-category order for
// categories whose in-menu grouping isn't captured by item sortId alone.
// Ashes of War: the affinity buckets are our own overlay
// (tools/aow_categories), so they carry no natural sortId grouping; the
// requested order is the scaling progression -- physical (Strength,
// Dexterity, Quality) then Magic, Faith, Arcane (user request 2026-08-02).
// Listed sub-categories get tiny sequential ranks so they sort ahead of any
// unlisted one (e.g. Ashes of War's un-affinitied "None" items, which keep
// their real sortId rank and fall after the affinity groups).
var explicitSubCatOrder = map[string][]string{
	"ashes_of_war": {"Strength", "Dexterity", "Quality", "Magic", "Faith", "Arcane", "No Affinity"},
}

// gameOrderKey collapses (sortGroupId, sortId) into one comparable int64 --
// the item's absolute position in the game's menu order. sortGroupId is the
// coarse group, sortId the position within it; the multiplier exceeds every
// real sortId (max observed ~3.4M for weapons) so the group always dominates.
const gameOrderMul = int64(10_000_000)

func gameOrderKey(group, sortID int64) int64 { return group*gameOrderMul + sortID }

// buildSubCategoryRank precomputes, for every (category, subCategory) pair,
// the display order of the sub-categories in the filter dropdown -- ranking
// each by its members' MINIMUM game-order key (sortGroupId, sortId), i.e. the
// order in which the sub-category first appears as you scroll the grid (which
// itemGroupRank now orders by the same game key). Minimum (first appearance)
// is correct here because the grid is the game's flat (sortGroupId, sortId)
// list, not a sub-category-grouped list, so the dropdown just mirrors that
// order. A pair with no member carrying real sort data is simply absent
// (subCategoryRank falls back to noSortRank), unless explicitSubCatOrder pins
// it. explicitSubCatOrder still drives BOTH the dropdown and the grid group
// order for its categories (Ashes of War affinities).
func buildSubCategoryRank(items []*Item, sortOrder, sortGroup map[int64]int64) map[string]map[string]int64 {
	collect := make(map[string]map[string]int64) // cat -> sub -> min game-order key
	for _, it := range items {
		sortID, ok := sortOrder[it.ID]
		if !ok {
			continue
		}
		key := gameOrderKey(sortGroup[it.ID], sortID)
		byCat, ok := collect[it.Category]
		if !ok {
			byCat = make(map[string]int64)
			collect[it.Category] = byCat
		}
		if cur, seen := byCat[it.SubCategory]; !seen || key < cur {
			byCat[it.SubCategory] = key
		}
	}
	out := make(map[string]map[string]int64, len(collect))
	for cat, byCat := range collect {
		ranks := make(map[string]int64, len(byCat))
		for sub, key := range byCat {
			ranks[sub] = key
		}
		out[cat] = ranks
	}
	// Explicit overrides win over the game-order key (e.g. Ashes of War
	// affinities, an overlay with no natural sortId grouping -- see
	// explicitSubCatOrder). Their tiny 0..N ranks also make itemGroupRank
	// group the grid by affinity for those categories.
	for cat, order := range explicitSubCatOrder {
		byCat, ok := out[cat]
		if !ok {
			byCat = make(map[string]int64)
			out[cat] = byCat
		}
		for i, sub := range order {
			byCat[sub] = int64(i)
		}
	}
	return out
}

// MaxUpgradeLevel returns itemID's real max reinforcement ("+N") level and
// whether it's a weapon-table item at all (armor/talismans/goods/Ashes of
// War never appear in weapon_reinforce.json, so ok is false for them). A
// weapon that genuinely can't be reinforced in-game (a handful exist) comes
// back as (0, true), not (0, false) -- still a weapon, just capped at +0.
func (c *Catalog) MaxUpgradeLevel(itemID int64) (max int, ok bool) {
	max, ok = c.weaponMaxLvl[itemID]
	return max, ok
}
