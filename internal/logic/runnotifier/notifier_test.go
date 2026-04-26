package runnotifier

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

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
	if completed.Summary != "Created /tmp/noema_matrix_contract.sh" {
		t.Fatalf("expected enriched completion summary, got %q", completed.Summary)
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
