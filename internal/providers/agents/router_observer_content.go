package agents

import (
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (o *simpleObserver) appendMessageChunk(text string, contents []acpContent, metadata map[string]interface{}) {
	phase := messagePhase(metadata)
	o.mu.Lock()
	o.content += text
	converted := middlewareContents(contents)
	o.blocks = append(o.blocks, converted...)
	if phase == "final_answer" {
		o.hasExplicitFinal = true
		o.finalContent += text
		o.finalBlocks = append(o.finalBlocks, converted...)
	}
	o.mu.Unlock()
	o.forwardThought(middleware.ThoughtTypeThinking, text, "", metadata)
	o.signalUpdate()
}

func (o *simpleObserver) ContentBlocks() []middleware.Content {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.hasExplicitFinal {
		return append([]middleware.Content(nil), o.finalBlocks...)
	}
	return append([]middleware.Content(nil), o.blocks...)
}

func middlewareContents(contents []acpContent) []middleware.Content {
	converted := make([]middleware.Content, 0, len(contents))
	for _, content := range contents {
		converted = append(converted, middleware.Content{
			Type:        content.Type,
			Text:        content.Text,
			Data:        content.Data,
			MimeType:    content.MimeType,
			URI:         content.URI,
			Name:        content.Name,
			Title:       content.Title,
			Description: content.Description,
			Size:        content.Size,
			Resource:    content.Resource,
			Annotations: content.Annotations,
			Meta:        content.Meta,
		})
	}
	return converted
}

func messagePhase(metadata map[string]interface{}) string {
	phase, _ := metadata["message_phase"].(string)
	return strings.TrimSpace(phase)
}

func acpMessagePhase(meta map[string]interface{}) string {
	if phase, ok := meta["messagePhase"].(string); ok {
		return strings.TrimSpace(phase)
	}
	if phase, ok := meta["message_phase"].(string); ok {
		return strings.TrimSpace(phase)
	}
	codex, _ := meta["codex"].(map[string]interface{})
	phase, _ := codex["phase"].(string)
	return strings.TrimSpace(phase)
}

func messageClassification(phase string) string {
	switch strings.TrimSpace(phase) {
	case "commentary":
		return "progress"
	case "final_answer":
		return "final"
	default:
		return "unclassified"
	}
}

func streamUpdateMetadata(notif acpSessionNotification) map[string]interface{} {
	phase := acpMessagePhase(notif.Update.Meta)
	meta := map[string]interface{}{
		"source_update_type": notif.Update.SessionUpdate,
		"protocol":           "acp",
		"protocol_method":    "session/update",
		"acp": map[string]interface{}{
			"session_id":     notif.SessionID,
			"session_update": notif.Update.SessionUpdate,
			"message_id":     notif.Update.MessageID,
			"content":        notif.Update.Content,
			"content_blocks": notif.Update.Contents,
			"tool_contents":  notif.Update.ToolContents,
			"title":          notif.Update.Title,
			"updated_at":     notif.Update.UpdatedAt,
			"_meta":          notif.Update.Meta,
		},
	}
	if strings.TrimSpace(notif.Update.MessageID) != "" {
		meta["message_id"] = notif.Update.MessageID
	}
	if phase != "" {
		meta["message_phase"] = phase
		meta["message_classification"] = messageClassification(phase)
	} else {
		meta["message_classification"] = "unclassified"
	}
	for k, v := range notif.Update.Meta {
		meta[k] = v
	}
	return meta
}
