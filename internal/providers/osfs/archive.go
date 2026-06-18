// Package osfs provides OS-based filesystem and configuration providers.
package osfs

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ArchiveProvider implements middleware.Archive for local filesystem.
type ArchiveProvider struct{}

// NewArchiveProvider returns a new Archive provider.
func NewArchiveProvider() *ArchiveProvider {
	return &ArchiveProvider{}
}

// Extract uncompresses a .zip, .tar.gz, or .tar.bz2 file from src to dest directory.
func (p *ArchiveProvider) Extract(src, dest string) error {
	if strings.HasSuffix(src, ".zip") {
		return extractZip(src, dest)
	}
	if strings.HasSuffix(src, ".tar.gz") || strings.HasSuffix(src, ".tgz") {
		return extractTarGz(src, dest)
	}
	if strings.HasSuffix(src, ".tar.bz2") || strings.HasSuffix(src, ".tbz2") {
		return extractTarBz2(src, dest)
	}
	return fmt.Errorf("unsupported archive format: %s", src)
}

// isSafePath checks that candidate is within dest (prevents path traversal / zip slip).
func isSafePath(dest, candidate string) bool {
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return false
	}
	// Ensure the candidate starts with the destination path + separator
	prefix := absDest + string(filepath.Separator)
	return strings.HasPrefix(abs, prefix)
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		if !isSafePath(dest, fpath) {
			slog.Warn("archive entry escapes destination, skipping", "entry", f.Name, "dest", dest)
			continue
		}

		if f.FileInfo().IsDir() {
			if err := mkdirArchiveDir(fpath); err != nil {
				return err
			}
			continue
		}

		if !f.FileInfo().Mode().IsRegular() {
			slog.Warn("unsupported zip entry type, skipping", "entry", f.Name, "mode", f.FileInfo().Mode().String())
			continue
		}

		if err := extractZipFile(f, fpath); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, fpath string) error {
	if err := os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
		return err
	}

	safeMode := safeArchiveFileMode(f.Mode())
	outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, safeMode)
	if err != nil {
		return err
	}

	rc, err := f.Open()
	if err != nil {
		_ = outFile.Close()
		return err
	}

	_, err = io.Copy(outFile, rc)
	_ = outFile.Close()
	_ = rc.Close()
	if err != nil {
		return err
	}
	return os.Chmod(fpath, safeMode)
}

func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()

	return extractTar(dest, tar.NewReader(gzr))
}

func extractTarBz2(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return extractTar(dest, tar.NewReader(bzip2.NewReader(f)))
}

func extractTar(dest string, tr *tar.Reader) error {
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := extractTarEntry(dest, header, tr); err != nil {
			return err
		}
	}
	return nil
}

func extractTarEntry(dest string, header *tar.Header, tr *tar.Reader) error {
	target := filepath.Join(dest, header.Name)

	if !isSafePath(dest, target) {
		slog.Warn("tar entry escapes destination, skipping", "entry", header.Name, "dest", dest)
		return nil
	}

	switch header.Typeflag {
	case tar.TypeDir:
		return mkdirArchiveDir(target)
	case tar.TypeReg, 0:
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		safeMode := safeArchiveFileMode(os.FileMode(header.Mode))
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, safeMode)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if _, err := io.Copy(f, tr); err != nil {
			return err
		}
		return os.Chmod(target, safeMode)
	default:
		slog.Warn("unsupported tar entry type, skipping", "entry", header.Name, "type", header.Typeflag)
	}
	return nil
}

func mkdirArchiveDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
}

func safeArchiveFileMode(mode os.FileMode) os.FileMode {
	mode &^= 0o7000
	mode &^= 0o022
	if mode&0o111 != 0 {
		return 0o755
	}
	return 0o644
}
