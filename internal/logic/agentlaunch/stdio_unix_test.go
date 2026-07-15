//go:build linux || darwin

package agentlaunch

import (
	"strings"
	"testing"
)

func TestPrepareStdioAppliesNVMPolicyForServiceEnvironment(t *testing.T) {
	command, args := PrepareStdio("gemini", []string{"--acp"}, true)
	if command != "bash" || len(args) != 2 {
		t.Fatalf("unexpected launch: command=%q args=%v", command, args)
	}
	if !strings.Contains(args[1], "NVM_DIR") || !strings.Contains(args[1], `"gemini" "--acp"`) {
		t.Fatalf("NVM launch policy missing: %q", args[1])
	}
}
