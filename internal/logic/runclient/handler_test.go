package runclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/runapi"
)

// Only provider work is simulated: decoding, authentication, workspace identity,
// async dispatch, idempotency and terminal persistence use the real run handler.
type blockedSubmissionRouter struct {
	middleware.SessionRouter
	received      chan middleware.ConversationRequest
	release       chan struct{}
	calls         atomic.Int32
	releaseOnce   sync.Once
	workspaceRoot string
}

func (r *blockedSubmissionRouter) finish() {
	r.releaseOnce.Do(func() { close(r.release) })
}

func (r *blockedSubmissionRouter) RouteConversation(ctx context.Context, req middleware.ConversationRequest) (string, error) {
	r.calls.Add(1)
	r.received <- req
	select {
	case <-r.release:
		return "provider finished", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (*blockedSubmissionRouter) HandleSessionActionTyped(_ context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	return middleware.SessionActionResult{Action: req.Action}, nil
}

func submissionFixture(t *testing.T) (Input, *blockedSubmissionRouter, *runtrace.Store) {
	t.Helper()
	storage := memstore.New()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{ID: "project", RootPath: root}); err != nil {
		t.Fatal(err)
	}
	router := &blockedSubmissionRouter{received: make(chan middleware.ConversationRequest, 4), release: make(chan struct{}), workspaceRoot: root}
	t.Cleanup(router.finish)
	runtime := runapi.NewServer(router).WithTraceStorage(storage).WithAPIKey("matrix-key")
	server := httptest.NewServer(http.HandlerFunc(runtime.HandleRuns))
	t.Cleanup(server.Close)
	return Input{
		Address: runtimeAddress(t, server), APIKey: "matrix-key", AgentID: "specialist",
		WorkspaceID: "project", Prompt: "review the project", IdempotencyKey: "review-1", Timeout: 2 * time.Second,
	}, router, runtrace.NewStore(storage)
}

func receiveSubmission(t *testing.T, router *blockedSubmissionRouter) middleware.ConversationRequest {
	t.Helper()
	select {
	case req := <-router.received:
		return req
	case <-time.After(2 * time.Second):
		t.Fatal("accepted submission was not dispatched")
		return middleware.ConversationRequest{}
	}
}

func awaitSubmissionCompletion(t *testing.T, store *runtrace.Store, runID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, found, err := store.LoadRun(runID)
		if err != nil {
			t.Fatal(err)
		}
		if found && run.Status == runtrace.StatusCompleted {
			if run.Output != "provider finished" || run.ExecutionMode != runtrace.ExecutionModeAsync {
				t.Fatalf("completed run = %+v", run)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("released provider did not produce a completed run")
}

func TestSubmitRealHandlerAsyncReplayAndChannelScope(t *testing.T) {
	input, router, store := submissionFixture(t)
	first, err := Submit(context.Background(), input)
	if err != nil || first.Status != runtrace.StatusRunning || first.RunID == "" {
		t.Fatalf("async acceptance before provider release = %+v, %v", first, err)
	}
	req := receiveSubmission(t, router)
	if req.ChannelID != DefaultChannelID || req.WorkspaceID != input.WorkspaceID || req.WorkspacePath != router.workspaceRoot || req.AgentID != input.AgentID {
		t.Fatalf("dispatch lost identity: %+v", req)
	}
	replay, err := Submit(context.Background(), input)
	if err != nil || !replay.Replayed || replay.RunID != first.RunID || router.calls.Load() != 1 {
		t.Fatalf("replay = %+v, %v; calls=%d", replay, err, router.calls.Load())
	}
	conflicting := input
	conflicting.Prompt = "different work"
	if _, err := Submit(context.Background(), conflicting); err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatalf("changed keyed payload was not refused: %v", err)
	}
	input.ChannelID = "other.supervisor"
	other, err := Submit(context.Background(), input)
	if err != nil || other.Replayed || other.RunID == first.RunID {
		t.Fatalf("different channel did not have its own key scope: %+v, %v", other, err)
	}
	if req := receiveSubmission(t, router); req.ChannelID != input.ChannelID {
		t.Fatalf("explicit channel lost: %+v", req)
	}
	router.finish()
	awaitSubmissionCompletion(t, store, first.RunID)
	awaitSubmissionCompletion(t, store, other.RunID)
}

func TestSubmitRealHandlerRefusesAuthAndUnknownWorkspace(t *testing.T) {
	input, router, _ := submissionFixture(t)
	wrongKey := input
	wrongKey.APIKey = "wrong-key"
	if _, err := Submit(context.Background(), wrongKey); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("incorrect surface key was not refused: %v", err)
	}
	input.WorkspaceID = "unknown"
	if _, err := Submit(context.Background(), input); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("unknown workspace was not refused: %v", err)
	}
	if router.calls.Load() != 0 {
		t.Fatal("refused submission reached provider work")
	}
}
