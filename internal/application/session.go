// Package application coordinates one editor session without depending on a
// window toolkit or operating-system UI. It is the stateful boundary between
// presentation code and the catalog/character/savefile core packages.
package application

import (
	"fmt"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
)

// FieldChange is a staged scalar PARAM field mutation.
type FieldChange struct {
	From int64
	To   int64
}

// ItemChange is a staged replacement of the item sold by a merchant row.
type ItemChange struct {
	FromName       string
	ToName         string
	IconPath       string
	EquipID        int64
	EquipType      int64
	BaseEquipID    int64
	Level          int64
	ClearOverrides []string
}

// DisplayName returns the staged item's player-facing name.
func (change *ItemChange) DisplayName() string {
	if change.Level > 0 {
		return fmt.Sprintf("%s +%d", change.ToName, change.Level)
	}
	return change.ToName
}

// IsLevelOnly reports whether the base item is unchanged.
func (change *ItemChange) IsLevelOnly() bool {
	return change.FromName == change.ToName
}

// RowEdit contains every staged mutation for one merchant row.
type RowEdit struct {
	Label        string
	Merchant     string
	CostType     int64
	IconPath     string
	FieldChanges map[string]FieldChange
	ItemChange   *ItemChange
}

// Session is the platform-independent source of truth for one loaded save and
// its pending mutations. UI packages may retain selections and widgets, but
// staged edits live here.
type Session struct {
	Catalog *catalog.Catalog

	PendingEdits            map[int64]*RowEdit
	PendingFlagEdits        map[int]map[int64]bool
	PendingBellBearingEdits map[int]map[uint32]bool
}

// NewSession creates an empty editing session around a catalog service.
func NewSession(cat *catalog.Catalog) *Session {
	return &Session{
		Catalog:                 cat,
		PendingEdits:            make(map[int64]*RowEdit),
		PendingFlagEdits:        make(map[int]map[int64]bool),
		PendingBellBearingEdits: make(map[int]map[uint32]bool),
	}
}

// ClearPending discards every staged save mutation.
func (session *Session) ClearPending() {
	session.PendingEdits = make(map[int64]*RowEdit)
	session.PendingFlagEdits = make(map[int]map[int64]bool)
	session.PendingBellBearingEdits = make(map[int]map[uint32]bool)
}
