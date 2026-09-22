package frontendevents

import (
	"testing"
)

// TestFirstPathPrefersAnAbsoluteDeclaredPath is the rule that keeps the trace
// honest: only an absolute path can be trusted as "the file this call touched",
// so a relative or absent value must not be reported as one.
func TestFirstPathPrefersAnAbsoluteDeclaredPath(t *testing.T) {
	cases := map[string]struct {
		content  string
		metadata map[string]interface{}
		want     string
	}{
		"declared path":       {"", map[string]interface{}{"path": "/tmp/../etc/hosts"}, "/etc/hosts"},
		"declared filename":   {"", map[string]interface{}{"filename": "/var/log/matrix.log"}, "/var/log/matrix.log"},
		"relative is ignored": {"", map[string]interface{}{"path": "main.go"}, ""},
		"nested map":          {"", map[string]interface{}{"input": map[string]interface{}{"path": "/a/b"}}, "/a/b"},
		"nested slice":        {"", map[string]interface{}{"edits": []interface{}{map[string]interface{}{"file": "/a/c"}}}, "/a/c"},
		"from content json":   {`{"path":"/from/content"}`, nil, "/from/content"},
		"nothing at all":      {"", nil, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := FirstPath(tc.content, tc.metadata); got != tc.want {
				t.Fatalf("FirstPath = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFirstPathFallsBackToATextMatch keeps a path visible even when the provider
// only mentions it inside a sentence, which is common in agent narration.
func TestFirstPathFallsBackToATextMatch(t *testing.T) {
	got := FirstPath("I edited /home/jose/project/main.go to fix the bug", nil)
	if got != "/home/jose/project/main.go" {
		t.Fatalf("a path in prose must be recovered, got %q", got)
	}
	if got := FirstPath("no path here", nil); got != "" {
		t.Fatalf("text without a path must yield nothing, got %q", got)
	}
}

// TestToolOperationReadsTheCommandThatRan pins the extraction order and the
// rejection rules: a bare command name is an operation, a path or a name with a
// separator is not.
func TestToolOperationReadsTheCommandThatRan(t *testing.T) {
	cases := map[string]struct {
		content  string
		metadata map[string]interface{}
		want     string
	}{
		"operation key":     {"", map[string]interface{}{"operation": "Write"}, "Write"},
		"command key":       {"", map[string]interface{}{"command": "/usr/bin/go test"}, "go"},
		"cmd key":           {"", map[string]interface{}{"cmd": "/bin/ls -la"}, "ls"},
		"nested command":    {"", map[string]interface{}{"input": map[string]interface{}{"command": "npm"}}, "npm"},
		"from content json": {`{"command":"cargo"}`, nil, "cargo"},
		"path is not an op": {"", map[string]interface{}{"command": "/usr/bin/"}, ""},
		"nothing":           {"just prose", nil, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ToolOperation(tc.content, tc.metadata); got != tc.want {
				t.Fatalf("ToolOperation = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestNormalizeToolNameAndKind pins the canonical vocabulary the UI and the
// filters depend on.
func TestNormalizeToolNameAndKind(t *testing.T) {
	if got := NormalizeToolName("  Read-File "); got != "read_file" {
		t.Fatalf("NormalizeToolName = %q, want read_file", got)
	}
	if got := NormalizeToolName(""); got != "tool" {
		t.Fatalf("an empty name must fall back to the placeholder, got %q", got)
	}
	if got := NormalizeToolKind("read_file", ""); got != "read" {
		t.Fatalf("a read must normalize to the read kind, got %q", got)
	}
	if got := NormalizeToolKind("mystery_tool", "some content"); got != "other" {
		t.Fatalf("an unknown tool must normalize to other, got %q", got)
	}
	kind, source, confidence := NormalizeToolKindFromMetadata(map[string]interface{}{"tool_kind": "edit"}, "x", "")
	if kind != "edit" || source != "protocol_metadata" || confidence != "high" {
		t.Fatalf("declared metadata must win with high confidence, got %q %q %q", kind, source, confidence)
	}
	kind, source, confidence = NormalizeToolKindFromMetadata(nil, "mystery", "")
	if kind != "other" || source != "unknown" || confidence != "low" {
		t.Fatalf("with no evidence the classification must be reported as unknown, got %q %q %q", kind, source, confidence)
	}
}

// TestNormalizeToolStatusSeparatesRunningFromFinished keeps an unfinished tool
// call from being recorded as completed, which would make a hung run look done.
func TestNormalizeToolStatusSeparatesRunningFromFinished(t *testing.T) {
	for raw, want := range map[string]string{
		"pending":     "pending",
		"started":     "pending",
		"in_progress": "running",
		"running":     "running",
		"failed":      "failed",
		"error":       "failed",
		"completed":   "completed",
		"anything":    "completed",
		"":            "completed",
	} {
		if got := NormalizeToolStatus(raw); got != want {
			t.Fatalf("NormalizeToolStatus(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestTerminalSignalDistinguishesRunningFromExited is the subtle one: a nil exit
// code means the command is still running, and recording it as zero would claim
// a success that has not happened.
func TestTerminalSignalDistinguishesRunningFromExited(t *testing.T) {
	running := ACPClientTerminalToolSignal("go", []string{"test", "./..."}, "/tmp", nil)
	if _, present := running.Metadata["exit_code"]; present {
		t.Fatal("a running command must not carry an exit code")
	}
	if running.Metadata["tool_name"] != "shell" || running.Metadata["tool_kind"] != "execute" {
		t.Fatalf("unexpected terminal identity: %v", running.Metadata)
	}
	if running.Metadata["tool_semantic_kind"] != "validate" {
		t.Fatalf("a test command must be marked as validation, got %v", running.Metadata["tool_semantic_kind"])
	}

	code := 0
	exited := ACPClientTerminalToolSignal("go", []string{"build"}, "/tmp", &code)
	if exited.Metadata["exit_code"] != 0 {
		t.Fatalf("a finished command must carry its exit code, got %v", exited.Metadata["exit_code"])
	}
	if exited.Metadata["tool_semantic_kind"] != "" {
		t.Fatalf("a build is not a validation, got %v", exited.Metadata["tool_semantic_kind"])
	}
}

// TestToolContentKeepsTheMethodVisible keeps the method in the trace when there
// is no subject, instead of producing an empty line.
func TestToolContentKeepsTheMethodVisible(t *testing.T) {
	if got := ToolContent("fs/read_text_file", "/a/b"); got != "fs/read_text_file /a/b" {
		t.Fatalf("ToolContent = %q", got)
	}
	if got := ToolContent("fs/read_text_file", "   "); got != "fs/read_text_file" {
		t.Fatalf("ToolContent without a subject = %q", got)
	}
}

// TestNormalizePermissionCarriesTheDecision keeps a denial distinguishable from
// an approval in the audit trail.
func TestNormalizePermissionCarriesTheDecision(t *testing.T) {
	unresolved := NormalizePermission("run-1", "Allow running rm -rf?", nil)
	if unresolved.ID == "" {
		t.Fatalf("a permission request must be identified: %+v", unresolved)
	}
	// With no decision the event must say so rather than implying a resolution.
	if unresolved.ResolutionOutputs["decision"] != "unknown" {
		t.Fatalf("an unresolved permission must report an unknown decision, got %v", unresolved.ResolutionOutputs)
	}
	if unresolved.RequestInputs != nil {
		t.Fatalf("a request without a path or command must carry no inputs, got %v", unresolved.RequestInputs)
	}

	granted := NormalizePermission("run-1", "Allow?", map[string]interface{}{"decision": "allow", "option_id": "yes"})
	denied := NormalizePermission("run-1", "Allow?", map[string]interface{}{"decision": "deny"})
	if granted.ResolutionOutputs["decision"] == denied.ResolutionOutputs["decision"] {
		t.Fatalf("an approval and a denial must not share a decision: %v", granted.ResolutionOutputs)
	}
	if granted.ResolutionSummary == denied.ResolutionSummary {
		t.Fatal("an approval and a denial must not share a summary")
	}
	if granted.ResolutionOutputs["option_id"] != "yes" {
		t.Fatalf("the chosen option must be recorded, got %v", granted.ResolutionOutputs)
	}
	// A permission carries the path it is about, which is what makes it
	// auditable after the fact.
	onPath := NormalizePermission("run-1", "Allow writing /etc/hosts?", nil)
	if onPath.RequestInputs["path"] != "/etc/hosts" {
		t.Fatalf("the permission must name the path, got %v", onPath.RequestInputs)
	}
	// The stable id must depend on the request, not on the moment it arrived.
	first := StablePermissionID("run-1", "same")
	repeat := StablePermissionID("run-1", "same")
	if first != repeat {
		t.Fatalf("a permission id must be stable for the same request: %q vs %q", first, repeat)
	}
	if StablePermissionID("run-1", "a") == StablePermissionID("run-1", "b") {
		t.Fatal("two different requests must not share an id")
	}
}

// TestMetadataHelpersKeepTheContractStable covers the small helpers every event
// builder leans on.
func TestMetadataHelpersKeepTheContractStable(t *testing.T) {
	if got := StringValue(map[string]interface{}{"k": 42}, "k"); got != "" {
		t.Fatalf("a non-string value must not be coerced to text, got %q", got)
	}
	if got := StringValue(map[string]interface{}{"k": " v "}, "k"); got != "v" {
		t.Fatalf("StringValue must trim, got %q", got)
	}
	if got := SourceUpdateType(nil, "fallback"); got != "fallback" {
		t.Fatalf("an absent source update type must fall back, got %q", got)
	}
	if got := ProtocolMeta(nil); got != nil {
		t.Fatalf("absent metadata must produce no protocol metadata, got %v", got)
	}
	meta := ProtocolMeta(map[string]interface{}{"foo": "bar"})
	if meta == nil || meta["foo"] != "bar" {
		t.Fatalf("protocol metadata must pass the source through, got %v", meta)
	}
	merged := Merge(map[string]interface{}{"a": 1}, map[string]interface{}{"a": 2, "b": 3})
	if merged["a"] != 2 || merged["b"] != 3 {
		t.Fatalf("Merge must let the extra map win, got %v", merged)
	}
	if got := Merge(nil, map[string]interface{}{"b": 3}); got["b"] != 3 {
		t.Fatalf("Merge must handle a nil base, got %v", got)
	}
}

// TestEnrichToolWithContextIgnoresEmptyEvidence keeps the enrichment from
// overwriting what the provider already supplied with nothing.
func TestEnrichToolWithContextIgnoresEmptyEvidence(t *testing.T) {
	base := NormalizeTool("Read main.go", map[string]interface{}{"tool_name": "read_file"}, "")
	unchanged := EnrichToolWithContext(base, "", "")
	if unchanged.Summary != base.Summary {
		t.Fatalf("no context must not change the summary: %q vs %q", unchanged.Summary, base.Summary)
	}
	enriched := EnrichToolWithContext(base, "/a/b.go", "read")
	if enriched.Summary == "" {
		t.Fatal("enrichment must produce a summary")
	}
}
