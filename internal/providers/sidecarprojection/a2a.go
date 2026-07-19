package sidecarprojection

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/sidecar"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

func A2AMessageParts(turn middleware.ConversationTurn) []*a2a.Part {
	capsules := sidecar.NormalizeCapsules(turn.SidecarCapsules)
	parts := make([]*a2a.Part, 0, len(turn.ContentBlocks)+len(capsules)*2+1)
	if turn.Message != "" {
		parts = append(parts, a2a.NewTextPart(turn.Message))
	}
	for _, block := range turn.ContentBlocks {
		if part := A2APartFromContent(block); part != nil {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, a2a.NewTextPart(""))
	}
	for _, capsule := range capsules {
		dataPart := a2a.NewDataPart(map[string]any{"sidecar": map[string]any{
			"provider":   capsule.Provider,
			"id":         capsule.ID,
			"schema":     capsule.Schema,
			"version":    capsule.Version,
			"visibility": capsule.Visibility,
			"format":     capsule.Format,
			"metadata":   capsule.Metadata,
		}})
		dataPart.MediaType = sidecar.MediaType(capsule)
		dataPart.Metadata = map[string]any{
			"matrix.sidecar": true,
			"provider":       capsule.Provider,
			"capsule_id":     capsule.ID,
			"schema":         capsule.Schema,
			"visibility":     capsule.Visibility,
		}
		parts = append(parts, dataPart)
		if capsule.Visibility == middleware.SidecarVisibilityLLMVisible && capsule.Content != "" {
			parts = append(parts, a2a.NewTextPart(capsule.Content))
		}
	}
	return parts
}

// A2APartFromContent projects Matrix's protocol-neutral content model onto the
// three stable A2A Part variants: text, file (raw or URL), and structured data.
func A2APartFromContent(content middleware.Content) *a2a.Part {
	part := a2aPartPayload(content)
	if part == nil {
		return nil
	}
	part.Filename = content.Name
	part.MediaType = content.MimeType
	part.Metadata = contentMetadata(content)
	return part
}

func a2aPartPayload(content middleware.Content) *a2a.Part {
	switch strings.ToLower(strings.TrimSpace(content.Type)) {
	case "text":
		return a2a.NewTextPart(content.Text)
	case "image", "audio", "file", "resource_link":
		return filePart(content)
	case "resource":
		return resourcePart(content)
	case "data":
		return a2a.NewDataPart(decodeStructuredData(content.Data))
	default:
		return fallbackPart(content)
	}
}

func resourcePart(content middleware.Content) *a2a.Part {
	if content.URI != "" || content.Data != "" {
		return filePart(content)
	}
	if content.Resource != nil {
		return a2a.NewDataPart(content.Resource)
	}
	return nil
}

func fallbackPart(content middleware.Content) *a2a.Part {
	if content.Text != "" {
		return a2a.NewTextPart(content.Text)
	}
	if content.URI != "" || content.Data != "" {
		return filePart(content)
	}
	if content.Resource != nil {
		return a2a.NewDataPart(content.Resource)
	}
	return nil
}

func filePart(content middleware.Content) *a2a.Part {
	if content.URI != "" {
		return a2a.NewFileURLPart(a2a.URL(content.URI), content.MimeType)
	}
	if content.Data == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(content.Data)
	if err != nil {
		raw = []byte(content.Data)
	}
	return a2a.NewRawPart(raw)
}

func decodeStructuredData(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err == nil {
		return decoded
	}
	return value
}

func contentMetadata(content middleware.Content) map[string]any {
	if len(content.Meta) == 0 && content.Title == "" && content.Description == "" {
		return nil
	}
	metadata := make(map[string]any, len(content.Meta)+2)
	for key, value := range content.Meta {
		metadata[key] = value
	}
	if content.Title != "" {
		metadata["title"] = content.Title
	}
	if content.Description != "" {
		metadata["description"] = content.Description
	}
	return metadata
}

func ApplyA2AMetadata(msg *a2a.Message, capsules []middleware.SidecarCapsule) {
	normalized := sidecar.NormalizeCapsules(capsules)
	if len(normalized) == 0 {
		return
	}
	msg.Extensions = append(msg.Extensions, middleware.SidecarA2AExtensionURI)
	msg.Metadata = map[string]any{"matrix.sidecar": map[string]any{
		"capsule_ids": sidecar.CapsuleIDs(normalized),
		"count":       len(normalized),
	}}
}

func A2ARequestMetadata(capsules []middleware.SidecarCapsule) map[string]any {
	normalized := sidecar.NormalizeCapsules(capsules)
	if len(normalized) == 0 {
		return nil
	}
	return map[string]any{"matrix.sidecar": map[string]any{
		"capsule_ids": sidecar.CapsuleIDs(normalized),
		"extension":   middleware.SidecarA2AExtensionURI,
	}}
}
