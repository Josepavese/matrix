package runnotifier

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

func TestNotifierPersistsProviderModelSelection(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{AgentID: "dsh", RequestedModel: "missing", ModelVerification: "unverified", ChannelID: "http.test"})
	if err != nil {
		t.Fatal(err)
	}
	notifier := New(store, run.ID, "dsh", "acp")
	notifier.OnModelSelection(middleware.ModelSelection{ConfiguredModel: "available", Verification: "unverified",
		FallbackUsed: true, FallbackReason: "requested_model_unavailable"})
	trace, found, err := store.Trace(run.ID)
	if err != nil || !found {
		t.Fatalf("trace: found=%v err=%v", found, err)
	}
	if trace.Run.RequestedModel != "missing" || trace.Run.ConfiguredModel != "available" ||
		trace.Run.EffectiveModel != "" || !trace.Run.ModelFallbackUsed || trace.Run.ModelFallbackReason != "requested_model_unavailable" ||
		trace.Run.ModelVerification != "unverified" {
		t.Fatalf("model provenance inaccurate: %+v", trace.Run)
	}
	selected := false
	for _, event := range trace.Events {
		selected = selected || event.Kind == "model.selection"
	}
	if !selected {
		t.Fatalf("model selection event missing: %+v", trace.Events)
	}
}

func TestNotifierRecordsIntermediateEvents(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{AgentID: "codex", Protocol: "acp", ChannelID: "http.test"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	notifier := New(store, run.ID, "codex", "acp")
	notifier.SetHeader("codex", "remote-123")
	notifier.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeThinking, Content: "partial"})
	notifier.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeToolCall, Content: "shell ls"})
	notifier.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeToolResult, Content: "ok"})

	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	kinds := map[string]bool{}
	for _, event := range events {
		kinds[event.Kind] = true
	}
	for _, kind := range []string{"agent.message.delta", "tool.call.requested", "tool.result.received"} {
		if !kinds[kind] {
			t.Fatalf("missing event kind %s in %#v", kind, kinds)
		}
	}
	updated, found, err := store.LoadRun(run.ID)
	if err != nil || !found {
		t.Fatalf("load run: found=%v err=%v", found, err)
	}
	if updated.RemoteSessionID != "remote-123" {
		t.Fatalf("expected remote session update, got %s", updated.RemoteSessionID)
	}
}

func TestNotifierProjectsACPPlanThoughtAndUsage(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{AgentID: "opencode", Protocol: "acp", ChannelID: "http.test"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	notifier := New(store, run.ID, "opencode", "acp")
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypeThinking,
		Content: "considering",
		Metadata: map[string]interface{}{
			"source_update_type": "agent_thought_chunk",
		},
	})
	entries := []interface{}{map[string]interface{}{"content": "test", "status": "pending", "priority": "high"}}
	notifier.OnThought(middleware.ThoughtUpdate{
		Type: middleware.ThoughtTypeThinking,
		Metadata: map[string]interface{}{
			"source_update_type": "plan",
			"plan_entries":       entries,
		},
	})
	usage := map[string]interface{}{"inputTokens": 10}
	notifier.OnThought(middleware.ThoughtUpdate{
		Type: middleware.ThoughtTypeThinking,
		Metadata: map[string]interface{}{
			"source_update_type": "usage_update",
			"usage":              usage,
		},
	})

	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	byKind := map[string]runtrace.Event{}
	for _, event := range events {
		byKind[event.Kind] = event
	}
	if byKind["agent.thought.delta"].Summary != "Agent thinking" {
		t.Fatalf("expected thought event, got %#v", byKind)
	}
	plan := byKind["agent.plan.updated"]
	if plan.Outputs["entries"] == nil || plan.Metadata["frontend_visible"] != true {
		t.Fatalf("expected structured plan event, got %#v", plan)
	}
	usageEvent := byKind["agent.usage.updated"]
	if usageEvent.Outputs["usage"] == nil || usageEvent.Metadata["audit_visible"] != true {
		t.Fatalf("expected structured usage event, got %#v", usageEvent)
	}
}

func TestNotifierRecordsMessageDeltaContent(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{AgentID: "codex", Protocol: "acp", ChannelID: "http.test"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	notifier := New(store, run.ID, "codex", "acp")
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypeThinking,
		Content: "OK",
		Metadata: map[string]interface{}{
			"source_update_type": "agent_message_chunk",
		},
	})

	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	for _, event := range events {
		if event.Kind == "agent.message.delta" {
			if event.Message != "OK" {
				t.Fatalf("expected delta message content, got %#v", event)
			}
			return
		}
	}
	t.Fatalf("agent.message.delta event not found: %#v", events)
}

func TestNotifierPreservesExactMessageDeltaBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		chunks   []string
		expected string
	}{
		{name: "trailing space", chunks: []string{"Ciao ", "Jose"}, expected: "Ciao Jose"},
		{name: "newline", chunks: []string{"prima riga\n", "seconda riga"}, expected: "prima riga\nseconda riga"},
		{name: "sub-token", chunks: []string{"con", "testo"}, expected: "contesto"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := runtrace.NewStore(memstore.New())
			run, _, err := store.Start(runtrace.Run{AgentID: "codex", Protocol: "acp", ChannelID: "http.test"})
			if err != nil {
				t.Fatalf("start run: %v", err)
			}
			notifier := New(store, run.ID, "codex", "acp")
			for _, chunk := range tt.chunks {
				notifier.OnThought(middleware.ThoughtUpdate{
					Type:    middleware.ThoughtTypeThinking,
					Content: chunk,
					Metadata: map[string]interface{}{
						"source_update_type":     "agent_message_chunk",
						"message_classification": "unclassified",
					},
				})
			}
			events, err := store.LoadEvents(run.ID, 0)
			if err != nil {
				t.Fatalf("load events: %v", err)
			}
			var got string
			for _, event := range events {
				if event.Kind == "agent.message.delta" {
					got += event.Message
				}
			}
			if got != tt.expected {
				t.Fatalf("delta boundaries changed: got %q want %q", got, tt.expected)
			}
		})
	}
}

func TestNotifierClassifiesProgressFinalAndPostFinalDiagnostic(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{AgentID: "codex", Protocol: "acp", ChannelID: "http.test"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	notifier := New(store, run.ID, "codex", "acp")
	updates := []middleware.ThoughtUpdate{
		{Type: middleware.ThoughtTypeThinking, Content: "Leggo il contesto. ", Metadata: map[string]interface{}{
			"source_update_type": "agent_message_chunk", "message_id": "commentary-1", "message_phase": "commentary", "message_classification": "progress",
		}},
		{Type: middleware.ThoughtTypeThinking, Content: "Ciao Jose", Metadata: map[string]interface{}{
			"source_update_type": "agent_message_chunk", "message_id": "final-1", "message_phase": "final_answer", "message_classification": "final",
		}},
		{Type: middleware.ThoughtTypeThinking, Content: "Warning: disk full\n\n", Metadata: map[string]interface{}{
			"source_update_type": "agent_message_chunk", "message_classification": "unclassified",
		}},
	}
	for _, update := range updates {
		notifier.OnThought(update)
	}
	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	byKind := map[string]runtrace.Event{}
	for _, event := range events {
		byKind[event.Kind] = event
	}
	if byKind["agent.message.progress"].Message != "Leggo il contesto. " {
		t.Fatalf("commentary not projected as progress: %#v", byKind)
	}
	if byKind["agent.message.delta"].Message != "Ciao Jose" {
		t.Fatalf("final delta missing: %#v", byKind)
	}
	diagnostic := byKind["agent.runtime.diagnostic"]
	if diagnostic.Message != "Warning: disk full\n\n" || diagnostic.Metadata["frontend_visible"] != false || diagnostic.Metadata["audit_visible"] != true {
		t.Fatalf("post-final diagnostic not isolated: %#v", diagnostic)
	}
}

func TestNotifierNormalizesFrontendToolEvents(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID:     "opencode",
		Protocol:    "acp",
		ChannelID:   "http.test",
		TracePolicy: runtrace.TracePolicy{ContentMode: runtrace.ContentModeInline, RedactionProfile: "frontend", IncludeProtocolMeta: false},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	notifier := New(store, run.ID, "opencode", "acp")
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypeToolCall,
		Content: "create /tmp/casual_script.sh",
		Metadata: map[string]interface{}{
			"source_update_type": "tool_call",
		},
	})
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypeToolResult,
		Content: "created /tmp/casual_script.sh",
		Metadata: map[string]interface{}{
			"source_update_type": "tool_call_update",
		},
	})

	trace, found, err := store.Trace(run.ID)
	if err != nil || !found {
		t.Fatalf("trace found=%v err=%v", found, err)
	}
	var requested, completed *runtrace.Event
	for i := range trace.Events {
		switch trace.Events[i].Kind {
		case "tool.call.requested":
			requested = &trace.Events[i]
		case "tool.result.received":
			completed = &trace.Events[i]
		}
	}
	if requested == nil || completed == nil {
		t.Fatalf("expected tool request and result events: %#v", trace.Events)
	}
	if requested.ToolCallID == "" || requested.ToolCallID != completed.ToolCallID {
		t.Fatalf("expected stable tool_call_id, got requested=%q completed=%q", requested.ToolCallID, completed.ToolCallID)
	}
	if requested.ToolName != "write_file" || requested.ToolKind != "edit" {
		t.Fatalf("unexpected requested tool normalization: %#v", requested)
	}
	if requested.Status != "pending" || completed.Status != runtrace.StatusCompleted {
		t.Fatalf("unexpected lifecycle statuses: requested=%q completed=%q", requested.Status, completed.Status)
	}
	if requested.Inputs["path"] != "/tmp/casual_script.sh" || completed.Outputs["path"] != "/tmp/casual_script.sh" {
		t.Fatalf("expected structured path metadata: requested=%#v completed=%#v", requested.Inputs, completed.Outputs)
	}
	if len(completed.ArtifactRefs) != 1 || completed.ArtifactRefs[0] != "file:///tmp/casual_script.sh" {
		t.Fatalf("expected artifact ref, got %#v", completed.ArtifactRefs)
	}
	if requested.ProtocolMeta != nil || completed.ProtocolMeta != nil {
		t.Fatalf("expected frontend projection without protocol meta: requested=%#v completed=%#v", requested.ProtocolMeta, completed.ProtocolMeta)
	}
	if requested.Sequence <= 0 || completed.Sequence <= requested.Sequence {
		t.Fatalf("expected increasing sequence, got requested=%d completed=%d", requested.Sequence, completed.Sequence)
	}
}

func TestNotifierRecordsMetadataOnlyACPToolEvents(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID:     "opencode",
		Protocol:    "acp",
		ChannelID:   "http.test",
		TracePolicy: runtrace.TracePolicy{ContentMode: runtrace.ContentModeInline, RedactionProfile: "frontend", IncludeProtocolMeta: false},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	notifier := New(store, run.ID, "opencode", "acp")
	metadata := map[string]interface{}{
		"source_update_type": "tool_call",
		"tool_call_id":       "native-tool-1",
		"tool_name":          "write_file",
		"tool_kind":          "edit",
		"status":             "pending",
		"raw_input": map[string]interface{}{
			"path": "/tmp/noema_matrix_contract.go",
		},
	}
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:     middleware.ThoughtTypeToolCall,
		Title:    "write_file",
		Metadata: metadata,
	})
	resultMetadata := map[string]interface{}{
		"source_update_type": "tool_call_update",
		"tool_call_id":       "native-tool-1",
		"tool_name":          "write_file",
		"tool_kind":          "edit",
		"status":             "completed",
		"raw_input": map[string]interface{}{
			"path": "/tmp/noema_matrix_contract.go",
		},
	}
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:     middleware.ThoughtTypeToolResult,
		Metadata: resultMetadata,
	})

	trace, found, err := store.Trace(run.ID)
	if err != nil || !found {
		t.Fatalf("trace found=%v err=%v", found, err)
	}
	var requested, completed *runtrace.Event
	for i := range trace.Events {
		switch trace.Events[i].Kind {
		case "tool.call.requested":
			requested = &trace.Events[i]
		case "tool.result.received":
			completed = &trace.Events[i]
		}
	}
	if requested == nil || completed == nil {
		t.Fatalf("expected metadata-only tool events in trace: %#v", trace.Events)
	}
	if requested.ToolName != "write_file" || requested.ToolKind != "edit" {
		t.Fatalf("unexpected requested event: %#v", requested)
	}
	if requested.Inputs["path"] != "/tmp/noema_matrix_contract.go" || completed.Outputs["path"] != "/tmp/noema_matrix_contract.go" {
		t.Fatalf("expected path projection: requested=%#v completed=%#v", requested.Inputs, completed.Outputs)
	}
	if len(completed.ArtifactRefs) != 1 || completed.ArtifactRefs[0] != "file:///tmp/noema_matrix_contract.go" {
		t.Fatalf("expected artifact ref, got %#v", completed.ArtifactRefs)
	}
}

func TestNotifierRecordsPermissionAuditEvents(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{AgentID: "opencode", Protocol: "acp", ChannelID: "http.test"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	notifier := New(store, run.ID, "opencode", "acp")
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypePermission,
		Content: `{"path":"/tmp/casual_script.sh"}`,
		Metadata: map[string]interface{}{
			"protocol_method": "session/request_permission",
			"decision":        "approved",
			"option_id":       "once",
			"approval_mode":   "auto",
		},
	})

	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	var requested, resolved *runtrace.Event
	for i := range events {
		switch events[i].Kind {
		case "permission.requested":
			requested = &events[i]
		case "permission.resolved":
			resolved = &events[i]
		}
	}
	if requested == nil || resolved == nil {
		t.Fatalf("expected permission requested/resolved events: %#v", events)
	}
	if requested.PermissionID == "" || requested.PermissionID != resolved.PermissionID {
		t.Fatalf("expected stable permission_id, got requested=%q resolved=%q", requested.PermissionID, resolved.PermissionID)
	}
	if requested.Metadata["frontend_visible"] != false || requested.Metadata["audit_visible"] != true {
		t.Fatalf("expected permission request audit metadata, got %#v", requested.Metadata)
	}
	if resolved.Outputs["decision"] != "approved" || resolved.Outputs["option_id"] != "once" {
		t.Fatalf("expected permission resolution outputs, got %#v", resolved.Outputs)
	}
}

func TestNotifierCorrelatesACPToolWithPermissionContext(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID:     "opencode",
		Protocol:    "acp",
		ChannelID:   "http.test",
		TracePolicy: runtrace.TracePolicy{ContentMode: runtrace.ContentModeInline, RedactionProfile: "frontend", IncludeProtocolMeta: false},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	notifier := New(store, run.ID, "opencode", "acp")
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypeToolCall,
		Content: "write",
		Metadata: map[string]interface{}{
			"source_update_type": "tool_call",
		},
	})
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypePermission,
		Content: `{"path":"/tmp/noema_matrix_contract.sh","command":"chmod +x /tmp/noema_matrix_contract.sh"}`,
		Metadata: map[string]interface{}{
			"protocol_method": "session/request_permission",
			"decision":        "approved",
			"option_id":       "once",
			"approval_mode":   "auto",
		},
	})
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypeToolResult,
		Content: "write",
		Metadata: map[string]interface{}{
			"source_update_type": "tool_call_update",
		},
	})

	trace, found, err := store.Trace(run.ID)
	if err != nil || !found {
		t.Fatalf("trace found=%v err=%v", found, err)
	}
	var requested, completed, permission *runtrace.Event
	for i := range trace.Events {
		switch trace.Events[i].Kind {
		case "tool.call.requested":
			requested = &trace.Events[i]
		case "tool.result.received":
			completed = &trace.Events[i]
		case "permission.requested":
			permission = &trace.Events[i]
		}
	}
	if requested == nil || completed == nil || permission == nil {
		t.Fatalf("expected tool and permission events: %#v", trace.Events)
	}
	if requested.Summary != "Create /tmp/noema_matrix_contract.sh" {
		t.Fatalf("expected enriched requested summary, got %q", requested.Summary)
	}
	if requested.Inputs["path"] != "/tmp/noema_matrix_contract.sh" {
		t.Fatalf("expected requested path from permission context, got %#v", requested.Inputs)
	}
	// The kind came from the word "write" in the request, not from protocol
	// metadata or a declared tool identity, so the completed summary stays
	// neutral: it must not turn a content guess into a factual creation claim.
	if completed.Summary != "Completed write_file" {
		t.Fatalf("expected neutral completion summary for a certainty-limited kind, got %q", completed.Summary)
	}
	if completed.Outputs["path"] != "/tmp/noema_matrix_contract.sh" {
		t.Fatalf("expected completed output path, got %#v", completed.Outputs)
	}
	if len(completed.ArtifactRefs) != 1 || completed.ArtifactRefs[0] != "file:///tmp/noema_matrix_contract.sh" {
		t.Fatalf("expected completed artifact ref, got %#v", completed.ArtifactRefs)
	}
	if requested.Metadata["frontend_visible"] != true || permission.Metadata["frontend_visible"] != false {
		t.Fatalf("unexpected frontend visibility: tool=%#v permission=%#v", requested.Metadata, permission.Metadata)
	}
	if requested.ProtocolMeta != nil || completed.ProtocolMeta != nil {
		t.Fatalf("expected frontend projection without protocol meta: requested=%#v completed=%#v", requested.ProtocolMeta, completed.ProtocolMeta)
	}
}

func TestPresentationUpdatesAreNotTurnWorkEvidence(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{AgentID: "opencode", Protocol: "acp", ChannelID: "http.test"})
	if err != nil {
		t.Fatal(err)
	}
	n := New(store, run.ID, "opencode", "acp")
	for _, kind := range []middleware.ThoughtUpdateType{middleware.ThoughtTypeNotice, middleware.ThoughtTypeCompaction, middleware.ThoughtTypeSessionState} {
		n.OnThought(middleware.ThoughtUpdate{Type: kind, Content: "status only"})
	}
	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, event := range events {
		kinds[event.Kind] = true
		if event.Kind == "agent.message.delta" || event.Kind == "agent.message.final" || event.Kind == "run.completed" {
			t.Fatalf("presentation became turn evidence: %s", event.Kind)
		}
	}
	for _, kind := range []string{"agent.notice", "agent.context.compaction", "agent.session.state"} {
		if !kinds[kind] {
			t.Fatalf("missing neutral event %s", kind)
		}
	}
}

// TestNotifierKeepsAReadResultAReadWhenThePatchOmitsKind is the end-to-end
// regression for the external reader/proof trace. The call declares read_file,
// the update patch omits the unchanged kind and carries page text that quotes
// delete and update words, and the trace must still show a read: the identity is
// inherited from the correlated call, never guessed from the result body.
func TestNotifierKeepsAReadResultAReadWhenThePatchOmitsKind(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{AgentID: "opencode", Protocol: "acp", ChannelID: "http.test"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	notifier := New(store, run.ID, "opencode", "acp")
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypeToolCall,
		Content: "read the public documentation page",
		Metadata: map[string]interface{}{
			"source_update_type": "tool_call",
			"title":              "read_file",
		},
	})
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypeToolResult,
		Content: "Public page /workspace/docs: delete the stale workspace, remove files, write and edit sections.",
		Metadata: map[string]interface{}{
			"source_update_type": "tool_call_update",
			"status":             "completed",
		},
	})

	trace, found, err := store.Trace(run.ID)
	if err != nil || !found {
		t.Fatalf("trace found=%v err=%v", found, err)
	}
	var requested, completed *runtrace.Event
	for i := range trace.Events {
		switch trace.Events[i].Kind {
		case "tool.call.requested":
			requested = &trace.Events[i]
		case "tool.result.received":
			completed = &trace.Events[i]
		}
	}
	if requested == nil || completed == nil {
		t.Fatalf("expected tool request and result events: %#v", trace.Events)
	}
	if requested.ToolName != "read_file" || requested.ToolKind != "read" {
		t.Fatalf("the call must be a read, got name=%q kind=%q", requested.ToolName, requested.ToolKind)
	}
	if completed.ToolName != "read_file" || completed.ToolKind != "read" || completed.ToolEffect != "read_only" {
		t.Fatalf("the result must inherit the call identity, got name=%q kind=%q effect=%q", completed.ToolName, completed.ToolKind, completed.ToolEffect)
	}
	if completed.ToolClassificationSource != "heuristic_fallback" || completed.ToolClassificationConfidence != "low" {
		t.Fatalf("inherited identity must stay low confidence, got source=%q confidence=%q", completed.ToolClassificationSource, completed.ToolClassificationConfidence)
	}
	if completed.Summary != "Completed read_file" {
		t.Fatalf("expected a neutral read summary, got %q", completed.Summary)
	}
	if completed.ToolCallID == "" || completed.ToolCallID != requested.ToolCallID {
		t.Fatalf("the result must correlate with its call, got %q vs %q", completed.ToolCallID, requested.ToolCallID)
	}
}

// An ACP turn can have more than one tool call in flight. A completion must keep
// the identity and context of the call it belongs to, even when another request
// is the most recent one and the completions arrive out of request order.
func TestNotifierCorrelatesInterleavedToolCalls(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{AgentID: "opencode", Protocol: "acp", ChannelID: "http.test"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	notifier := New(store, run.ID, "opencode", "acp")
	toolCall := func(id, path, content string) {
		notifier.OnThought(middleware.ThoughtUpdate{
			Type:    middleware.ThoughtTypeToolCall,
			Content: content,
			Metadata: map[string]interface{}{
				"tool_call_id":       id,
				"tool_name":          "read_file",
				"source_update_type": "tool_call",
				"path":               path,
				"status":             "pending",
			},
		})
	}
	toolResult := func(id, content string) {
		notifier.OnThought(middleware.ThoughtUpdate{
			Type:    middleware.ThoughtTypeToolResult,
			Content: content,
			Metadata: map[string]interface{}{
				"tool_call_id":       id,
				"tool_name":          "read_file",
				"source_update_type": "tool_call_update",
				"status":             "completed",
			},
		})
	}

	toolCall("call-a", "/workspace/report.md", "Read report")
	toolCall("call-b", "/workspace/plan.md", "Read plan")
	toolResult("call-a", "Read report")
	toolResult("call-b", "Read plan")

	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	var requested, results []runtrace.Event
	for _, event := range events {
		switch event.Kind {
		case "tool.call.requested":
			requested = append(requested, event)
		case "tool.result.received":
			results = append(results, event)
		}
	}
	if len(requested) != 2 || len(results) != 2 {
		t.Fatalf("expected two requests and two results, got requests=%#v results=%#v", requested, results)
	}
	wantPaths := []string{"/workspace/report.md", "/workspace/plan.md"}
	for i := range results {
		if results[i].ToolCallID == "" || results[i].ToolCallID != requested[i].ToolCallID {
			t.Fatalf("completion %d lost its request identity: requested=%q completed=%q", i, requested[i].ToolCallID, results[i].ToolCallID)
		}
		if got := results[i].Inputs["path"]; got != wantPaths[i] {
			t.Fatalf("completion %d inherited the wrong call context: got %#v, want path %q", i, got, wantPaths[i])
		}
	}
}
