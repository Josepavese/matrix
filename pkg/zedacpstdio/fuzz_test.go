package zedacpstdio

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------------
// Fuzz target for the frame reader
// ----------------------------------------------------------------------------
//
// This reader sits in front of every ACP message Matrix exchanges with an agent
// process. A bug here corrupts or truncates protocol frames, so the property is
// exact: whatever comes back must be a prefix-preserving slice of the input up
// to and including the first newline, or a clean error.

// FuzzReadBoundedLine asserts the reader never loses, duplicates or reorders
// bytes, and always stops at the first newline or the size limit.
func FuzzReadBoundedLine(f *testing.F) {
	seeds := []string{
		"{\"jsonrpc\":\"2.0\"}\n",
		"no newline at all",
		"\n",
		"a\nb\n",
		"\r\n",
		strings.Repeat("x", 100) + "\n",
		strings.Repeat("x", 5000) + "\nrest",
		"\x00\x01\x02\n",
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		const limit = 4096
		reader := bufio.NewReaderSize(bytes.NewReader(data), 64)
		line, err := readBoundedLine(reader, limit)
		if err != nil {
			// Two failures are legitimate: the explicit size error, and a stream
			// that ends without a newline (io.EOF), which is how a truncated
			// frame surfaces.
			if errors.Is(err, io.EOF) {
				if len(line) != 0 {
					t.Fatalf("EOF with partial data returned: %q", line)
				}
				return
			}
			if !errors.Is(err, ErrFrameTooLarge) {
				t.Fatalf("unexpected error for %d bytes: %v", len(data), err)
			}
			return
		}
		if len(line) > limit {
			t.Fatalf("reader returned %d bytes above the limit %d", len(line), limit)
		}
		// The result must be a prefix of the input.
		if !bytes.HasPrefix(data, line) {
			t.Fatalf("returned bytes are not a prefix of the input: %q vs %q", line, data)
		}
		// It must include the terminating newline, and that newline must be the
		// first one in the input.
		if len(line) == 0 || line[len(line)-1] != '\n' {
			t.Fatalf("returned line is not newline-terminated: %q", line)
		}
		if first := bytes.IndexByte(data, '\n'); first != len(line)-1 {
			t.Fatalf("returned line does not stop at the first newline: %q (first newline at %d)", line, first)
		}
	})

	// Inputs with no newline must fail rather than block or truncate silently.
	f.Add([]byte(strings.Repeat("y", 9000)))
}
