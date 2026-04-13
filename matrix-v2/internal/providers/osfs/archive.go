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
			_ = os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
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
	}
	return nil
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

	// Sanitize mode: strip setuid/setgid/sticky bits to prevent privilege escalation
	safeMode := os.FileMode(header.Mode) &^ 0o7000

	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, 0755)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, safeMode)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if _, err := io.Copy(f, tr); err != nil {
			return err
		}
	}
	return nil
}
