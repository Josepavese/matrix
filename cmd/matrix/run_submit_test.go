package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSubmitPromptRefusesWhatItCannotSend keeps an empty prompt from becoming a
// successful run with no output, which reads as a broken agent.
func TestSubmitPromptRefusesWhatItCannotSend(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(empty, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ why, prompt, file string }{
		{why: "neither source", prompt: "", file: ""},
		{why: "both sources", prompt: "hello", file: empty},
		{why: "empty file", prompt: "", file: empty},
	} {
		if _, err := submitPromptText(test.prompt, test.file); err == nil {
			t.Fatalf("%s must be refused", test.why)
		}
	}
	got, err := submitPromptText("saluta", "")
	if err != nil || got != "saluta" {
		t.Fatalf("submitPromptText = %q, %v", got, err)
	}
}
