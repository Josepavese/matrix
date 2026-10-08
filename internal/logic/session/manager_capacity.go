package session

import (
	"context"
	"github.com/Josepavese/matrix/internal/logic/admission"
	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
	"log/slog"
	"time"
)

// WithCapacity wires the runtime host's PAL observer before serving requests.
func (m *Manager) WithCapacity(observer middleware.Capacity) *Manager {
	m.capacity = observer
	return m
}

func (m *Manager) workspaceCapacity(path string) *middleware.CapacitySnapshot {
	if m.capacity == nil {
		return nil
	}
	snapshot := workspace.ObserveCapacity(path, m.capacity)
	return &snapshot
}

func (m *Manager) WithAdmission(manager *admission.Manager) *Manager {
	m.admission = manager
	return m
}

func (m *Manager) AdmissionState() admission.State {
	if m.admission == nil {
		return admission.State{Scope: "not_configured"}
	}
	return m.admission.State()
}

func (m *Manager) acquireCapacity(ctx context.Context, req middleware.CapacityRequest, path string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.admission != nil {
		return m.admission.Acquire(path, req)
	}
	if req != (middleware.CapacityRequest{}) {
		return nil, &admission.Refusal{Code: "capacity_policy_unavailable"}
	}
	return func() {}, nil
}

func (m *Manager) routeWithCapacity(ctx context.Context, capacity middleware.CapacityRequest, req middleware.RouteRequest) (string, string, []middleware.ToolCall, middleware.ConversationMetadata, error) {
	release, err := m.acquireCapacity(ctx, capacity, req.WorkspacePath)
	if err != nil {
		return "", "", nil, middleware.ConversationMetadata{}, err
	}
	defer release()
	started := time.Now()
	output, remote, tools, metadata, routeErr := m.router.Route(ctx, req)
	status := "returned"
	if routeErr != nil {
		status = "failed"
	}
	slog.Info("provider route timing observed", "event", "route_timing", "agent", req.AgentID, "logical_session", req.LogicalSessionID, "duration_ms", time.Since(started).Milliseconds(), "status", status)
	return output, remote, tools, metadata, routeErr
}
