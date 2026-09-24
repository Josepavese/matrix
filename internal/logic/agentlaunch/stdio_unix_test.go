//go:build linux || darwin

package agentlaunch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareStdioAppliesNVMPolicyForServiceEnvironment(t *testing.T) {
	command, args := PrepareStdio("gemini", []string{"--acp"}, true)
	if command != "bash" || len(args) != 2 {
		t.Fatalf("unexpected launch: command=%q args=%v", command, args)
	}
	if !strings.Contains(args[1], "NVM_DIR") || !strings.Contains(args[1], `'gemini' '--acp'`) {
		t.Fatalf("NVM launch policy missing: %q", args[1])
	}
}

func TestPrepareStdioDoesNotExpandShellExpressions(t *testing.T) {
	command, args := PrepareStdio("agent", []string{"$HOME", "`printf injected`", "it's safe"}, true)
	if command != "bash" || len(args) != 2 {
		t.Fatalf("unexpected launch: %q %v", command, args)
	}
	for _, expected := range []string{"'$HOME'", "'`printf injected`'", `'it'"'"'s safe'`} {
		if !strings.Contains(args[1], expected) {
			t.Fatalf("shell argument not quoted literally: %q", args[1])
		}
	}
}

func TestPrepareStdioRunsShellMetacharactersAsLiteralArguments(t *testing.T) {
	printf, err := exec.LookPath("printf")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	marker := filepath.Join(home, "injected")
	inputs := []string{"$HOME", "`touch " + marker + "`", "$(touch " + marker + ")", "it's safe"}
	command, args := PrepareStdio(printf, append([]string{"%s\n"}, inputs...), true)
	proc := exec.Command(command, args...)
	proc.Env = append(os.Environ(), "HOME="+home)
	output, err := proc.CombinedOutput()
	if err != nil {
		t.Fatalf("literal argument launch: %v: %s", err, output)
	}
	if got, want := string(output), strings.Join(inputs, "\n")+"\n"; got != want {
		t.Fatalf("shell changed agent arguments: got %q, want %q", got, want)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("shell command substitution ran: marker stat error %v", err)
	}
}
