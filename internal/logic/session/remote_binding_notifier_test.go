package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

type cancelBeforeResultRouter struct {
	mockRouter
	bound chan struct{}
}

func (r *cancelBeforeResultRouter) Route(ctx context.Context, req middleware.RouteRequest) (string, string, []middleware.ToolCall, middleware.ConversationMetadata, error) {
	req.ThoughtNotifier.SetHeader(req.AgentID, "remote-before-result")
	close(r.bound)
	<-ctx.Done()
	return "", "", nil, middleware.ConversationMetadata{}, ctx.Err()
}

func TestRemoteBindingSurvivesCancellationBeforeOrderedResult(t *testing.T) {
	s := &mockStorage{}
	if err := s.Set("system.configured", []byte("true")); err != nil {
		t.Fatal(err)
	}
	r := &cancelBeforeResultRouter{bound: make(chan struct{})}
	m := NewManager(s, r, newTestWizard(s), nil)
	id, err := m.GetOrCreateSession("channel", "agent")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := m.Route(ctx, "channel", "agent", "test", nil); done <- err }()
	select {
	case <-r.bound:
	case <-time.After(time.Second):
		t.Fatal("provider never bound its session")
	}
	meta, found, err := m.loadSessionMeta(id)
	if err != nil || !found || meta.AgentSessionID != "remote-before-result" {
		t.Fatalf("live remote identity was lost: %+v %v", meta, err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected result: %v", err)
	}
	meta, _, err = m.loadSessionMeta(id)
	if err != nil || meta.AgentSessionID != "remote-before-result" {
		t.Fatalf("cancel erased the remote identity: %+v %v", meta, err)
	}
}

func TestRemoteBindingRejectsUnknownOrMismatchedAgent(t *testing.T) {
	s := &mockStorage{}
	m := NewManager(s, &mockRouter{}, newTestWizard(s), nil)
	id, err := m.GetOrCreateSession("channel", "agent")
	if err != nil {
		t.Fatal(err)
	}
	m.remoteBinding(id, nil).SetHeader("other", "wrong-remote")
	m.remoteBinding("missing", nil).SetHeader("agent", "wrong-remote")
	meta, _, err := m.loadSessionMeta(id)
	if err != nil || meta.AgentSessionID != "" {
		t.Fatalf("unrelated provider rebound session: %+v %v", meta, err)
	}
}
