package main

import (
	"archive/zip"
	"os"
	"path/filepath"
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
		if entry.Mode().Perm() != 0o755 {
			t.Fatalf("executable mode = %o, want 755", entry.Mode().Perm())
		}
		return
	}
	t.Fatalf("archive is missing %q", wantName)
}
