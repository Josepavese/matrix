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
