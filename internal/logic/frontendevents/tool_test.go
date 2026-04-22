package frontendevents

import "testing"

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
