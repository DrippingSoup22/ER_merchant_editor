package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCreateArchiveIncludesBundleAndPreservesExecutableMode(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "ER-Merchant-Editor-linux-amd64-1.1.0")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "ERMerchantEditor")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "release.zip")
	if err := createArchive(root, archive); err != nil {
		t.Fatal(err)
	}

	zr, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 2 {
		t.Fatalf("archive entries = %d, want directory and executable", len(zr.File))
	}
	wantName := filepath.Base(root) + "/ERMerchantEditor"
	for _, entry := range zr.File {
		if entry.Name != wantName {
			continue
		}
		// Windows has no POSIX executable bit: os.WriteFile creates this test
		// fixture as 0666 there. Linux must retain 0755 so the extracted GUI
		// remains directly runnable.
		if runtime.GOOS != "windows" && entry.Mode().Perm() != 0o755 {
			t.Fatalf("executable mode = %o, want 755", entry.Mode().Perm())
		}
		return
	}
	t.Fatalf("archive is missing %q", wantName)
}
