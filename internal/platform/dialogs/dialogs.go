// Package dialogs provides the desktop file-picker adapter. Keeping this
// behind Service lets Linux move to XDG portals and macOS to a bundled native
// implementation without changing application or view logic.
package dialogs

import "github.com/ncruces/zenity"

// ErrCanceled identifies a user-dismissed dialog.
var ErrCanceled = zenity.ErrCanceled

// Service selects input and output save paths.
type Service interface {
	OpenSave() (string, error)
	SaveAs(suggested string) (string, error)
}

// Native is the current cross-platform desktop implementation.
type Native struct{}

// NewNative constructs the default desktop dialog service.
func NewNative() Native { return Native{} }

// OpenSave asks the user for a decrypted Elden Ring save.
func (Native) OpenSave() (string, error) {
	return zenity.SelectFile(
		zenity.Title("Open Elden Ring save file"),
		zenity.FileFilters{
			// .dat is the decrypted PlayStation export, .sl2 the PC save.
			// The container is detected from its magic, never from the
			// extension, so these patterns are a convenience only.
			{Name: "Save files", Patterns: []string{"*.dat", "*.sl2"}, CaseFold: true},
			{Name: "All files", Patterns: []string{"*"}},
		},
	)
}

// SaveAs asks the user where to write a new edited save.
func (Native) SaveAs(suggested string) (string, error) {
	return zenity.SelectFileSave(
		zenity.Title("Save edited file as"),
		zenity.Filename(suggested),
		zenity.ConfirmOverwrite(),
		zenity.FileFilters{
			// .dat is the decrypted PlayStation export, .sl2 the PC save.
			// The container is detected from its magic, never from the
			// extension, so these patterns are a convenience only.
			{Name: "Save files", Patterns: []string{"*.dat", "*.sl2"}, CaseFold: true},
			{Name: "All files", Patterns: []string{"*"}},
		},
	)
}
