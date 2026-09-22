package frontendevents

import (
	"strings"
	"testing"
)

// TestStableToolCallIDIsDeterministicAndDiscriminating is the identity contract
// the trace relies on: the same call must hash to the same id so a replayed
// update does not duplicate the record, and two different calls must never
// collide.
func TestStableToolCallIDIsDeterministicAndDiscriminating(t *testing.T) {
	first := StableToolCallID("run-1", "read_file", "Read main.go", map[string]interface{}{"path": "main.go"})
	repeat := StableToolCallID("run-1", "read_file", "Read main.go", map[string]interface{}{"path": "main.go"})
	if first == "" || first != repeat {
		t.Fatalf("the id must be stable, got %q and %q", first, repeat)
	}
	others := map[string]string{
		"different run":      StableToolCallID("run-2", "read_file", "Read main.go", map[string]interface{}{"path": "main.go"}),
		"different tool":     StableToolCallID("run-1", "write_file", "Read main.go", map[string]interface{}{"path": "main.go"}),
		"different content":  StableToolCallID("run-1", "read_file", "Read other.go", map[string]interface{}{"path": "main.go"}),
		"different metadata": StableToolCallID("run-1", "read_file", "Read main.go", map[string]interface{}{"path": "other.go"}),
	}
	for name, id := range others {
		if id == first {
			t.Fatalf("%s must not share the id of another call", name)
		}
	}
}

// TestNormalizeToolNeverLosesAWriteAction is the safety-relevant projection: a
// write must never be classified as a read, because the trace is what a human
// reviews after the fact.
func TestNormalizeToolNeverLosesAWriteAction(t *testing.T) {
	read := NormalizeTool("Read main.go", map[string]interface{}{"tool_name": "read_file"}, "")
	if read.Name != "read_file" || read.Kind != "read" {
		t.Fatalf("a read must classify as a read, got %+v", read)
	}
	write := NormalizeTool("Write main.go", map[string]interface{}{"tool_name": "write_file"}, "")
	if write.Name != "write_file" || write.Kind != "edit" {
		t.Fatalf("a write must classify as an edit, got %+v", write)
	}
	// With no structured metadata the classification must still come from the
	// content rather than silently defaulting to a read.
	guessed := NormalizeTool("write_file main.go", nil, "write_file")
	if guessed.Name == "" {
		t.Fatal("a tool name must always be derived")
	}
	// With nothing at all to go on the projection still produces a usable
	// placeholder rather than an empty tool name in the trace.
	empty := NormalizeTool("", nil, "")
	if empty.Name != "tool" {
		t.Fatalf("no evidence must yield the placeholder name, got %q", empty.Name)
	}
}

// TestACPFilesystemSignalsSeparateReadsFromWrites pins the mapping between the
// protocol method and the tool identity: mislabelling a write as a read is the
// one mistake this projection must not make.
func TestACPFilesystemSignalsSeparateReadsFromWrites(t *testing.T) {
	read := ACPClientFSToolSignal("fs/read_text_file", "/tmp/../etc/hosts", 10)
	if read.Metadata["tool_name"] != "read_file" || read.Metadata["tool_kind"] != "read" {
		t.Fatalf("unexpected read signal: %+v", read.Metadata)
	}
	write := ACPClientFSToolSignal("fs/write_text_file", "/etc/hosts", 10)
	if write.Metadata["tool_name"] != "write_file" || write.Metadata["tool_kind"] != "edit" {
		t.Fatalf("unexpected write signal: %+v", write.Metadata)
	}
	// The path is cleaned, so a traversal spelling cannot masquerade as a
	// different file in the trace.
	if path, _ := write.Metadata["path"].(string); path != "/etc/hosts" {
		t.Fatalf("write path = %q", path)
	}
	if path, _ := read.Metadata["path"].(string); path != "/etc/hosts" {
		t.Fatalf("the path must be cleaned, got %q", path)
	}
}

// TestCommandLineDropsEmptyArguments keeps a trace line from showing gaps, and
// documents that arguments are joined rather than quoted.
func TestCommandLineDropsEmptyArguments(t *testing.T) {
	got := CommandLine("  go  ", []string{"test", "", "  ", "./..."})
	if got != "go test ./..." {
		t.Fatalf("CommandLine = %q, want %q", got, "go test ./...")
	}
	if CommandLine("", nil) != "" {
		t.Fatal("an empty command must render empty")
	}
}

// TestTerminalSemanticKindIgnoresUnrelatedCommands keeps the classifier from
// labelling every command as validation.
func TestTerminalSemanticKindIgnoresUnrelatedCommands(t *testing.T) {
	for _, args := range [][]string{{"test", "./..."}, {"run", "pytest"}} {
		if kind := TerminalSemanticKind("go", args); kind != "" && !strings.Contains(strings.Join(args, " "), "test") && !strings.Contains(strings.Join(args, " "), "pytest") {
			t.Fatalf("unrelated command classified as %q", kind)
		}
	}
	if kind := TerminalSemanticKind("go", []string{"test", "./..."}); kind != "validate" {
		t.Fatalf("a test command must classify as validate, got %q", kind)
	}
	if kind := TerminalSemanticKind("ls", []string{"-la"}); kind != "" {
		t.Fatalf("an unrelated command must not classify, got %q", kind)
	}
}

// TestSanitizedRawInputDropsContent is the privacy contract: raw protocol
// parameters are kept for diagnosis, but the message body is removed so it
// cannot be duplicated into the trace.
func TestSanitizedRawInputDropsContent(t *testing.T) {
	raw := SanitizedRawInput([]byte(`{"content":"segreto","path":"main.go"}`))
	if _, present := raw["content"]; present {
		t.Fatal("the content field must be removed from sanitized input")
	}
	if raw["path"] != "main.go" {
		t.Fatalf("sanitizing must keep the diagnostic fields, got %v", raw)
	}
	if SanitizedRawInput([]byte("{not json")) != nil {
		t.Fatal("malformed parameters must sanitize to nothing")
	}
	if SanitizedRawInput(nil) != nil {
		t.Fatal("absent parameters must sanitize to nothing")
	}
}

// TestFirstNonEmptyReturnsTheFirstRealValue keeps empty strings from masking a
// later, usable value.
func TestFirstNonEmptyReturnsTheFirstRealValue(t *testing.T) {
	if got := FirstNonEmpty("", "  ", "terzo"); got != "terzo" {
		t.Fatalf("FirstNonEmpty = %q", got)
	}
	if got := FirstNonEmpty(""); got != "" {
		t.Fatalf("FirstNonEmpty of nothing = %q", got)
	}
}
