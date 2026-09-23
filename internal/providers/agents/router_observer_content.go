package agents

import (
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// acpMessageContribution is what one messageId contributed to the turn. A
// version 2 message upsert replaces this content where a chunk appends to it.
type acpMessageContribution struct {
	text        string
	blocks      []middleware.Content
	final       bool
	finalText   string
	finalBlocks []middleware.Content
}

func (o *simpleObserver) appendMessageChunk(text string, contents []acpContent, metadata map[string]interface{}, messageID string) {
	phase := messagePhase(metadata)
	converted := middlewareContents(contents)
	o.mu.Lock()
	contribution := o.contributionLocked(messageID)
	contribution.text += text
	contribution.blocks = append(contribution.blocks, converted...)
	if phase == "final_answer" {
		contribution.final = true
		contribution.finalText += text
		contribution.finalBlocks = append(contribution.finalBlocks, converted...)
	}
	o.content += text
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

// contributionLocked returns the contribution buffer for a message, remembering
// the order messages were first seen in. Callers hold the lock.
func (o *simpleObserver) contributionLocked(messageID string) *acpMessageContribution {
	if o.messages == nil {
		o.messages = map[string]*acpMessageContribution{}
	}
	contribution, ok := o.messages[messageID]
	if !ok {
		contribution = &acpMessageContribution{}
		o.messages[messageID] = contribution
		o.order = append(o.order, messageID)
	}
	return contribution
}

// applyMessageUpsert applies the version 2 message upsert: concrete content
// replaces everything stored for that messageId, omitted content leaves it
// unchanged, and an explicit null clears it. Later chunks with the same id
// append to whatever the upsert left behind.
func (o *simpleObserver) applyMessageUpsert(notif acpSessionNotification, text string) {
	metadata := streamUpdateMetadata(notif)
	phase := messagePhase(metadata)
	o.mu.Lock()
	contribution := o.contributionLocked(notif.Update.MessageID)
	switch {
	case !notif.Update.ContentSet:
	case notif.Update.ContentCleared:
		*contribution = acpMessageContribution{}
	default:
		*contribution = acpMessageContribution{
			text:   text,
			blocks: middlewareContents(notif.Update.Contents),
			final:  phase == "final_answer",
		}
		if contribution.final {
			contribution.finalText, contribution.finalBlocks = contribution.text, contribution.blocks
		}
	}
	o.rebuildLocked()
	o.mu.Unlock()
	o.forwardThought(middleware.ThoughtTypeThinking, text, notif.Update.Title, metadata)
	o.signalUpdate()
}

// rebuildLocked recomposes the flat accumulators from the per-message
// contributions, in the order the messages first appeared. A peer that never
// sends an upsert never reaches this, so its output is the flat accumulation the
// observer always produced.
func (o *simpleObserver) rebuildLocked() {
	o.content, o.finalContent, o.hasExplicitFinal = "", "", false
	o.blocks, o.finalBlocks = nil, nil
	for _, id := range o.order {
		contribution := o.messages[id]
		o.content += contribution.text
		o.blocks = append(o.blocks, contribution.blocks...)
		o.finalContent += contribution.finalText
		o.finalBlocks = append(o.finalBlocks, contribution.finalBlocks...)
		if contribution.final {
			o.hasExplicitFinal = true
		}
	}
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
