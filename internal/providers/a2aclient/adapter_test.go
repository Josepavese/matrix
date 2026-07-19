package a2aclient

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
	matrixa2a "github.com/Josepavese/matrix/internal/providers/a2a"
	"github.com/Josepavese/matrix/internal/providers/a2astate"
	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"
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

type recordingThoughtNotifier struct {
	headerAgent string
	headerID    string
	updates     []middleware.ThoughtUpdate
}

func (n *recordingThoughtNotifier) OnThought(update middleware.ThoughtUpdate) {
	n.updates = append(n.updates, update)
}

func (n *recordingThoughtNotifier) SetHeader(agentID, agentSessionID string) {
	n.headerAgent = agentID
	n.headerID = agentSessionID
}

func (n *recordingThoughtNotifier) FormattedHeader() string {
	return ""
}

func TestA2AConversationClient_ExecuteTurn(t *testing.T) {
	handler := a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(echoA2AExecutor{}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()

	factory := Factory{}
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

func TestA2AConversationClient_ListRemoteSessionsTraversesAllPages(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	server.Start()
	defer server.Close()
	matrixa2a.NewServer(cardTestRouter{}, server.URL, "opencode").RegisterRoutes(mux)

	base, err := (Factory{}).NewClient(context.Background(), middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindA2A, Transport: "JSONRPC", Address: server.URL + "/a2a",
	}, middleware.ConversationFactoryDeps{})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = base.Close() }()
	for i := 0; i < 101; i++ {
		if _, err := base.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "task"}); err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
	}
	sessionControl, ok := base.(middleware.ConversationSessionControl)
	if !ok {
		t.Fatal("A2A client does not expose session control")
	}
	sessions, err := sessionControl.ListRemoteSessions(context.Background())
	if err != nil {
		t.Fatalf("ListRemoteSessions failed: %v", err)
	}
	if len(sessions) != 101 {
		t.Fatalf("expected 101 sessions across two pages, got %d", len(sessions))
	}
}

func TestA2AConversationClient_AttachesGovernedHeaders(t *testing.T) {
	handler := a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(echoA2AExecutor{}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Matrix-Key") != "secret" || r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()

	client, err := (Factory{}).NewClient(context.Background(), middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindA2A,
		Transport: "JSONRPC",
		Address:   server.URL,
		Headers: map[string]string{
			"X-Matrix-Key":  "secret",
			"Authorization": "Bearer token",
		},
	}, middleware.ConversationFactoryDeps{})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "authenticated"}); err != nil {
		t.Fatalf("authenticated A2A turn failed: %v", err)
	}
}

func TestA2AConversationClient_ResolvesCardAndNegotiatesInterface(t *testing.T) {
	router := &cardTestRouter{}
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	server.Start()
	defer server.Close()
	matrixa2a.NewServer(router, server.URL, "opencode").WithAPIKey("secret").RegisterRoutes(mux)

	client, err := (Factory{}).NewClient(context.Background(), middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindA2A,
		Transport: "HTTP+JSON",
		CardURL:   server.URL + "/.well-known/agent-card.json",
		Headers:   map[string]string{"X-Matrix-Key": "secret"},
	}, middleware.ConversationFactoryDeps{})
	if err != nil {
		t.Fatalf("NewClient from card failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	result, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "from card"})
	if err != nil {
		t.Fatalf("card-negotiated turn failed: %v", err)
	}
	if result.Output != "card:from card" {
		t.Fatalf("unexpected card-negotiated output: %#v", result)
	}
}

type cardTestRouter struct{}

func (cardTestRouter) Route(_ context.Context, _, _, input string, _ middleware.ThoughtNotifier) (string, error) {
	return "card:" + input, nil
}

type noopPushSender struct{}

func (noopPushSender) SendPush(context.Context, *a2asdk.PushConfig, a2asdk.Event) error { return nil }

type tenantCaptureInterceptor struct {
	a2asrv.PassthroughCallInterceptor
	tenant string
}

func (i *tenantCaptureInterceptor) Before(ctx context.Context, callCtx *a2asrv.CallContext, _ *a2asrv.Request) (context.Context, any, error) {
	i.tenant = callCtx.Tenant()
	return ctx, nil, nil
}

func TestA2AConversationClient_PropagatesDirectEndpointTenant(t *testing.T) {
	capture := &tenantCaptureInterceptor{}
	handler := a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(echoA2AExecutor{}, a2asrv.WithCallInterceptors(capture)))
	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := (Factory{}).NewClient(context.Background(), middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindA2A, Transport: "JSONRPC", Address: server.URL, Tenant: "project-7",
	}, middleware.ConversationFactoryDeps{})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()
	if _, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "tenant"}); err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if capture.tenant != "project-7" {
		t.Fatalf("tenant = %q, want project-7", capture.tenant)
	}
}

func TestA2AConversationClient_CoversPushCRUDAndExtendedCard(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	server.Start()
	defer server.Close()
	extended := &a2asdk.AgentCard{
		Name:               "Matrix Extended",
		Description:        "authenticated profile",
		Version:            "2",
		Capabilities:       a2asdk.AgentCapabilities{},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Skills:             []a2asdk.AgentSkill{},
		SupportedInterfaces: []*a2asdk.AgentInterface{
			a2asdk.NewAgentInterface(server.URL+"/a2a", a2asdk.TransportProtocolJSONRPC),
		},
	}
	matrixa2a.NewServer(cardTestRouter{}, server.URL, "opencode").
		WithPushNotifications(push.NewInMemoryStore(), noopPushSender{}).
		WithExtendedAgentCard(extended).
		RegisterRoutes(mux)

	base, err := (Factory{}).NewClient(context.Background(), middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindA2A, Transport: "JSONRPC", CardURL: server.URL + "/.well-known/agent-card.json",
	}, middleware.ConversationFactoryDeps{})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = base.Close() }()
	turn, err := base.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "configure push"})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}

	pushControl, ok := base.(middleware.ConversationTaskPushControl)
	if !ok {
		t.Fatal("A2A client does not expose push control")
	}
	created, err := pushControl.CreateTaskPushConfig(context.Background(), middleware.TaskPushConfig{
		RemoteSessionID: turn.RemoteSessionID,
		URL:             "https://callback.example/a2a",
		Token:           "verify-me",
		Auth:            &middleware.TaskPushAuth{Scheme: "Bearer", Credentials: "callback-secret"},
	})
	if err != nil || created.ID == "" {
		t.Fatalf("CreateTaskPushConfig failed: created=%#v err=%v", created, err)
	}
	got, err := pushControl.GetTaskPushConfig(context.Background(), turn.RemoteSessionID, created.ID)
	if err != nil || got.Token != "verify-me" || got.Auth == nil || got.Auth.Scheme != "Bearer" {
		t.Fatalf("GetTaskPushConfig lost fields: got=%#v err=%v", got, err)
	}
	listed, err := pushControl.ListTaskPushConfigs(context.Background(), turn.RemoteSessionID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListTaskPushConfigs failed: listed=%#v err=%v", listed, err)
	}
	if err := pushControl.DeleteTaskPushConfig(context.Background(), turn.RemoteSessionID, created.ID); err != nil {
		t.Fatalf("DeleteTaskPushConfig failed: %v", err)
	}
	profileReader, ok := base.(middleware.ConversationExtendedProfileReader)
	if !ok {
		t.Fatal("A2A client does not expose extended profile control")
	}
	doc, err := profileReader.GetExtendedProfile(context.Background())
	if err != nil || !strings.Contains(string(doc.Data), "Matrix Extended") {
		t.Fatalf("GetExtendedProfile failed: doc=%s err=%v", doc.Data, err)
	}
}

func TestA2AConversationClient_UsesNonStreamingWithoutAdvertisedCapability(t *testing.T) {
	caps := &a2asdk.AgentCapabilities{Streaming: false}
	handler := a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(echoA2AExecutor{}, a2asrv.WithCapabilityChecks(caps)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()

	factory := Factory{}
	client, err := factory.NewClient(context.Background(), middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindA2A,
		Transport: "JSONRPC",
		Address:   server.URL,
	}, middleware.ConversationFactoryDeps{})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	notifier := &recordingThoughtNotifier{}
	result, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		AgentID:          "echo-a2a",
		LogicalSessionID: "logical-a2a",
		Message:          "hello",
		ThoughtNotifier:  notifier,
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if strings.TrimSpace(result.Output) != "echo:hello" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if result.RemoteSessionID == "" || notifier.headerID == "" {
		t.Fatalf("expected non-streaming turn to preserve remote session id, result=%#v notifier=%#v", result, notifier)
	}
	if len(notifier.updates) != 0 {
		t.Fatalf("non-streaming turn must not synthesize thought deltas, got %#v", notifier.updates)
	}
}

func TestA2AConversationClient_ProjectsSidecarCapsules(t *testing.T) {
	executor := &captureSidecarA2AExecutor{}
	handler := a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(executor))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()

	factory := Factory{}
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

func TestA2AConversationClientCapabilitiesDoNotAdvertiseDelete(t *testing.T) {
	client := &a2aConversationClient{}
	caps := client.SessionCapabilities()
	if caps.Delete {
		t.Fatal("A2A must not advertise delete when the protocol adapter only supports cancel")
	}
	if caps.Details["delete"].Supported {
		t.Fatalf("delete descriptor should be unsupported: %+v", caps.Details["delete"])
	}
	if err := client.DeleteRemoteSession(context.Background(), "task-1"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported delete error, got %v", err)
	}
}

func TestA2AProtocolCapabilitiesExposeFullStableRPCSurface(t *testing.T) {
	report := (&a2aConversationClient{advertised: &a2asdk.AgentCapabilities{Streaming: true, PushNotifications: true, ExtendedAgentCard: true}}).ProtocolCapabilities()
	for _, name := range []string{
		"SendMessage", "SendStreamingMessage", "GetTask", "ListTasks", "CancelTask",
		"SubscribeToTask", "CreateTaskPushConfig", "GetTaskPushConfig",
		"ListTaskPushConfigs", "DeleteTaskPushConfig", "GetExtendedAgentCard",
	} {
		if !report.Operations[name].Supported {
			t.Fatalf("stable A2A operation %s is not covered: %#v", name, report.Operations[name])
		}
	}
	if !report.Transports["JSONRPC"].Supported || !report.Transports["HTTP+JSON"].Supported || report.Transports["GRPC"].Supported {
		t.Fatalf("unexpected A2A binding report: %#v", report.Transports)
	}
	unknown := (&a2aConversationClient{}).ProtocolCapabilities()
	if unknown.Operations["SendStreamingMessage"].Supported || unknown.Operations["SendStreamingMessage"].Status != "unknown" {
		t.Fatalf("unadvertised optional operations must fail closed: %#v", unknown.Operations["SendStreamingMessage"])
	}
}

func TestA2ASendMessagePreservesExtensionsAndTaskReferences(t *testing.T) {
	referenced := a2astate.Encode(a2astate.State{TaskID: "task-parent", ContextID: "context-parent"})
	req := a2aSendMessageRequest(middleware.ConversationTurn{
		Message:                  "continue",
		ExtensionURIs:            []string{"https://example.com/extensions/audit/v1"},
		ReferencedRemoteSessions: []string{referenced},
	})
	if len(req.Message.Extensions) != 1 || req.Message.Extensions[0] != "https://example.com/extensions/audit/v1" {
		t.Fatalf("extensions were not preserved: %#v", req.Message.Extensions)
	}
	if len(req.Message.ReferenceTasks) != 1 || req.Message.ReferenceTasks[0] != "task-parent" {
		t.Fatalf("task references were not preserved: %#v", req.Message.ReferenceTasks)
	}
}

func TestA2AResultPreservesRichArtifactParts(t *testing.T) {
	image := a2asdk.NewRawPart([]byte("png"))
	image.Filename = "image.png"
	image.MediaType = "image/png"
	data := a2asdk.NewDataPart(map[string]any{"answer": float64(42)})
	data.MediaType = "application/json"
	event := &a2asdk.TaskArtifactUpdateEvent{
		TaskID:    "task-1",
		ContextID: "context-1",
		Artifact: &a2asdk.Artifact{
			ID:    "artifact-1",
			Name:  "result",
			Parts: a2asdk.ContentParts{image, data},
		},
	}

	projection := projectA2AEvent(event)
	if len(projection.Blocks) != 2 {
		t.Fatalf("expected two rich content blocks, got %#v", projection.Blocks)
	}
	if projection.Blocks[0].Type != "image" || projection.Blocks[0].Name != "image.png" || projection.Blocks[0].Data == "" {
		t.Fatalf("image artifact was not preserved: %#v", projection.Blocks[0])
	}
	if projection.Blocks[1].Type != "data" || !strings.Contains(projection.Blocks[1].Data, "answer") {
		t.Fatalf("data artifact was not preserved: %#v", projection.Blocks[1])
	}
	if projection.Metadata["artifact_id"] != "artifact-1" {
		t.Fatalf("artifact metadata was not preserved: %#v", projection.Metadata)
	}
}

func TestA2AProjectionDoesNotMixWorkingStatusIntoFinalOutput(t *testing.T) {
	working := projectA2ATask(&a2asdk.Task{ID: "task-1", Status: a2asdk.TaskStatus{
		State:   a2asdk.TaskStateWorking,
		Message: a2asdk.NewMessage(a2asdk.MessageRoleAgent, a2asdk.NewTextPart("still thinking")),
	}})
	if working.Output {
		t.Fatalf("working task status must remain progress-only: %#v", working)
	}
	completed := projectA2ATask(&a2asdk.Task{ID: "task-1", Status: a2asdk.TaskStatus{
		State:   a2asdk.TaskStateCompleted,
		Message: a2asdk.NewMessage(a2asdk.MessageRoleAgent, a2asdk.NewTextPart("final answer")),
	}})
	if !completed.Output || completed.Text != "final answer" {
		t.Fatalf("completed task output was not preserved: %#v", completed)
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
