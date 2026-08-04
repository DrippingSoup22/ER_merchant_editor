package application

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/savefile"
)

// PendingCount reports how many merchant rows have staged edits.
func (session *Session) PendingCount() int {
	return len(session.PendingEdits)
}

// BuildEdits converts staged editor changes into deterministic low-level save
// mutations. Presentation details never cross this boundary.
func (session *Session) BuildEdits() []savefile.Edit {
	ids := make([]int64, 0, len(session.PendingEdits))
	for id := range session.PendingEdits {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	edits := make([]savefile.Edit, 0, len(ids))
	for _, id := range ids {
		entry := session.PendingEdits[id]
		fields := make(map[string]json.Number, len(entry.FieldChanges)+2)
		for name, change := range entry.FieldChanges {
			fields[name] = json.Number(strconv.FormatInt(change.To, 10))
		}
		if entry.ItemChange != nil {
			fields["equipId"] = json.Number(strconv.FormatInt(entry.ItemChange.EquipID, 10))
			fields["equipType"] = json.Number(strconv.FormatInt(entry.ItemChange.EquipType, 10))
			for _, name := range entry.ItemChange.ClearOverrides {
				fields[name] = json.Number("-1")
			}
		}
		edits = append(edits, savefile.Edit{RowID: id, Fields: fields})
	}
	return edits
}

// EquipParamRef identifies a row in one of the EquipParam tables.
type EquipParamRef struct {
	EquipType int64
	EquipID   int64
}

// EquipParamTarget describes the live and required sellValue for an item.
type EquipParamTarget struct {
	Current   int64
	CurrentOK bool
	Target    int64
}

// EquipParamRefForEdit resolves the item and effective price affected by an
// edit. Quantity-only and unrelated changes return ok=false.
func EquipParamRefForEdit(row *catalog.Row, edit *RowEdit) (ref EquipParamRef, price int64, ok bool) {
	valueChange, hasValueEdit := edit.FieldChanges["value"]
	if !hasValueEdit && edit.ItemChange == nil {
		return EquipParamRef{}, 0, false
	}
	price = -1
	if row.Price != nil {
		price = *row.Price
	}
	if hasValueEdit {
		price = valueChange.To
	}
	if edit.ItemChange != nil {
		ref = EquipParamRef{EquipType: edit.ItemChange.EquipType, EquipID: edit.ItemChange.BaseEquipID}
	} else {
		ref = EquipParamRef{EquipType: row.Fields["equipType"], EquipID: row.Fields["equipId"]}
	}
	return ref, price, true
}

func currentRowRef(row *catalog.Row) (ref EquipParamRef, price int64) {
	price = -1
	if row.Price != nil {
		price = *row.Price
	}
	return EquipParamRef{EquipType: row.Fields["equipType"], EquipID: row.Fields["equipId"]}, price
}

// ComputeEquipParamTargets calculates the maximum safe sellValue for every
// item affected by the current batch. It is shared by saving and UI preview.
func (session *Session) ComputeEquipParamTargets() (targets map[EquipParamRef]*EquipParamTarget, rowRefs map[int64]EquipParamRef, err error) {
	if len(session.PendingEdits) == 0 {
		return nil, nil, nil
	}
	rowsByID, err := session.Catalog.RowsByID()
	if err != nil {
		return nil, nil, err
	}

	touchedRefs := map[EquipParamRef]bool{}
	rowRefs = map[int64]EquipParamRef{}
	for rowID, edit := range session.PendingEdits {
		row := rowsByID[rowID]
		if row == nil {
			continue
		}
		ref, _, ok := EquipParamRefForEdit(row, edit)
		if !ok {
			continue
		}
		rowRefs[rowID] = ref
		touchedRefs[ref] = true
	}
	if len(touchedRefs) == 0 {
		return nil, rowRefs, nil
	}

	globalMin := map[EquipParamRef]int64{}
	settled := map[EquipParamRef]bool{}
	for rowID, row := range rowsByID {
		var ref EquipParamRef
		var price int64
		if edit, staged := session.PendingEdits[rowID]; staged {
			if effectiveRef, effectivePrice, ok := EquipParamRefForEdit(row, edit); ok {
				ref, price = effectiveRef, effectivePrice
			} else {
				ref, price = currentRowRef(row)
			}
		} else {
			ref, price = currentRowRef(row)
		}
		if !touchedRefs[ref] || settled[ref] {
			continue
		}
		if price == -1 {
			globalMin[ref] = -1
			settled[ref] = true
			continue
		}
		if current, seen := globalMin[ref]; !seen || price < current {
			globalMin[ref] = price
		}
	}

	targets = map[EquipParamRef]*EquipParamTarget{}
	for ref := range touchedRefs {
		current, ok := session.Catalog.SellValue(ref.EquipType, ref.EquipID)
		targets[ref] = &EquipParamTarget{Current: current, CurrentOK: ok, Target: globalMin[ref]}
	}
	return targets, rowRefs, nil
}

// BuildEquipParamEdits builds the sellValue guard mutations required by the
// current merchant edits, grouped by EquipParam entry name.
func (session *Session) BuildEquipParamEdits() (map[string][]savefile.Edit, error) {
	targets, _, err := session.ComputeEquipParamTargets()
	if err != nil {
		return nil, err
	}
	out := map[string][]savefile.Edit{}
	for ref, target := range targets {
		if !target.CurrentOK || target.Target == target.Current {
			continue
		}
		entryName, ok := savefile.EquipParamEntryName(ref.EquipType)
		if !ok {
			continue
		}
		out[entryName] = append(out[entryName], savefile.Edit{
			RowID: ref.EquipID,
			Fields: map[string]json.Number{
				"sellValue": json.Number(strconv.FormatInt(target.Target, 10)),
			},
		})
	}
	return out, nil
}

// SuggestedOutPath returns the default non-destructive destination next to
// the currently loaded save.
func (session *Session) SuggestedOutPath() string {
	path := session.Catalog.SavePath()
	if path == "" {
		return ""
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(filepath.Base(path), ext)
	return filepath.Join(filepath.Dir(path), stem+"-edited"+ext)
}
