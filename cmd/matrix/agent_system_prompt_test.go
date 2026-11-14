package main

import (
	"strings"
	"testing"
)

// TestReadSystemPromptPreservesTheBodyAndDropsOneShellNewline: stdin is a
// transport, not part of the instruction. One trailing newline (an echo or a
// here-document terminator) is removed; every other byte, including inner
// newlines and surrounding spaces, is kept as written.
func TestReadSystemPromptPreservesTheBodyAndDropsOneShellNewline(t *testing.T) {
	cases := map[string]string{
		"plain\n":              "plain",
		"  spaced  \n":         "  spaced  ",
		"line one\nline two\n": "line one\nline two",
		"line one\nline two":   "line one\nline two",
		"double trailing\n\n":  "double trailing\n",
	}
	for input, want := range cases {
		if got := readSystemPrompt(strings.NewReader(input)); got != want {
			t.Fatalf("readSystemPrompt(%q) = %q, want %q", input, got, want)
		}
	}
}
