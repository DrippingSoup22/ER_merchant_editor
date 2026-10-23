package application

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/character"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/savefile"
)

// Load validates a save and warms the merchant decode before returning. It is
// synchronous; desktop callers normally run it outside their frame goroutine.
func (session *Session) Load(path string) error {
	if err := session.Catalog.LoadSave(path); err != nil {
		return err
	}
	_, err := session.Catalog.ListMerchants()
	return err
}

// SavePlan is an immutable snapshot of all staged mutations. Preparing the
// plan before displaying Save As prevents UI changes made while a native
// dialog is open from being silently omitted.
type SavePlan struct {
	session         *Session
	itemEdits       []savefile.Edit
	flagTargets     map[int][]character.FlagTarget
	equipParamEdits map[string][]savefile.Edit
}

// PrepareSave validates and snapshots every currently staged edit.
func (session *Session) PrepareSave() (*SavePlan, error) {
	plan := &SavePlan{
		session:     session,
		itemEdits:   session.BuildEdits(),
		flagTargets: make(map[int][]character.FlagTarget),
	}

	equipEdits, err := session.BuildEquipParamEdits()
	if err != nil {
		return nil, err
	}
	plan.equipParamEdits = equipEdits

	if len(session.PendingFlagEdits) > 0 {
		rowsByID, err := session.Catalog.RowsByID()
		if err != nil {
			return nil, err
		}
		for charIndex, rowEdits := range session.PendingFlagEdits {
			for rowID, released := range rowEdits {
				if row := rowsByID[rowID]; row != nil {
					plan.flagTargets[charIndex] = append(plan.flagTargets[charIndex], character.FlagTarget{
						FlagID:   row.UnlockFlag,
						Released: released,
						Label:    fmt.Sprintf("row %d", row.RowID),
					})
				}
			}
		}
	}
	for charIndex, edits := range session.PendingBellBearingEdits {
		for flagID, released := range edits {
			plan.flagTargets[charIndex] = append(plan.flagTargets[charIndex], character.FlagTarget{
				FlagID:   int64(flagID),
				Released: released,
				Label:    fmt.Sprintf("bell bearing flag %d", flagID),
			})
		}
	}
	return plan, nil
}

// Apply writes a prepared plan to outPath through the required ordered save
// stages. Every stage but the last writes into a private folder beside the
// output, removed at the end. The loaded catalog advances to the output only
// after every stage succeeds; a failure leaves the input loaded.
func (plan *SavePlan) Apply(outPath string) error {
	session := plan.session
	inPath := session.Catalog.SavePath()

	type stageFunc func(from, to string) error
	var stages []stageFunc
	if len(plan.flagTargets) > 0 {
		stages = append(stages, func(from, to string) error {
			_, err := character.ApplyBatchToFile(from, to, plan.flagTargets)
			return err
		})
	}
	for equipType := int64(0); equipType <= 4; equipType++ {
		entryName, ok := savefile.EquipParamEntryName(equipType)
		if !ok || len(plan.equipParamEdits[entryName]) == 0 {
			continue
		}
		schema, err := savefile.LoadEquipParamSchema(equipType)
		if err != nil {
			return err
		}
		name := entryName
		edits := plan.equipParamEdits[entryName]
		stages = append(stages, func(from, to string) error {
			_, err := savefile.ApplyWithSchema(from, to, name, edits, schema)
			return err
		})
	}
	if len(plan.itemEdits) > 0 {
		stages = append(stages, func(from, to string) error {
			if from != session.Catalog.SavePath() {
				if err := session.Catalog.LoadSave(from); err != nil {
					return err
				}
			}
			_, err := session.Catalog.ApplyEdits(plan.itemEdits, to)
			return err
		})
	}

	var stageDir string
	if len(stages) > 1 {
		dir, err := os.MkdirTemp(filepath.Dir(outPath), ".ermerchant-save-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		stageDir = dir
	}
	current := inPath
	for index, run := range stages {
		target := outPath
		if index < len(stages)-1 {
			target = filepath.Join(stageDir, fmt.Sprintf("stage%d.dat", index))
		}
		if err := run(current, target); err != nil {
			// The item stage loads its input, which may be a stage file about
			// to be removed: put the input back so the next save starts there.
			if session.Catalog.SavePath() != inPath {
				if reloadErr := session.Catalog.LoadSave(inPath); reloadErr != nil {
					return errors.Join(err, reloadErr)
				}
			}
			return err
		}
		current = target
	}

	if session.Catalog.SavePath() != outPath {
		if err := session.Catalog.LoadSave(outPath); err != nil {
			return err
		}
	}
	_, _ = session.Catalog.ListMerchants()
	return nil
}
