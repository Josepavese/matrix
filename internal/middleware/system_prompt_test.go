package middleware

import (
	"strings"
	"testing"
)

// TestSystemPromptValidationKeepsTheWireModesClosed: the stable wire admits
// append and override today, with future modes reserved upstream. Matrix must
// not write a mode it cannot name into the vault, and an empty body means no
// prompt rather than an empty instruction set on the wire.
func TestSystemPromptValidationKeepsTheWireModesClosed(t *testing.T) {
	cases := []struct {
		name    string
		prompt  SystemPrompt
		wantErr string
	}{
		{name: "unset", prompt: SystemPrompt{}},
		{name: "append default", prompt: SystemPrompt{Text: "be concise"}},
		{name: "explicit append", prompt: SystemPrompt{Mode: SystemPromptModeAppend, Text: "be concise"}},
		{name: "override", prompt: SystemPrompt{Mode: SystemPromptModeOverride, Text: "be concise"}},
		{name: "whitespace body", prompt: SystemPrompt{Text: " \n\t "}, wantErr: "whitespace"},
		{name: "unknown mode", prompt: SystemPrompt{Mode: "replace", Text: "be concise"}, wantErr: "unsupported system prompt mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSystemPrompt(tc.prompt)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestSystemPromptEffectiveModeDefaultsToAppend(t *testing.T) {
	if got := (SystemPrompt{Text: "x"}).EffectiveMode(); got != SystemPromptModeAppend {
		t.Fatalf("default mode = %q, want append", got)
	}
	if got := (SystemPrompt{Mode: SystemPromptModeOverride, Text: "x"}).EffectiveMode(); got != SystemPromptModeOverride {
		t.Fatalf("explicit mode = %q, want override", got)
	}
	if !(SystemPrompt{}).IsZero() || !(SystemPrompt{Text: " "}).IsZero() || (SystemPrompt{Text: "x"}).IsZero() {
		t.Fatal("IsZero must follow the instruction body, not the mode")
	}
}
