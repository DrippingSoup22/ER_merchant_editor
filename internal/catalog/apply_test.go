package catalog

// ApplyEdits validation tests. Uses the fixture save (gitignored -> CI skips).

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/savefile"
)

func loadedCatalog(t *testing.T) *Catalog {
	t.Helper()
	if _, err := os.Stat(fixtureSave); err != nil {
		t.Skipf("fixture save absent, skipping: %v", err)
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.LoadSave(fixtureSave); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestApplyEditsRejectsMaterialLocked(t *testing.T) {
	c := loadedCatalog(t)
	rows, err := c.ShopRows()
	if err != nil {
		t.Fatal(err)
	}
	var locked int64 = -1
	for _, r := range rows {
		if r.MaterialLocked {
			locked = r.RowID
			break
		}
	}
	if locked == -1 {
		t.Fatal("no material-locked row in fixture")
	}

	edits := []savefile.Edit{{RowID: locked, Fields: map[string]json.Number{"value": "1"}}}
	out := filepath.Join(t.TempDir(), "out.dat")
	_, err = c.ApplyEdits(edits, out)
	var ee *EditError
	if !errors.As(err, &ee) {
		t.Fatalf("want *EditError, got %v", err)
	}
	// Nothing written, save path unchanged.
	if abs, _ := filepath.Abs(fixtureSave); c.SavePath() != fixtureSave && c.SavePath() != abs {
		t.Errorf("save path changed after rejected edit: %q", c.SavePath())
	}
}

func TestApplyEditsRejectsSameOutPath(t *testing.T) {
	c := loadedCatalog(t)
	edits := []savefile.Edit{{RowID: 1, Fields: map[string]json.Number{"value": "1"}}}
	_, err := c.ApplyEdits(edits, fixtureSave)
	var ee *EditError
	if !errors.As(err, &ee) {
		t.Fatalf("want *EditError for out == current save, got %v", err)
	}
}

func TestApplyEditsRejectsEmpty(t *testing.T) {
	c := loadedCatalog(t)
	_, err := c.ApplyEdits(nil, filepath.Join(t.TempDir(), "out.dat"))
	var ee *EditError
	if !errors.As(err, &ee) {
		t.Fatalf("want *EditError for empty edits, got %v", err)
	}
}
