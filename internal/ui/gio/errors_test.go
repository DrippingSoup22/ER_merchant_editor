package gio

// Tests for the error-reporting round-trip (reportError -> ModalErr ->
// dismissModal) and appendLog's best-effort file append. All are
// mu-guarded (see errors.go / state.go's concurrency contract); the public
// ModalErr()/dismissModal() accessors take the lock themselves, so the test
// never touches s.modalErr directly.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempLogPath points the package-level LogPath at a throwaway file for
// the duration of a test, restoring it afterward, so appendLog doesn't write
// to the real OS temp dir.
func withTempLogPath(t *testing.T) string {
	t.Helper()
	prev := LogPath
	p := filepath.Join(t.TempDir(), "gio.log")
	LogPath = p
	t.Cleanup(func() { LogPath = prev })
	return p
}

// TestReportErrorRoundTrip: reportError surfaces the modal (with the context,
// error, and log path in the text) and writes a log entry; dismissModal
// clears the modal text.
func TestReportErrorRoundTrip(t *testing.T) {
	logPath := withTempLogPath(t)
	s := &State{}

	if got := s.ModalErr(); got != "" {
		t.Fatalf("ModalErr() before any error = %q, want empty", got)
	}

	s.reportError("loading save", errors.New("boom"))

	got := s.ModalErr()
	if got == "" {
		t.Fatal("ModalErr() empty after reportError, want the modal text")
	}
	for _, want := range []string{"loading save", "boom", logPath} {
		if !strings.Contains(got, want) {
			t.Errorf("ModalErr() = %q, want it to contain %q", got, want)
		}
	}

	// The error must also have been appended to the log file.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "loading save") || !strings.Contains(body, "boom") {
		t.Errorf("log file = %q, want it to contain the context and error", body)
	}

	s.dismissModal()
	if got := s.ModalErr(); got != "" {
		t.Errorf("ModalErr() after dismissModal = %q, want empty", got)
	}
}

// TestAppendLogCreatesAndAppends: appendLog creates the log file if missing
// and appends (never truncates) successive entries, each with its context and
// detail.
func TestAppendLogCreatesAndAppends(t *testing.T) {
	logPath := withTempLogPath(t)

	appendLog("first context", "first detail")
	appendLog("second context", "second detail")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	body := string(data)
	for _, want := range []string{"first context", "first detail", "second context", "second detail"} {
		if !strings.Contains(body, want) {
			t.Errorf("log file missing %q; got:\n%s", want, body)
		}
	}
	// The first entry must survive the second append (append, not truncate).
	if strings.Index(body, "first context") > strings.Index(body, "second context") {
		t.Error("entries out of order: the first append must precede the second")
	}
}
