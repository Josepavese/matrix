// Package oslog provides a log sink that writes structured logs using the OS logging subsystem.
package oslog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Josepavese/matrix/internal/middleware"
)

// Factory creates log sinks backed by the local OS.
type Factory struct{}

// NewFactory returns a new oslog Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// Build creates a log sink based on the provided options.
func (f *Factory) Build(options middleware.LogSinkOptions) (middleware.LogSink, error) {
	switch options.Target {
	case "stderr":
		return &stderrSink{}, nil
	case "file":
		return newFileSink(options.FilePath, options.MaxBytes, options.MaxBackups)
	case "both":
		fileSink, err := newFileSink(options.FilePath, options.MaxBytes, options.MaxBackups)
		if err != nil {
			return nil, err
		}
		return &multiSink{
			writer:     io.MultiWriter(fileSink.Writer(), os.Stderr),
			closers:    []io.Closer{fileSink},
			descriptor: fmt.Sprintf("file+stderr(%s)", options.FilePath),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported log sink target %q", options.Target)
	}
}

type fileSink struct {
	mu         sync.Mutex
	path       string
	file       *os.File
	size       int64
	maxBytes   int64
	maxBackups int
}

func newFileSink(path string, maxBytes int64, maxBackups int) (*fileSink, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to stat log file %s: %w", path, err)
	}
	return &fileSink{
		path:       path,
		file:       f,
		size:       info.Size(),
		maxBytes:   maxBytes,
		maxBackups: maxBackups,
	}, nil
}

func (s *fileSink) Writer() io.Writer {
	return s
}

func (s *fileSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.maxBytes > 0 && s.size+int64(len(p)) > s.maxBytes {
		if err := s.rotate(); err != nil {
			return 0, err
		}
	}

	n, err := s.file.Write(p)
	s.size += int64(n)
	return n, err
}

func (s *fileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file.Close()
}

func (s *fileSink) Descriptor() string {
	return s.path
}

func (s *fileSink) rotate() error {
	if err := s.file.Close(); err != nil {
		return fmt.Errorf("failed to close log file before rotation: %w", err)
	}

	for i := s.maxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", s.path, i)
		dst := fmt.Sprintf("%s.%d", s.path, i+1)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, dst); err != nil {
				return fmt.Errorf("failed to rotate backup %s -> %s: %w", src, dst, err)
			}
		}
	}

	if _, err := os.Stat(s.path); err == nil {
		if err := os.Rename(s.path, s.path+".1"); err != nil {
			return fmt.Errorf("failed to rotate active log file: %w", err)
		}
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to reopen rotated log file %s: %w", s.path, err)
	}

	s.file = f
	s.size = 0
	return nil
}

type stderrSink struct{}

func (s *stderrSink) Writer() io.Writer {
	return os.Stderr
}

func (s *stderrSink) Close() error {
	return nil
}

func (s *stderrSink) Descriptor() string {
	return "stderr"
}

type multiSink struct {
	writer     io.Writer
	closers    []io.Closer
	descriptor string
}

func (s *multiSink) Writer() io.Writer {
	return s.writer
}

func (s *multiSink) Close() error {
	var firstErr error
	for _, closer := range s.closers {
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *multiSink) Descriptor() string {
	return s.descriptor
}
