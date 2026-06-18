package osfs

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractZipClampsRiskyModes(t *testing.T) {
	src := filepath.Join(t.TempDir(), "payload.zip")
	if err := writeZipFixture(src); err != nil {
		t.Fatalf("write zip fixture: %v", err)
	}

	dest := t.TempDir()
	if err := NewArchiveProvider().Extract(src, dest); err != nil {
		t.Fatalf("extract zip: %v", err)
	}

	assertPerm(t, filepath.Join(dest, "bin"), 0o755)
	assertPerm(t, filepath.Join(dest, "bin", "run.sh"), 0o755)
	assertPerm(t, filepath.Join(dest, "data.txt"), 0o644)
	assertNotExists(t, filepath.Join(dest, "link"))
}

func TestExtractZipRejectsExistingSymlinkEscape(t *testing.T) {
	src := filepath.Join(t.TempDir(), "payload.zip")
	if err := writeZipFixture(src); err != nil {
		t.Fatalf("write zip fixture: %v", err)
	}

	dest := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dest, "bin")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if err := NewArchiveProvider().Extract(src, dest); err == nil {
		t.Fatalf("expected extraction to reject symlink escape")
	}
	if _, err := os.Stat(filepath.Join(outside, "run.sh")); !os.IsNotExist(err) {
		t.Fatalf("archive wrote through symlink, err=%v", err)
	}
}

func TestExtractTarGzClampsRiskyModes(t *testing.T) {
	src := filepath.Join(t.TempDir(), "payload.tar.gz")
	if err := writeTarGzFixture(src); err != nil {
		t.Fatalf("write tar fixture: %v", err)
	}

	dest := t.TempDir()
	if err := NewArchiveProvider().Extract(src, dest); err != nil {
		t.Fatalf("extract tar.gz: %v", err)
	}

	assertPerm(t, filepath.Join(dest, "bin"), 0o755)
	assertPerm(t, filepath.Join(dest, "bin", "run.sh"), 0o755)
	assertPerm(t, filepath.Join(dest, "data.txt"), 0o644)
	assertNotExists(t, filepath.Join(dest, "link"))
}

func TestExtractTarGzRejectsExistingSymlinkEscape(t *testing.T) {
	src := filepath.Join(t.TempDir(), "payload.tar.gz")
	if err := writeTarGzFixture(src); err != nil {
		t.Fatalf("write tar fixture: %v", err)
	}

	dest := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dest, "bin")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if err := NewArchiveProvider().Extract(src, dest); err == nil {
		t.Fatalf("expected extraction to reject symlink escape")
	}
	if _, err := os.Stat(filepath.Join(outside, "run.sh")); !os.IsNotExist(err) {
		t.Fatalf("archive wrote through symlink, err=%v", err)
	}
}

func writeZipFixture(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	dir := &zip.FileHeader{Name: "bin/"}
	dir.SetMode(os.ModeDir | 0o777)
	if _, err := zw.CreateHeader(dir); err != nil {
		_ = zw.Close()
		return err
	}
	run := &zip.FileHeader{Name: "bin/run.sh"}
	run.SetMode(0o4777)
	w, err := zw.CreateHeader(run)
	if err != nil {
		_ = zw.Close()
		return err
	}
	if _, err := w.Write([]byte("#!/bin/sh\n")); err != nil {
		_ = zw.Close()
		return err
	}
	data := &zip.FileHeader{Name: "data.txt"}
	data.SetMode(0o666)
	w, err = zw.CreateHeader(data)
	if err != nil {
		_ = zw.Close()
		return err
	}
	if _, err := w.Write([]byte("data\n")); err != nil {
		_ = zw.Close()
		return err
	}
	link := &zip.FileHeader{Name: "link"}
	link.SetMode(os.ModeSymlink | 0o777)
	w, err = zw.CreateHeader(link)
	if err != nil {
		_ = zw.Close()
		return err
	}
	if _, err := w.Write([]byte("data.txt")); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

func writeTarGzFixture(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: "bin", Typeflag: tar.TypeDir, Mode: 0o777}); err != nil {
		_ = tw.Close()
		_ = gw.Close()
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "bin/run.sh", Typeflag: tar.TypeReg, Mode: 0o4777, Size: int64(len("#!/bin/sh\n"))}); err != nil {
		_ = tw.Close()
		_ = gw.Close()
		return err
	}
	if _, err := tw.Write([]byte("#!/bin/sh\n")); err != nil {
		_ = tw.Close()
		_ = gw.Close()
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "data.txt", Typeflag: tar.TypeReg, Mode: 0o666, Size: int64(len("data\n"))}); err != nil {
		_ = tw.Close()
		_ = gw.Close()
		return err
	}
	if _, err := tw.Write([]byte("data\n")); err != nil {
		_ = tw.Close()
		_ = gw.Close()
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Mode: 0o777, Linkname: "data.txt"}); err != nil {
		_ = tw.Close()
		_ = gw.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		_ = gw.Close()
		return err
	}
	return gw.Close()
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("expected %s mode %o, got %o", path, want, got)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be skipped, err=%v", path, err)
	}
}
