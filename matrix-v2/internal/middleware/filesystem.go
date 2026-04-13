package middleware

import (
	"io"
	"os"
)

// File abstracts a file handle for reading, writing, seeking and closing.
type File interface {
	io.Reader
	io.Writer
	io.Seeker
	Close() error
}

// FS defines the Filesystem abstraction layer interface
type FS interface {
	Mount(dir string) error
	Unmount() error
	CreateDirectory(path string) error

	// General operations
	RemoveAll(path string) error
	Stat(path string) (os.FileInfo, error)
	MkdirAll(path string, perm os.FileMode) error
	UserHomeDir() (string, error)
	TempDir() string

	// File-level operations
	Open(path string) (File, error)
	OpenFile(path string, flag int, perm os.FileMode) (File, error)
	ReadFile(path string) ([]byte, error)
	Remove(path string) error
	Rename(oldpath, newpath string) error
}
