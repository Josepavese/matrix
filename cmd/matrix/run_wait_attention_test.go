//go:build linux || darwin

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

func TestRunWaitAttentionIsNonterminalAndOptIn(t *testing.T) {
	server, path := newNotificationTestServer(t)
	run, _, err := server.Store().Start(runtrace.Run{AgentID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Store().NotifyUnobservedActivity(run.ID); err != nil {
		t.Fatal(err)
	}
	previous := runWaitOnAttention
	runWaitOnAttention = true
	t.Cleanup(func() { runWaitOnAttention = previous })
	withWaitFlags(t, 0, time.Second)
	cmd, output := testCommand()
	if err := runWaitAt(cmd, path, run.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "outcome=attention_required") || !strings.Contains(output.String(), "activity_unobserved") {
		t.Fatalf("wakeup missing: %s", output)
	}
	active, _, err := server.Store().LoadRun(run.ID)
	if err != nil || active.Status != runtrace.StatusRunning {
		t.Fatalf("notice was terminal: %+v %v", active, err)
	}
	runWaitOnAttention = false
	if _, terminal := waitOutcome("run.attention_required"); terminal {
		t.Fatal("default wait stopped before the outcome")
	}
}
