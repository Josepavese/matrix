package a2a

import (
	"context"
	"fmt"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
)

// taskStateGuard answers the A2A error the specification fixes - or, for a task that
// is still running, the error type that describes the refusal honestly - where the
// protocol SDK would otherwise answer something outside the A2A error set.
//
//   - A message addressed to a task that has reached a terminal state is
//     UnsupportedOperationError. The specification fixes that error for both the unary
//     and the streaming form (§3.1.1 and §3.1.2: "Messages sent to Tasks that are in a
//     terminal state ... cannot accept further messages") and for resubscribing
//     (§3.1.6: "Returns UnsupportedOperationError if the task is in a terminal
//     state"). The SDK instead chooses different codes before any Matrix code runs:
//     the executor factory refuses with ErrInvalidParams (a2asrv/agentexec.go:228 in
//     a2a-go v2.5.0), which JSON-RPC reports as -32602, and SubscribeToTask finds no
//     execution left to attach to (internal/taskexec/local_manager.go:134), which the
//     handler wraps in ErrTaskNotFound (a2asrv/handler.go:373), reported as -32001.
//
//   - A message addressed to a task whose turn is still running is refused by the
//     execution manager with its internal ErrExecutionInProgress
//     (internal/taskexec/local_manager.go:197). That value is not one of the A2A
//     error types and lives in an internal package Matrix cannot even name, so both
//     bindings fall back to an internal error - -32603 on JSON-RPC, 500 on HTTP+JSON -
//     for a condition a client can trigger deliberately. Matrix does not accept a
//     second message for a task whose turn is in flight, so the refusal is an
//     UnsupportedOperationError: §3.3.2 defines that error as "a specific aspect of it
//     is not supported by this server agent implementation". Resubscription is
//     deliberately exempt, because streaming a running task is the operation's whole
//     purpose, and a task awaiting input is exempt because a follow-up message is the
//     documented way to continue it (§3.4.3).
//
// Both bindings share one intercepted handler, and a2asrv.CallInterceptor is the hook
// the SDK offers for exactly this: an interceptor's Before may refuse a request before
// the handler reaches those paths. The guard acts only when the store holds the task,
// so an unknown or inaccessible id keeps the SDK's own TaskNotFoundError.
type taskStateGuard struct {
	a2asrv.PassthroughCallInterceptor

	tasks taskstore.Store
}

// Before implements a2asrv.CallInterceptor.
func (g *taskStateGuard) Before(ctx context.Context, _ *a2asrv.CallContext, req *a2asrv.Request) (context.Context, any, error) {
	if req == nil {
		return ctx, nil, nil
	}
	taskID, operation, applies := guardedTarget(req.Payload)
	if !applies {
		return ctx, nil, nil
	}
	stored, err := g.tasks.Get(ctx, taskID)
	if err == nil && stored != nil && stored.Task != nil {
		switch state := stored.Task.Status.State; {
		case state.Terminal():
			return ctx, nil, fmt.Errorf("task %s is in the terminal state %q: %w", taskID, state, a2asdk.ErrUnsupportedOperation)
		case operation == operationMessage && (state == a2asdk.TaskStateSubmitted || state == a2asdk.TaskStateWorking):
			return ctx, nil, fmt.Errorf("task %s is still %q, and Matrix serves one turn at a time: %w", taskID, state, a2asdk.ErrUnsupportedOperation)
		}
	}
	// A task the store does not hold (or cannot read for this caller) is not this
	// guard's business: the SDK's own TaskNotFoundError stands.
	return ctx, nil, nil
}

// guardedOperation is the operation a request asks for, when this server constrains it
// on a task that has not finished.
type guardedOperation int

const (
	// operationMessage is SendMessage and its streaming form: one turn per task.
	operationMessage guardedOperation = iota
	// operationSubscribe is SubscribeToTask, which is never refused for a running task
	// because observing one is what it is for.
	operationSubscribe
)

// guardedTarget reports the task a request addresses and which operation asks, for the
// operations this server constrains. The SDK calls the interceptor with the decoded
// request, so the check is on the typed payload rather than on anything a client could
// spell.
func guardedTarget(payload any) (a2asdk.TaskID, guardedOperation, bool) {
	switch typed := payload.(type) {
	case *a2asdk.SendMessageRequest:
		if typed == nil || typed.Message == nil || typed.Message.TaskID == "" {
			return "", 0, false
		}
		return typed.Message.TaskID, operationMessage, true
	case *a2asdk.SubscribeToTaskRequest:
		if typed == nil || typed.ID == "" {
			return "", 0, false
		}
		return typed.ID, operationSubscribe, true
	default:
		return "", 0, false
	}
}
