package sidecar

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func validCapsule() middleware.SidecarCapsule {
	return middleware.SidecarCapsule{
		Provider:   "noema",
		ID:         "cap-1",
		Visibility: middleware.SidecarVisibilityTraceOnly,
		Content:    "body",
	}
}

// TestValidateCapsulesRejectsIncompleteCapsules is the gate that protects the
// whole sidecar path: the error must name the offending entry so an operator can
// fix the producer, and it must reject rather than coerce.
func TestValidateCapsulesRejectsIncompleteCapsules(t *testing.T) {
	missingProvider := validCapsule()
	missingProvider.Provider = ""
	missingID := validCapsule()
	missingID.ID = ""
	// A model-visible capsule without content would reach the model as an empty
	// block, which reads as a real message that says nothing.
	emptyVisible := validCapsule()
	emptyVisible.Visibility = middleware.SidecarVisibilityLLMVisible
	emptyVisible.Content = ""

	cases := map[string]struct {
		capsules []middleware.SidecarCapsule
		want     string
	}{
		"missing provider":          {[]middleware.SidecarCapsule{missingProvider}, "provider is required"},
		"missing id":                {[]middleware.SidecarCapsule{missingID}, "id is required"},
		"empty llm_visible content": {[]middleware.SidecarCapsule{emptyVisible}, "content is required"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateCapsules(tc.capsules)
			if err == nil {
				t.Fatal("an incomplete capsule must be rejected")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q must explain the problem (%q)", err, tc.want)
			}
		})
	}
}

// TestValidateCapsulesAcceptsACompleteSet keeps the validator from rejecting
// valid input, including the index it reports.
func TestValidateCapsulesAcceptsACompleteSet(t *testing.T) {
	visible := validCapsule()
	visible.Visibility = middleware.SidecarVisibilityLLMVisible
	if err := ValidateCapsules([]middleware.SidecarCapsule{validCapsule(), visible}); err != nil {
		t.Fatalf("valid capsules were rejected: %v", err)
	}
	if err := ValidateCapsules(nil); err != nil {
		t.Fatalf("an empty set must be valid: %v", err)
	}
	// The reported index must point at the broken entry, not always the first.
	broken := validCapsule()
	broken.ID = ""
	err := ValidateCapsules([]middleware.SidecarCapsule{validCapsule(), broken})
	if err == nil || !strings.Contains(err.Error(), "sidecar_capsules[1]") {
		t.Fatalf("the error must name the offending index, got %v", err)
	}
}

// TestModelVisibleBlocksOnlyExposesVisibleCapsules is the privacy boundary: a
// trace-only capsule must never be rendered into the model prompt.
func TestModelVisibleBlocksOnlyExposesVisibleCapsules(t *testing.T) {
	traceOnly := validCapsule()
	traceOnly.Content = "secret"
	visible := validCapsule()
	visible.ID = "cap-2"
	visible.Visibility = middleware.SidecarVisibilityLLMVisible
	visible.Content = "public"

	blocks := ModelVisibleBlocks([]middleware.SidecarCapsule{traceOnly, visible})
	joined := strings.Join(blocks, "\n")
	if strings.Contains(joined, "secret") {
		t.Fatalf("a trace-only capsule leaked into the model blocks: %v", blocks)
	}
	if !strings.Contains(joined, "public") {
		t.Fatalf("a model-visible capsule was dropped: %v", blocks)
	}
	ids := CapsuleIDs([]middleware.SidecarCapsule{traceOnly, visible})
	if len(ids) != 2 {
		t.Fatalf("both capsules must be auditable by id, got %v", ids)
	}
}
