package gio

// Error reporting: unexpected failures append full context to a log file under
// the OS temp dir (the folder holding a portable exe is often not writable)
// and surface a modal the user can dismiss. Ported from app/gui/errors.py.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	platformpaths "github.com/DrippingSoup22/ER_merchant_editor/internal/platform/paths"
)

// LogPath is where unexpected errors and panics are appended.
var LogPath = platformpaths.LogFile()

// appendLog best-effort appends a timestamped entry with a stack trace to
// LogPath. Never fails the caller.
func appendLog(context string, detail string) {
	_ = os.MkdirAll(filepath.Dir(LogPath), 0o755)
	f, err := os.OpenFile(LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n%s\n%s\n\n",
		time.Now().Format(time.RFC3339), context, detail, debug.Stack())
}

// LogPanic records a panic value to the log file and returns the log path (for
// the crash message). Used by cmd/editor's last-resort recover.
func LogPanic(r any) string {
	appendLog("panic", fmt.Sprint(r))
	return LogPath
}

// reportError logs an unexpected error and raises the error modal.
func (s *State) reportError(context string, err error) {
	appendLog(context, err.Error())
	s.mu.Lock()
	s.modalErr = fmt.Sprintf("%s: %v\n\nDetails were written to:\n%s", context, err, LogPath)
	s.mu.Unlock()
	s.invalidate()
}

// ModalErr returns the current modal error text ("" = no modal).
func (s *State) ModalErr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modalErr
}

// dismissModal closes the error modal.
func (s *State) dismissModal() {
	s.mu.Lock()
	s.modalErr = ""
	s.mu.Unlock()
}
