package a2a

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
)

func a2aInput(parts a2asdk.ContentParts) (string, []middleware.Content, error) {
	lines := make([]string, 0, len(parts))
	blocks := make([]middleware.Content, 0, len(parts))
	for _, part := range parts {
		if part == nil || part.Content == nil {
			continue
		}
		block := middleware.Content{MimeType: part.MediaType, Name: part.Filename, Meta: copyMetadata(part.Metadata)}
		switch value := part.Content.(type) {
		case a2asdk.Text:
			block.Type = "text"
			block.Text = string(value)
			if text := strings.TrimSpace(block.Text); text != "" {
				lines = append(lines, text)
			}
		case a2asdk.Raw:
			block.Type = contentType(part.MediaType)
			block.Data = base64.StdEncoding.EncodeToString([]byte(value))
		case a2asdk.URL:
			block.Type = contentType(part.MediaType)
			block.URI = string(value)
		case a2asdk.Data:
			block.Type = "data"
			encoded, err := json.Marshal(value.Value)
			if err != nil {
				return "", nil, fmt.Errorf("A2A data part is not JSON serializable: %w", err)
			}
			block.Data = string(encoded)
		default:
			return "", nil, fmt.Errorf("unsupported A2A part type %T", part.Content)
		}
		blocks = append(blocks, block)
	}
	return strings.Join(lines, "\n"), blocks, nil
}

func contentType(mediaType string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(mediaType), "image/"):
		return "image"
	case strings.HasPrefix(strings.ToLower(mediaType), "audio/"):
		return "audio"
	default:
		return "file"
	}
}

func copyMetadata(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func hasNonTextContent(blocks []middleware.Content) bool {
	for _, block := range blocks {
		if block.Type != "text" {
			return true
		}
	}
	return false
}
