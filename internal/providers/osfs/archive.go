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

// maxExtractedBytes bounds the total uncompressed size of one extraction. An
// archive is a compressed input from outside the process, so without a ceiling a
// small file can fill the disk: the limit is generous enough for a real agent
// distribution and turns a decompression bomb into a clear error.
const maxExtractedBytes = int64(2) << 30 // 2 GiB

// copyBounded copies an archive entry while charging it against a shared budget.
func copyBounded(dst io.Writer, src io.Reader, budget *int64) error {
	remaining := *budget
	if remaining <= 0 {
		return fmt.Errorf("archive expands beyond the %d byte extraction limit", maxExtractedBytes)
	}
	written, err := io.Copy(dst, io.LimitReader(src, remaining+1))
	*budget -= written
	if err != nil {
		return err
	}
	if written > remaining {
		return fmt.Errorf("archive expands beyond the %d byte extraction limit", maxExtractedBytes)
	}
	return nil
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
	budget := maxExtractedBytes
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
			if err := ensureSafeArchivePath(dest, fpath); err != nil {
				return err
			}
			if err := mkdirArchiveDir(fpath); err != nil {
				return err
			}
			continue
		}

		if !f.FileInfo().Mode().IsRegular() {
			slog.Warn("unsupported zip entry type, skipping", "entry", f.Name, "mode", f.FileInfo().Mode().String())
			continue
		}

		if err := extractZipFile(f, dest, fpath, &budget); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, dest, fpath string, budget *int64) error {
	if err := ensureSafeArchivePath(dest, filepath.Dir(fpath)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
		return err
	}
	if err := ensureNoSymlink(fpath); err != nil {
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

	err = copyBounded(outFile, rc, budget)
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
	budget := maxExtractedBytes
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := extractTarEntry(dest, header, tr, &budget); err != nil {
			return err
		}
	}
	return nil
}

func extractTarEntry(dest string, header *tar.Header, tr *tar.Reader, budget *int64) error {
	target := filepath.Join(dest, header.Name)

	if !isSafePath(dest, target) {
		slog.Warn("tar entry escapes destination, skipping", "entry", header.Name, "dest", dest)
		return nil
	}

	switch header.Typeflag {
	case tar.TypeDir:
		if err := ensureSafeArchivePath(dest, target); err != nil {
			return err
		}
		return mkdirArchiveDir(target)
	case tar.TypeReg, 0:
		if err := ensureSafeArchivePath(dest, filepath.Dir(target)); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := ensureNoSymlink(target); err != nil {
			return err
		}
		safeMode := safeArchiveFileMode(os.FileMode(header.Mode))
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, safeMode)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if err := copyBounded(f, tr, budget); err != nil {
			return err
		}
		return os.Chmod(target, safeMode)
	default:
		slog.Warn("unsupported tar entry type, skipping", "entry", header.Name, "type", header.Typeflag)
	}
	return nil
}

func mkdirArchiveDir(path string) error {
	if err := ensureNoSymlink(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
}

func ensureSafeArchivePath(dest, target string) error {
	realDest, rel, err := archivePathContext(dest, target)
	if err != nil || rel == "." {
		return err
	}
	return ensureArchiveComponentsSafe(realDest, rel)
}

func archivePathContext(dest, target string) (string, string, error) {
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", "", err
	}
	realDest, err := filepath.EvalSymlinks(absDest)
	if err != nil {
		return "", "", err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(absDest, absTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("archive entry escapes destination")
	}
	return realDest, rel, nil
}

func ensureArchiveComponentsSafe(realDest, rel string) error {
	current := realDest
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		realPath, exists, err := existingArchivePath(current)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		if !isResolvedArchivePathSafe(realDest, realPath) {
			return fmt.Errorf("archive entry crosses symlink outside destination")
		}
		current = realPath
	}
	return nil
}

func existingArchivePath(path string) (string, bool, error) {
	realPath, err := filepath.EvalSymlinks(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return realPath, true, nil
}

func isResolvedArchivePathSafe(dest, candidate string) bool {
	candidate = filepath.Clean(candidate)
	dest = filepath.Clean(dest)
	return candidate == dest || strings.HasPrefix(candidate, dest+string(filepath.Separator))
}

func ensureNoSymlink(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("archive destination is a symlink: %s", path)
	}
	return nil
}

func safeArchiveFileMode(mode os.FileMode) os.FileMode {
	mode &^= 0o7000
	mode &^= 0o022
	if mode&0o111 != 0 {
		return 0o755
	}
	return 0o644
}
