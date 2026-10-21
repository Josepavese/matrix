package a2a

import (
	"context"
	"fmt"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
)

// taskStateGuard enforces Matrix's one-running-turn policy. Terminal task
// refusals belong to the current protocol SDK; Matrix does not duplicate them.
type taskStateGuard struct {
	a2asrv.PassthroughCallInterceptor

	tasks taskstore.Store
}

// Before implements a2asrv.CallInterceptor.
func (g *taskStateGuard) Before(ctx context.Context, _ *a2asrv.CallContext, req *a2asrv.Request) (context.Context, any, error) {
	if req == nil {
		return ctx, nil, nil
	}
	taskID, applies := guardedTarget(req.Payload)
	if !applies {
		return ctx, nil, nil
	}
	stored, err := g.tasks.Get(ctx, taskID)
	if err == nil && stored != nil && stored.Task != nil {
		state := stored.Task.Status.State
		if state == a2asdk.TaskStateSubmitted || state == a2asdk.TaskStateWorking {
			return ctx, nil, fmt.Errorf("task %s is still %q, and Matrix serves one turn at a time: %w", taskID, state, a2asdk.ErrUnsupportedOperation)
		}
	}
	// A task the store does not hold (or cannot read for this caller) is not this
	// guard's business: the SDK's own TaskNotFoundError stands.
	return ctx, nil, nil
}

// guardedTarget selects only message insertion. Subscriptions never compete
// with the running turn and remain governed entirely by the protocol SDK.
func guardedTarget(payload any) (a2asdk.TaskID, bool) {
	typed, ok := payload.(*a2asdk.SendMessageRequest)
	if !ok || typed == nil || typed.Message == nil || typed.Message.TaskID == "" {
		return "", false
	}
	return typed.Message.TaskID, true
}
