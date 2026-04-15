package agents

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/jose/matrix-v2/internal/middleware"
)

type echoA2AExecutor struct{}

func (echoA2AExecutor) Execute(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2asdk.Event, error] {
	return func(yield func(a2asdk.Event, error) bool) {
		yield(a2asdk.NewMessageForTask(a2asdk.MessageRoleAgent, execCtx, a2asdk.NewTextPart("echo:"+a2aPartsText(execCtx.Message.Parts))), nil)
	}
}

func (echoA2AExecutor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2asdk.Event, error] {
	return func(yield func(a2asdk.Event, error) bool) {
		yield(a2asdk.NewStatusUpdateEvent(execCtx, a2asdk.TaskStateCanceled, nil), nil)
	}
}

func TestA2AConversationClient_ExecuteTurn(t *testing.T) {
	handler := a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(echoA2AExecutor{}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()

	factory := &a2aConversationFactory{}
	client, err := factory.NewClient(context.Background(), middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindA2A,
		Transport: "JSONRPC",
		Address:   server.URL,
	}, middleware.ConversationFactoryDeps{})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	first, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		AgentID:          "echo-a2a",
		LogicalSessionID: "logical-a2a",
		Message:          "hello",
	})
	if err != nil {
		t.Fatalf("ExecuteTurn first failed: %v", err)
	}
	if strings.TrimSpace(first.Output) != "echo:hello" {
		t.Fatalf("unexpected first output: %q", first.Output)
	}
	if first.RemoteSessionID == "" {
		t.Fatal("expected non-empty remote session id")
	}

	second, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		AgentID:          "echo-a2a",
		LogicalSessionID: "logical-a2a",
		RemoteSessionID:  first.RemoteSessionID,
		Message:          "again",
	})
	if err != nil {
		t.Fatalf("ExecuteTurn second failed: %v", err)
	}
	if strings.TrimSpace(second.Output) != "echo:again" {
		t.Fatalf("unexpected second output: %q", second.Output)
	}
	if second.RemoteSessionID == "" {
		t.Fatal("expected non-empty remote session id on follow-up turn")
	}
}

func TestA2ATaskGoneError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{err: nil, want: false},
		{err: errors.New("task not found"), want: true},
		{err: errors.New("failed to load a task: task not found"), want: true},
		{err: errors.New("some other failure"), want: false},
	}

	for _, tc := range cases {
		if got := a2aTaskGoneError(tc.err); got != tc.want {
			t.Fatalf("a2aTaskGoneError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
