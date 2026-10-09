package runclient

import (
	"fmt"
	"os"
	"strings"
)

// PromptText resolves the prompt from the flag or the file, refusing an
// empty prompt: a run with no input is accepted by the wire contract and then
// does nothing, which reads as a broken agent rather than a malformed command.
func PromptText(prompt, promptFile string) (string, error) {
	inline := strings.TrimSpace(prompt)
	file := strings.TrimSpace(promptFile)
	switch {
	case inline != "" && file != "":
		return "", fmt.Errorf("pass either --prompt or --prompt-file, not both")
	case file != "":
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read prompt file: %w", err)
		}
		if strings.TrimSpace(string(raw)) == "" {
			return "", fmt.Errorf("prompt file %s is empty", file)
		}
		return string(raw), nil
	case inline != "":
		return inline, nil
	default:
		return "", fmt.Errorf("a prompt is required: pass --prompt or --prompt-file")
	}
}
