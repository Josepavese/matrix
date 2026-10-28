package agentstore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var errReadOnlyStore = errors.New("agent store is open read-only; reopen it with OpenPublisher to mutate it")

// ErrLayoutWritable is returned when a store layout is writable by the current
// user. A consumer refuses to open such a store, because a runtime that can
// write the layout can swap a verified generation after the check. The failure
// says the store's role is wrong; it says nothing about the bytes.
var ErrLayoutWritable = errors.New("agent store layout is writable by the current user, so a verified generation can still be swapped")

// Store is an opened agent store. The root is absolute and symlink-resolved;
// readOnly is set by OpenConsumer and refuses every mutation.
type Store struct {
	root     string
	readOnly bool
}

// OpenPublisher validates the root, creates the store layout with private
// (0700) directories and returns a store that may publish and garbage-collect.
func OpenPublisher(root, palHome string) (*Store, error) {
	resolved, err := ValidateRoot(root, palHome)
	if err != nil {
		return nil, err
	}
	store := &Store{root: resolved}
	if err := store.ensureLayout(); err != nil {
		return nil, err
	}
	return store, nil
}

// OpenConsumer validates the root and the layout and then fails closed unless
// the layout directories are read-only for the runtime user: a verified
// generation is only trustworthy when the runtime cannot swap its bytes.
func OpenConsumer(root, palHome string) (*Store, error) {
	resolved, err := ValidateRoot(root, palHome)
	if err != nil {
		return nil, err
	}
	store := &Store{root: resolved, readOnly: true}
	if err := store.requireLayout(); err != nil {
		return nil, err
	}
	if err := store.requireReadOnly(); err != nil {
		return nil, err
	}
	return store, nil
}

// OpenInspection opens a store to read and verify its generations without
// claiming a trust boundary: it validates the root and requires a usable
// layout, but it neither checks nor changes permissions. It exists so a report
// can tell "the store's role is wrong" apart from "the bytes are wrong", which
// are different facts with different remedies. It never mutates the store and
// it must not be used to install or to run software.
func OpenInspection(root, palHome string) (*Store, error) {
	resolved, err := ValidateRoot(root, palHome)
	if err != nil {
		return nil, err
	}
	store := &Store{root: resolved, readOnly: true}
	if err := store.requireLayout(); err != nil {
		return nil, err
	}
	return store, nil
}

// Root is the absolute, symlink-resolved store root.
func (s *Store) Root() string { return s.root }

func (s *Store) blobsDir() string       { return filepath.Join(s.root, blobsDirName, sha256DirName) }
func (s *Store) generationsDir() string { return filepath.Join(s.root, generationsDirName) }
func (s *Store) tmpDir() string         { return filepath.Join(s.root, tmpDirName) }

func (s *Store) generationPath(digest string) string {
	return filepath.Join(s.generationsDir(), digest+".json")
}

func (s *Store) manifestPath(digest string) string {
	return filepath.Join(s.generationsDir(), digest+".manifest.json")
}

// layoutDirs are the directories a writer would have to own in order to swap
// published software. Consumers require every one of them to be sealed.
func (s *Store) layoutDirs() []string {
	return []string{s.root, filepath.Join(s.root, blobsDirName), s.blobsDir(), s.generationsDir()}
}

func (s *Store) ensureLayout() error {
	for _, dir := range append(s.layoutDirs(), s.tmpDir()) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create agent store directory %s: %w", dir, err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("protect agent store directory %s: %w", dir, err)
		}
	}
	return nil
}

func (s *Store) requireLayout() error {
	for _, dir := range s.layoutDirs() {
		if err := requireReadableDir(dir); err != nil {
			return fmt.Errorf("agent store layout is not usable: %w", err)
		}
	}
	return nil
}

func requireReadableDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("required directory %s is unavailable: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("required path %s is not a directory", path)
	}
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("required directory %s is not readable: %w", path, err)
	}
	defer func() { _ = dir.Close() }()
	if _, err := dir.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("required directory %s is not readable: %w", path, err)
	}
	return nil
}

func (s *Store) requireReadOnly() error {
	for _, dir := range s.layoutDirs() {
		if writableByCurrentUser(dir) {
			return fmt.Errorf("%w: publish from a different account or mount the store read-only for the runtime user (first offending directory: %s)", ErrLayoutWritable, dir)
		}
	}
	return nil
}

// writableByCurrentUser reports whether the current process can still modify
// path. An owner that cannot be resolved counts as writable: the check fails
// closed rather than trusting a mode bit it cannot attribute.
func writableByCurrentUser(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	ownerUID, ownerKnown := fileOwnerUID(info)
	return writableByMode(info.Mode().Perm(), ownerUID, ownerKnown, currentEUID())
}

// writableByMode is the pure form of the writability decision. A group-writable
// directory counts as writable because a supplementary-group grant cannot be
// proven from the process, and an owner-writable directory counts only when its
// owner is the current user.
func writableByMode(perm os.FileMode, ownerUID uint32, ownerKnown bool, euid uint32) bool {
	if perm&0o002 != 0 {
		return true
	}
	if perm&0o020 != 0 {
		return true
	}
	if !ownerKnown {
		return perm&0o200 != 0
	}
	return perm&0o200 != 0 && ownerUID == euid
}
