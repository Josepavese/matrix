//go:build linux || darwin

package main

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestWaitJSONNeverUsesDiagnosticStderr(t *testing.T) {
	cmd := &cobra.Command{}
	var diagnostics bytes.Buffer
	cmd.SetErr(&diagnostics)
	previous := runWaitJSON
	runWaitJSON = true
	t.Cleanup(func() { runWaitJSON = previous })
	// Deliberately leave Out unset, as in the actual executable. SetOut in the
	// older tests hid Cobra Println's fallback to stderr instead of stdout.
	printWakeupOutcome(cmd, wakeupReport{RunID: "stdio-proof", Outcome: "completed"})
	if diagnostics.Len() != 0 {
		t.Fatalf("machine result went to stderr: %s", &diagnostics)
	}
}
