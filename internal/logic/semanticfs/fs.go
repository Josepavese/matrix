package semanticfs

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"strings"
	"time"
)

func (s *FS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) || strings.Contains(name, "\\") {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	opened, err := s.open(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	switch f := opened.(type) {
	case *file:
		// This is a live projection, not a physical file with a filesystem mtime.
		// Zero keeps HTTP from returning a stale 304 based on the view's epoch.
		f.info.modified = time.Time{}
	case *dirFile:
		f.info.modified = s.epoch
	}
	return opened, nil
}

func (s *FS) open(name string) (fs.File, error) {
	if name == "." {
		return s.directory(".", []string{"agents", "runs", "workspaces"}), nil
	}
	parts := strings.Split(name, "/")
	if len(parts) == 1 {
		return s.category(parts[0])
	}
	id, err := decodeID(parts[1])
	if err != nil {
		return nil, err
	}
	if len(parts) == 2 {
		return s.entityDirectory(parts[0], id, parts[1])
	}
	if len(parts) != 3 {
		return nil, fs.ErrNotExist
	}
	return s.entityFile(parts[0], id, parts[2])
}

func jsonFile(name string, value any) (fs.File, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return newFile(name, append(data, '\n')), nil
}

type file struct {
	*bytes.Reader
	info info
}

func newFile(name string, data []byte) *file {
	return &file{Reader: bytes.NewReader(data), info: info{name: name, size: int64(len(data)), modified: time.Now().UTC()}}
}
func (f *file) Stat() (fs.FileInfo, error) { return f.info, nil }
func (*file) Close() error                 { return nil }

type info struct {
	name     string
	size     int64
	dir      bool
	modified time.Time
}

func (i info) Name() string { return i.name }
func (i info) Size() int64  { return i.size }
func (i info) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0500
	}
	return 0400
}
func (i info) ModTime() time.Time         { return i.modified }
func (i info) IsDir() bool                { return i.dir }
func (i info) Sys() any                   { return nil }
func (i info) Type() fs.FileMode          { return i.Mode().Type() }
func (i info) Info() (fs.FileInfo, error) { return i, nil }
