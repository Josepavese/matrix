package runtrace

import (
	"errors"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

func TestUnknownActivityNoticeNeverCancelsOrHidesTerminalWakeup(t *testing.T) {
	s := NewStore(memstore.New())
	run, _, err := s.Start(Run{AgentID: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.NotifyUnobservedActivity(run.ID); err != nil {
		t.Fatal(err)
	}
	active, _, _ := s.LoadRun(run.ID)
	if active.Status != StatusRunning {
		t.Fatalf("silence cancelled the task: %+v", active)
	}
	if _, err := s.Fail(run.ID, errors.New("provider refused")); err != nil {
		t.Fatal(err)
	}
	if err := s.NotifyUnobservedActivity(run.ID); err != nil {
		t.Fatal(err)
	}
	items, _, err := s.LoadNotificationsAfter(0, 100, nil)
	if err != nil || len(items) != 2 || items[0].FailureCode != "activity_unobserved" || items[1].Kind != "run.failed" {
		t.Fatalf("missing or late wakeup: %+v %v", items, err)
	}
}

func TestAttentionDoesNotSuppressCrashReconciliation(t *testing.T) {
	s := NewStore(memstore.New())
	run, _, err := s.Start(Run{AgentID: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.NotifyUnobservedActivity(run.ID); err != nil {
		t.Fatal(err)
	}
	run.Status = StatusFailed
	if err := s.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	count, err := s.ReconcileTerminalNotifications()
	if err != nil || count != 1 {
		t.Fatalf("attention hid a lost terminal wakeup: %d %v", count, err)
	}
	count, err = s.ReconcileTerminalNotifications()
	if err != nil || count != 0 {
		t.Fatalf("terminal delivery duplicated: %d %v", count, err)
	}
}
