package agents

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

type captureSidecarA2AExecutor struct {
	mu       sync.Mutex
	parts    a2asdk.ContentParts
	metadata map[string]any
}

func (e *captureSidecarA2AExecutor) Execute(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2asdk.Event, error] {
	e.mu.Lock()
	e.parts = execCtx.Message.Parts
	e.metadata = execCtx.Message.Metadata
	e.mu.Unlock()
	return func(yield func(a2asdk.Event, error) bool) {
		yield(a2asdk.NewMessageForTask(a2asdk.MessageRoleAgent, execCtx, a2asdk.NewTextPart("ok")), nil)
	}
}

func (e *captureSidecarA2AExecutor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2asdk.Event, error] {
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

func TestA2AConversationClient_ProjectsSidecarCapsules(t *testing.T) {
	executor := &captureSidecarA2AExecutor{}
	handler := a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(executor))
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

	_, err = client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		AgentID:          "echo-a2a",
		LogicalSessionID: "logical-a2a",
		Message:          "task body",
		SidecarCapsules: []middleware.SidecarCapsule{
			{
				Provider:   "noema",
				ID:         "caps-a2a",
				Schema:     "sidecar.intent.v0",
				Version:    "0.1",
				Visibility: middleware.SidecarVisibilityLLMVisible,
				Format:     middleware.SidecarFormatNoemaXML,
				Content:    "<noema id=\"caps-a2a\">intent</noema>",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}

	executor.mu.Lock()
	defer executor.mu.Unlock()
	if len(executor.parts) != 3 {
		t.Fatalf("expected task text, data part, and visible fallback text; got %+v", executor.parts)
	}
	if executor.parts[0].Text() != "task body" || executor.parts[2].Text() != "<noema id=\"caps-a2a\">intent</noema>" {
		t.Fatalf("unexpected text projection: %+v", executor.parts)
	}
	data, ok := executor.parts[1].Content.(a2asdk.Data)
	if !ok {
		t.Fatalf("expected A2A data part, got %#v", executor.parts[1].Content)
	}
	dataMap, ok := data.Value.(map[string]any)
	if !ok || dataMap["sidecar"] == nil {
		t.Fatalf("expected sidecar data payload, got %#v", data.Value)
	}
	if executor.parts[1].MediaType != "application/vnd.noema.sidecar+json" {
		t.Fatalf("unexpected sidecar media type: %q", executor.parts[1].MediaType)
	}
	if executor.metadata["matrix.sidecar"] == nil {
		t.Fatalf("expected Matrix sidecar message metadata, got %#v", executor.metadata)
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
