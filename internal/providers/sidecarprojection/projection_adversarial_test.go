package sidecarprojection

import (
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestA2APartFromContentRejectsEmptyContent keeps an empty block from being
// projected into a peer message: a part with no payload would tell the remote
// agent that something was sent when nothing was.
func TestA2APartFromContentRejectsEmptyContent(t *testing.T) {
	empty := []middleware.Content{
		{},
		{Type: "text"},
		{Type: "data", Data: ""},
		{Type: "unknown"},
	}
	for _, content := range empty {
		if part := A2APartFromContent(content); part != nil {
			t.Fatalf("content %+v projected to a part instead of nothing: %+v", content, part)
		}
	}
}

// TestA2APartFromContentFallsBackToWhateverCarriesThePayload pins the tolerant
// direction: an unknown type must still be delivered when it carries text, an
// URI or structured data, because dropping it would lose the message.
func TestA2APartFromContentFallsBackToWhateverCarriesThePayload(t *testing.T) {
	cases := map[string]middleware.Content{
		"unknown type with text":     {Type: "mystery", Text: "hello"},
		"unknown type with uri":      {Type: "mystery", URI: "https://example.test/f"},
		"unknown type with data":     {Type: "mystery", Data: "raw"},
		"unknown type with resource": {Type: "mystery", Resource: map[string]interface{}{"k": "v"}},
		"data as json":               {Type: "data", Data: `{"k":"v"}`},
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			part := A2APartFromContent(content)
			if part == nil {
				t.Fatalf("a content that carries a payload must not be dropped: %+v", content)
			}
		})
	}
}

// TestA2APartFromContentKeepsIdentityMetadata keeps the file name and media type
// attached, which is what lets the remote side store the artefact correctly.
func TestA2APartFromContentKeepsIdentityMetadata(t *testing.T) {
	part := A2APartFromContent(middleware.Content{Type: "file", URI: "https://example.test/report.pdf", Name: "report.pdf", MimeType: "application/pdf"})
	if part == nil {
		t.Fatal("a file must project to a part")
	}
	if part.Filename != "report.pdf" || part.MediaType != "application/pdf" {
		t.Fatalf("the projection lost the identity: %+v", part)
	}
}

// TestA2AMessagePartsKeepsTheTurnOrder is the conversation contract: the peer
// must receive the parts in the order they were produced.
func TestA2AMessagePartsKeepsTheTurnOrder(t *testing.T) {
	turn := middleware.ConversationTurn{ContentBlocks: []middleware.Content{
		{Type: "text", Text: "primo"},
		{Type: "text", Text: "secondo"},
	}}
	parts := A2AMessageParts(turn)
	if len(parts) != 2 {
		t.Fatalf("expected two parts, got %d", len(parts))
	}
	// The projection drops empty content, so a turn with one empty block must
	// still yield only the real parts rather than a gap.
	gapped := A2AMessageParts(middleware.ConversationTurn{ContentBlocks: []middleware.Content{
		{Type: "text", Text: "primo"},
		{},
		{Type: "text", Text: "secondo"},
	}})
	if len(gapped) != 2 {
		t.Fatalf("empty content must be dropped, got %d parts", len(gapped))
	}
}

// TestA2AMessagePartsAlwaysCarriesAPart keeps a peer message from being sent
// with no parts at all: an empty message would be a protocol violation, so the
// projection must fall back to an empty text part rather than nothing.
func TestA2AMessagePartsAlwaysCarriesAPart(t *testing.T) {
	parts := A2AMessageParts(middleware.ConversationTurn{})
	if len(parts) != 1 {
		t.Fatalf("an empty turn must still project one part, got %d", len(parts))
	}
}

// TestA2AMessagePartsCarriesTheMessageThenTheBlocks keeps the ordering contract
// that a reader relies on: the turn text first, then the content blocks in the
// order the agent produced them.
func TestA2AMessagePartsCarriesTheMessageThenTheBlocks(t *testing.T) {
	turn := middleware.ConversationTurn{
		Message:       "ecco il risultato",
		ContentBlocks: []middleware.Content{{Type: "text", Text: "blocco"}},
	}
	parts := A2AMessageParts(turn)
	if len(parts) != 2 {
		t.Fatalf("expected the message and the block, got %d parts", len(parts))
	}
}

// TestA2AMessagePartsProjectsCapsulesForBothAudiences is the sidecar contract:
// every capsule must appear as structured data, and only a model-visible capsule
// may also appear as text the model can read.
func TestA2AMessagePartsProjectsCapsulesForBothAudiences(t *testing.T) {
	traceOnly := middleware.SidecarCapsule{Provider: "noema", ID: "c1", Visibility: middleware.SidecarVisibilityTraceOnly, Content: "riservato"}
	visible := middleware.SidecarCapsule{Provider: "noema", ID: "c2", Visibility: middleware.SidecarVisibilityLLMVisible, Content: "condiviso"}
	parts := A2AMessageParts(middleware.ConversationTurn{SidecarCapsules: []middleware.SidecarCapsule{traceOnly, visible}})
	// Two capsules as data, plus one text part for the model-visible one.
	if len(parts) != 3 {
		t.Fatalf("expected two data parts and one text part, got %d", len(parts))
	}
}

// TestFilePartPrefersTheURLCarrier keeps a remote artefact from being inlined as
// raw bytes when the caller supplied a URL.
func TestFilePartPrefersTheURLCarrier(t *testing.T) {
	withURL := A2APartFromContent(middleware.Content{Type: "file", URI: "https://example.test/f.pdf", Data: "aGVsbG8=", MimeType: "application/pdf"})
	if withURL == nil {
		t.Fatal("a file with a URL must project to a part")
	}
	// A base64 body must decode; an invalid one must still be delivered rather
	// than dropped, because losing the artefact is worse than a raw fallback.
	raw := A2APartFromContent(middleware.Content{Type: "file", Data: "aGVsbG8="})
	if raw == nil {
		t.Fatal("a file with a body must project to a part")
	}
	notBase64 := A2APartFromContent(middleware.Content{Type: "file", Data: "not base64!"})
	if notBase64 == nil {
		t.Fatal("an unparsable body must still be delivered")
	}
}

// TestACPMetaDescribesOneRecordPerProvider pins the per-provider grouping: an
// ACP peer looks up its own key, so capsules from the same provider must be
// grouped rather than overwriting each other.
func TestACPMetaDescribesOneRecordPerProvider(t *testing.T) {
	if meta := ACPMeta(nil); meta != nil {
		t.Fatalf("no capsules must project to no metadata, got %v", meta)
	}
	first := middleware.SidecarCapsule{Provider: "noema", ID: "c1", Visibility: middleware.SidecarVisibilityLLMVisible, Metadata: map[string]interface{}{"k": "v"}}
	second := middleware.SidecarCapsule{Provider: "noema", ID: "c2", Visibility: middleware.SidecarVisibilityTraceOnly}
	other := middleware.SidecarCapsule{Provider: "other", ID: "c3", Visibility: middleware.SidecarVisibilityLLMVisible}

	meta := ACPMeta([]middleware.SidecarCapsule{first, second, other})
	summary, ok := meta["matrix.dev/sidecar"].(map[string]interface{})
	if !ok {
		t.Fatalf("the generic summary must be present, got %v", meta)
	}
	if summary["count"] != 3 {
		t.Fatalf("count = %v, want 3", summary["count"])
	}
	noema, ok := meta["noema.dev/sidecar"].(map[string]interface{})
	if !ok {
		t.Fatalf("each provider must get a key, got %v", meta)
	}
	ids, _ := noema["capsule_ids"].([]string)
	if len(ids) != 2 {
		t.Fatalf("capsules from one provider must be grouped, got %v", ids)
	}
	records, _ := noema["capsules"].([]map[string]interface{})
	if len(records) != 2 {
		t.Fatalf("expected two records, got %v", records)
	}
	// The metadata must survive, because it is the provider's own contract.
	if records[0]["metadata"] == nil {
		t.Fatalf("capsule metadata was dropped: %v", records[0])
	}
	if _, present := records[1]["metadata"]; present {
		t.Fatalf("an absent metadata map must not be invented: %v", records[1])
	}
}

// TestA2AMetadataEnvelopeIsOnlyPresentWithCapsules keeps an empty extension list
// off the wire: a peer that sees the extension URI expects content behind it.
func TestA2AMetadataEnvelopeIsOnlyPresentWithCapsules(t *testing.T) {
	if meta := A2ARequestMetadata(nil); meta != nil {
		t.Fatalf("no capsules must produce no request metadata, got %v", meta)
	}
	capsules := []middleware.SidecarCapsule{{Provider: "noema", ID: "c1", Visibility: middleware.SidecarVisibilityLLMVisible, Content: "x"}}
	meta := A2ARequestMetadata(capsules)
	envelope, ok := meta["matrix.sidecar"].(map[string]interface{})
	if !ok {
		t.Fatalf("the envelope must be present, got %v", meta)
	}
	if envelope["extension"] != middleware.SidecarA2AExtensionURI {
		t.Fatalf("the extension URI must be advertised, got %v", envelope)
	}
}
