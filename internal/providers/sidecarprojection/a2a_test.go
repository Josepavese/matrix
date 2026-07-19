package sidecarprojection

import (
	"encoding/base64"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestA2AMessagePartsProjectsProtocolNeutralRichContent(t *testing.T) {
	parts := A2AMessageParts(middleware.ConversationTurn{
		Message: "inspect",
		ContentBlocks: []middleware.Content{
			{Type: "image", Data: base64.StdEncoding.EncodeToString([]byte("png")), MimeType: "image/png", Name: "image.png"},
			{Type: "data", Data: `{"priority":"high"}`, MimeType: "application/json"},
			{Type: "resource_link", URI: "https://example.com/report.pdf", MimeType: "application/pdf", Name: "report.pdf"},
		},
	})
	if len(parts) != 4 {
		t.Fatalf("expected text, raw, data and URL parts, got %#v", parts)
	}
	if string(parts[1].Raw()) != "png" || parts[1].Filename != "image.png" || parts[1].MediaType != "image/png" {
		t.Fatalf("image projection lost fields: %#v", parts[1])
	}
	if parts[2].Data() == nil || string(parts[3].URL()) != "https://example.com/report.pdf" {
		t.Fatalf("data or URL projection lost: %#v", parts)
	}
}
