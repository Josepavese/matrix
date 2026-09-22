package sidecartrace

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

func capsule(visibility, format, content string) middleware.SidecarCapsule {
	return middleware.SidecarCapsule{
		Provider:   "noema",
		ID:         "cap-1",
		Schema:     "noema/v1",
		Version:    "1.0.0",
		Format:     format,
		Visibility: visibility,
		Content:    content,
		Metadata:   map[string]interface{}{"k": "v"},
	}
}

// TestCarrierFollowsVisibilityAndFormat pins the rule that decides where a
// capsule travels: a trace-only capsule never rides the model conversation.
func TestCarrierFollowsVisibilityAndFormat(t *testing.T) {
	if got := Carrier(capsule(middleware.SidecarVisibilityTraceOnly, middleware.SidecarFormatNoemaXML, "x")); got != "trace" {
		t.Fatalf("a trace-only capsule must be carried by the trace, got %q", got)
	}
	if got := Carrier(capsule(middleware.SidecarVisibilityLLMVisible, middleware.SidecarFormatNoemaXML, "x")); got != middleware.SidecarFormatNoemaXML {
		t.Fatalf("a model-visible capsule must keep its format, got %q", got)
	}
	// A model-visible capsule with no declared format must not be carried as an
	// empty string: the reader needs a usable default.
	if got := Carrier(capsule(middleware.SidecarVisibilityLLMVisible, "", "x")); got != "text" {
		t.Fatalf("an undeclared format must default to text, got %q", got)
	}
}

// TestEventsKeepsInlineContentBehindTheTracePolicy is the privacy invariant: the
// capsule body may only be inlined when the run's policy says so.
func TestEventsKeepsInlineContentBehindTheTracePolicy(t *testing.T) {
	run := runtrace.Run{ID: "run-1", Protocol: string(middleware.ProtocolKindACP)}

	reference := Events(run, []middleware.SidecarCapsule{capsule(middleware.SidecarVisibilityTraceOnly, middleware.SidecarFormatNoemaXML, "secret body")})
	if len(reference) != 1 {
		t.Fatalf("expected one event, got %d", len(reference))
	}
	if reference[0].Message != "" {
		t.Fatalf("content must not be inlined under the default policy, got %q", reference[0].Message)
	}
	if reference[0].ContentDigest == "" {
		t.Fatal("a referenced capsule must still carry a digest")
	}

	inline := run
	inline.TracePolicy.ContentMode = runtrace.ContentModeInline
	inlined := Events(inline, []middleware.SidecarCapsule{capsule(middleware.SidecarVisibilityTraceOnly, middleware.SidecarFormatNoemaXML, "secret body")})
	if inlined[0].Message != "secret body" {
		t.Fatalf("an inline policy must include the body, got %q", inlined[0].Message)
	}
}

// TestEventsAreIdentifiedAndDigestedPerCapsule keeps two capsules in one run
// from collapsing into one event or sharing a digest.
func TestEventsAreIdentifiedAndDigestedPerCapsule(t *testing.T) {
	run := runtrace.Run{ID: "run-2", Protocol: string(middleware.ProtocolKindA2A)}
	first := capsule(middleware.SidecarVisibilityLLMVisible, middleware.SidecarFormatText, "alpha")
	second := capsule(middleware.SidecarVisibilityLLMVisible, middleware.SidecarFormatText, "beta")
	second.ID = "cap-2"

	events := Events(run, []middleware.SidecarCapsule{first, second})
	if len(events) != 2 {
		t.Fatalf("expected two events, got %d", len(events))
	}
	if events[0].ContentRef == events[1].ContentRef {
		t.Fatal("two capsules must not share a content reference")
	}
	if events[0].ContentDigest == events[1].ContentDigest {
		t.Fatal("two different capsule bodies must not share a digest")
	}
	for _, event := range events {
		if event.RunID != "run-2" || event.Kind != "sidecar.capsule.delivered" || event.Status != runtrace.StatusCompleted {
			t.Fatalf("unexpected event envelope: %+v", event)
		}
	}
}

// TestProtocolMetaNamesTheRightCarrierPerProtocol pins the wire contract: A2A
// and ACP carry sidecars differently, and an unknown protocol must not invent a
// field.
func TestProtocolMetaNamesTheRightCarrierPerProtocol(t *testing.T) {
	a2a := Events(runtrace.Run{ID: "r", Protocol: string(middleware.ProtocolKindA2A)}, []middleware.SidecarCapsule{capsule(middleware.SidecarVisibilityLLMVisible, middleware.SidecarFormatText, "x")})[0]
	if _, ok := a2a.ProtocolMeta["a2a"]; !ok {
		t.Fatalf("an A2A run must describe the A2A extension: %v", a2a.ProtocolMeta)
	}
	if _, ok := a2a.ProtocolMeta["acp"]; ok {
		t.Fatal("an A2A run must not describe an ACP carrier")
	}

	acp := Events(runtrace.Run{ID: "r", Protocol: string(middleware.ProtocolKindACP)}, []middleware.SidecarCapsule{capsule(middleware.SidecarVisibilityLLMVisible, middleware.SidecarFormatNoemaXML, "x")})[0]
	acpMeta, ok := acp.ProtocolMeta["acp"].(map[string]interface{})
	if !ok {
		t.Fatalf("an ACP run must describe the ACP carrier: %v", acp.ProtocolMeta)
	}
	if acpMeta["primary_carrier"] != middleware.SidecarFormatNoemaXML {
		t.Fatalf("the ACP carrier must match the capsule format: %v", acpMeta)
	}

	unknown := Events(runtrace.Run{ID: "r", Protocol: "unknown"}, []middleware.SidecarCapsule{capsule(middleware.SidecarVisibilityLLMVisible, middleware.SidecarFormatText, "x")})[0]
	if _, ok := unknown.ProtocolMeta["a2a"]; ok {
		t.Fatal("an unknown protocol must not be described as A2A")
	}
	if _, ok := unknown.ProtocolMeta["acp"]; ok {
		t.Fatal("an unknown protocol must not be described as ACP")
	}
	if unknown.ProtocolMeta["carrier"] != middleware.SidecarFormatText {
		t.Fatal("the carrier must still be reported for an unknown protocol")
	}
}

// TestEventsWithNoCapsulesProduceNothing keeps an empty projection from
// fabricating an event.
func TestEventsWithNoCapsulesProduceNothing(t *testing.T) {
	if events := Events(runtrace.Run{ID: "r"}, nil); len(events) != 0 {
		t.Fatalf("no capsules must project to no events, got %d", len(events))
	}
}

// TestCarrierTreatsUnknownVisibilityAsTraceOnly is the fail-safe direction: a
// capsule whose visibility is unknown must never be promoted onto the model
// conversation.
func TestCarrierTreatsUnknownVisibilityAsTraceOnly(t *testing.T) {
	for _, visibility := range []string{"", "unknown", "public", "llm-visible"} {
		got := Carrier(capsule(visibility, middleware.SidecarFormatNoemaXML, "x"))
		if got != "trace" {
			t.Fatalf("visibility %q must degrade to the trace carrier, got %q", visibility, got)
		}
	}
}
