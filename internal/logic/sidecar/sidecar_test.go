package sidecar

import (
	"strings"
	"testing"

	"github.com/jose/matrix-v2/internal/middleware"
)

func TestProjectPromptOnlyIncludesLLMVisibleCapsules(t *testing.T) {
	got := ProjectPrompt("TASK:\nUpdate parser", []middleware.SidecarCapsule{
		{Provider: "noema", ID: "caps-visible", Visibility: middleware.SidecarVisibilityLLMVisible, Content: "<noema id=\"caps-visible\">visible</noema>"},
		{Provider: "noema", ID: "caps-trace", Visibility: middleware.SidecarVisibilityTraceOnly, Content: "<noema id=\"caps-trace\">hidden</noema>"},
	})
	if !strings.Contains(got, "<noema id=\"caps-visible\">visible</noema>") {
		t.Fatalf("expected visible capsule in prompt, got %q", got)
	}
	if strings.Contains(got, "caps-trace") {
		t.Fatalf("trace-only capsule leaked into prompt: %q", got)
	}
}

func TestValidateCapsules(t *testing.T) {
	if err := ValidateCapsules([]middleware.SidecarCapsule{{Provider: "noema", ID: "caps-ok", Visibility: middleware.SidecarVisibilityLLMVisible, Content: "<noema/>"}}); err != nil {
		t.Fatalf("expected valid capsule: %v", err)
	}
	if err := ValidateCapsules([]middleware.SidecarCapsule{{Provider: "noema", ID: "caps-missing-content", Visibility: middleware.SidecarVisibilityLLMVisible}}); err == nil {
		t.Fatal("expected llm_visible capsule without content to fail")
	}
	if err := ValidateCapsules([]middleware.SidecarCapsule{{Provider: "noema", ID: "caps-future", Visibility: "future_visibility"}}); err != nil {
		t.Fatalf("future visibility values should be accepted as trace-only by Matrix: %v", err)
	}
}
