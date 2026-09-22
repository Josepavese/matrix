package osfs

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestCopyBoundedStopsAtTheBudget is the decompression-bomb guard: an archive is
// compressed input from outside the process, so an entry that expands past the
// ceiling must be refused rather than written to disk.
func TestCopyBoundedStopsAtTheBudget(t *testing.T) {
	const limit = int64(10)
	budget := limit
	var out bytes.Buffer
	err := copyBounded(&out, strings.NewReader(strings.Repeat("x", 50)), &budget)
	if err == nil {
		t.Fatal("an entry larger than the budget must be refused")
	}
	// The guard reads one byte past the budget to detect the overflow, so at most
	// limit+1 bytes may reach the writer, never the whole payload.
	if int64(out.Len()) > limit+1 {
		t.Fatalf("the writer received %d bytes, the budget was %d", out.Len(), limit)
	}

	// Spending the budget exactly is allowed and leaves it at zero.
	exact := int64(5)
	if err := copyBounded(io.Discard, strings.NewReader("12345"), &exact); err != nil {
		t.Fatalf("an entry that fits must be accepted: %v", err)
	}
	if exact != 0 {
		t.Fatalf("the budget must be charged, got %d", exact)
	}

	// Once spent, any further entry is refused before any bytes are written.
	if err := copyBounded(io.Discard, strings.NewReader("x"), &exact); err == nil {
		t.Fatal("a spent budget must refuse further entries")
	}

	// The budget is shared across entries, which is what bounds the total.
	shared := int64(6)
	if err := copyBounded(io.Discard, strings.NewReader("abcd"), &shared); err != nil {
		t.Fatalf("first entry: %v", err)
	}
	if err := copyBounded(io.Discard, strings.NewReader("abcd"), &shared); err == nil {
		t.Fatal("the second entry must not fit the remaining budget")
	}
}
