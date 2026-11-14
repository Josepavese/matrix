package frontendevents

import (
	"strings"
	"testing"
)

func TestNormalizeToolPrefersProtocolKindOverNameHeuristics(t *testing.T) {
	tool := NormalizeTool("running grep", map[string]interface{}{
		"tool_name": "grep_workspace",
		"tool_kind": "execute",
		"status":    "running",
	}, "")
	if tool.Kind != "execute" {
		t.Fatalf("expected protocol tool kind, got %q", tool.Kind)
	}
	if tool.ClassificationSource != "protocol_metadata" || tool.ClassificationConfidence != "high" {
		t.Fatalf("expected protocol classification, got source=%q confidence=%q", tool.ClassificationSource, tool.ClassificationConfidence)
	}
	if tool.Effect != "execute" || tool.SubjectKind != "process" {
		t.Fatalf("unexpected structural taxonomy: effect=%q subject=%q", tool.Effect, tool.SubjectKind)
	}
}

func TestNormalizeToolCarriesSemanticKindEffectAndSubject(t *testing.T) {
	tool := NormalizeTool("", map[string]interface{}{
		"tool_name":          "run_go_test",
		"tool_kind":          "execute",
		"tool_semantic_kind": "validate",
		"tool_effect":        "read_only",
		"tool_subject_kind":  "repository",
	}, "")
	if tool.Kind != "execute" || tool.SemanticKind != "validate" || tool.Effect != "read_only" || tool.SubjectKind != "repository" {
		t.Fatalf("unexpected taxonomy: %#v", tool)
	}
}

func TestNormalizeToolFallbackIsMarkedLowConfidence(t *testing.T) {
	tool := NormalizeTool("write /tmp/file", nil, "")
	if tool.Kind != "edit" {
		t.Fatalf("expected heuristic edit kind, got %q", tool.Kind)
	}
	if tool.ClassificationSource != "heuristic_fallback" || tool.ClassificationConfidence != "low" {
		t.Fatalf("expected low-confidence fallback, got source=%q confidence=%q", tool.ClassificationSource, tool.ClassificationConfidence)
	}
}

// TestReadResultsWithMutationWordsAreNotWrites is the regression for the
// external reader/proof trace: a completed read whose result body quotes a
// public page mentioning delete, remove, write or edit must stay a read. The
// kind is decided by the declared tool identity, and the summary may not turn
// the quoted words into a past-tense write claim.
func TestReadResultsWithMutationWordsAreNotWrites(t *testing.T) {
	cases := map[string]struct {
		content  string
		metadata map[string]interface{}
	}{
		"page read result": {
			content:  "Public page /workspace/docs: delete the stale workspace, remove files, write and edit sections.",
			metadata: map[string]interface{}{"source_update_type": "tool_call_update", "title": "read_file", "status": "completed"},
		},
		"proof lookup result": {
			content:  `{"status":"ok","network_policy":"public","kind":"proof_record","path":"/workspace/notes/a.md","text":"The proof record was edited and updated."}`,
			metadata: map[string]interface{}{"source_update_type": "tool_call_update", "tool_name": "read_file", "status": "completed"},
		},
		"read request with quoted words": {
			content:  "Delete /workspace/cache while you read the page",
			metadata: map[string]interface{}{"source_update_type": "tool_call", "title": "read_file"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tool := NormalizeTool(tc.content, tc.metadata, "")
			if tool.Name != "read_file" || tool.Kind != "read" || tool.Effect != "read_only" {
				t.Fatalf("a read must stay a read, got name=%q kind=%q effect=%q", tool.Name, tool.Kind, tool.Effect)
			}
			if !tool.KindFromIdentity {
				t.Fatal("the read kind must be attributed to the declared tool identity")
			}
			if tool.ClassificationSource != "heuristic_fallback" || tool.ClassificationConfidence != "low" {
				t.Fatalf("identity fallback must stay low confidence, got source=%q confidence=%q", tool.ClassificationSource, tool.ClassificationConfidence)
			}
			assertNoAffirmativeWriteSummary(t, tool.Summary)
		})
	}
}

// TestToolResultWithoutIdentityStaysExplicitlyUnknown pins the second half of
// the contract: when a result carries neither a protocol kind nor a declared
// identity, Matrix must not classify it from the body words at all.
func TestToolResultWithoutIdentityStaysExplicitlyUnknown(t *testing.T) {
	tool := NormalizeTool(
		"The proof record for /workspace/notes was edited and updated, then the workspace was removed.",
		map[string]interface{}{"source_update_type": "tool_call_update", "status": "completed"},
		"",
	)
	if tool.Kind != "other" || tool.Effect != "unknown" || tool.SubjectKind != "unknown" {
		t.Fatalf("an unidentified result must stay unknown, got kind=%q effect=%q subject=%q", tool.Kind, tool.Effect, tool.SubjectKind)
	}
	if tool.ClassificationSource != "unknown" || tool.ClassificationConfidence != "low" {
		t.Fatalf("an unidentified result must be reported as unknown, got source=%q confidence=%q", tool.ClassificationSource, tool.ClassificationConfidence)
	}
	if tool.Summary != "Completed tool" {
		t.Fatalf("an unidentified result needs a neutral summary, got %q", tool.Summary)
	}
	assertNoAffirmativeWriteSummary(t, tool.Summary)
}

// TestDeclaredToolIdentityKeepsAWriteAWrite keeps the correction from losing
// real writes: a declared write identity (or a protocol kind) still produces the
// past-tense creation summary, even when the result body happens to contain
// delete or remove words.
func TestDeclaredToolIdentityKeepsAWriteAWrite(t *testing.T) {
	identity := NormalizeTool(
		"removed and deleted the old copy",
		map[string]interface{}{"source_update_type": "tool_call_update", "title": "write_file", "status": "completed", "path": "/tmp/x"},
		"",
	)
	if identity.Name != "write_file" || identity.Kind != "edit" || identity.Effect != "write" || !identity.KindFromIdentity {
		t.Fatalf("a declared write identity must stay a write, got %#v", identity)
	}
	if identity.Summary != "Created /tmp/x" {
		t.Fatalf("a verified write must keep its creation summary, got %q", identity.Summary)
	}
	protocol := NormalizeTool(
		"removed and deleted the old copy",
		map[string]interface{}{"source_update_type": "tool_call_update", "tool_name": "write_file", "tool_kind": "edit", "status": "completed", "path": "/tmp/y"},
		"",
	)
	if protocol.Kind != "edit" || protocol.ClassificationSource != "protocol_metadata" || protocol.Summary != "Created /tmp/y" {
		t.Fatalf("protocol metadata must keep the write summary, got kind=%q source=%q summary=%q", protocol.Kind, protocol.ClassificationSource, protocol.Summary)
	}
}

func assertNoAffirmativeWriteSummary(t *testing.T, summary string) {
	t.Helper()
	for _, verb := range []string{"Deleted", "Updated", "Created"} {
		if strings.HasPrefix(summary, verb+" ") {
			t.Fatalf("summary %q must not assert a write from content words", summary)
		}
	}
}
