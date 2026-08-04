// Command packagezip creates a release ZIP containing one bundle directory.
// It uses only the Go standard library so packaging behaves the same on every
// build host and preserves executable permission bits for Linux binaries.
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	root := flag.String("root", "", "bundle directory to archive")
	out := flag.String("out", "", "destination .zip path")
	flag.Parse()
	if *root == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "error: -root and -out are required")
		os.Exit(2)
	}
	if err := createArchive(*root, *out); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func createArchive(root, out string) (retErr error) {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("stat bundle: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("bundle root is not a directory: %s", root)
	}

	f, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer func() {
		if err := f.Close(); retErr == nil && err != nil {
			retErr = fmt.Errorf("close archive: %w", err)
		}
		if retErr != nil {
			_ = os.Remove(out)
		}
	}()

	zw := zip.NewWriter(f)
	defer func() {
		if err := zw.Close(); retErr == nil && err != nil {
			retErr = fmt.Errorf("finish archive: %w", err)
		}
	}()

	parent := filepath.Dir(filepath.Clean(root))
	return filepath.Walk(root, func(path string, entry os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(parent, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(entry)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if entry.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, src)
		closeErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
