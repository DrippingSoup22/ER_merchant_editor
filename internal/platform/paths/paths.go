// Package paths resolves writable per-user locations used by the desktop app.
package paths

import (
	"os"
	"path/filepath"
)

const directoryName = "er_merchant_editor"

// ConfigFile returns the platform-appropriate settings path.
func ConfigFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, directoryName, "config.json"), nil
}

// LogFile returns a writable diagnostic-log path.
func LogFile() string {
	return filepath.Join(os.TempDir(), directoryName, "editor.log")
}
