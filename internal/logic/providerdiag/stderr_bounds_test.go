package providerdiag

import (
	"strings"
	"testing"
)

func TestStderrHugeLineAndRepeatedRetriesAreBounded(t *testing.T) {
	w := NewStderrCapture(8192, "test")
	large := []byte(strings.Repeat("x", 1<<20))
	if n, err := w.Write(large); err != nil || n != len(large) {
		t.Fatalf("write: %d %v", n, err)
	}
	if len(w.line) > maxLineBytes || len(w.data) > 8192 {
		t.Fatal("unbounded raw stderr retained")
	}
	if _, err := w.Write([]byte("\n" + strings.Repeat("retry failed\n", 10000))); err != nil {
		t.Fatal(err)
	}
	if w.logged != maxLogLines || w.suppressed == 0 {
		t.Fatalf("unbounded process logging: %+v", w)
	}
	if got := w.Sanitized(); len(got) > MaxStderr || !strings.HasSuffix(got, "...") {
		t.Fatalf("silent truncation: %q", got)
	}
	w.Flush()
	if w.suppressed != 0 {
		t.Fatal("suppression summary was not drained")
	}
}
