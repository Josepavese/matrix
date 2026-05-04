package agents

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// simpleObserver buffers text chunks to form the final result.
// It also forwards real-time thought/tool updates to an optional ThoughtNotifier
// so the UI (e.g. Telegram) can show a live "thinking" indicator.
type simpleObserver struct {
	mu       sync.Mutex
	content  string
	updates  chan struct{}
	notifier middleware.ThoughtNotifier
	metadata middleware.ConversationMetadata
}

func (o *simpleObserver) OnUpdate(notif acpSessionNotification) {
	log := slog.With("component", "acp_observer", "session", notif.SessionID, "update_type", notif.Update.SessionUpdate)
	text := updateContentText(notif.Update)
	log.Info("session update received", "event", "session_update", "update_type", notif.Update.SessionUpdate, "text_len", len(text), "text_preview", truncate(text, 120))
	o.handleStreamUpdate(log, notif)
	o.mergeUpdateMetadata(notif.Update)
}

func (o *simpleObserver) handleStreamUpdate(log *slog.Logger, notif acpSessionNotification) {
	text := updateContentText(notif.Update)
	switch notif.Update.SessionUpdate {
	case "agent_message_chunk":
		o.appendMessageChunk(text)
	case "agent_thought_chunk":
		o.forwardThought(middleware.ThoughtTypeThinking, text, "", nil)
	case "tool_call", "tool_call_update":
		o.forwardToolUpdate(log, notif)
	case "plan", "available_commands_update", "current_mode_update", "config_option_update", "session_info_update", "usage_update":
		o.forwardThought(middleware.ThoughtTypeThinking, text, notif.Update.Title, structuralUpdateMetadata(notif))
	}
}

func (o *simpleObserver) appendMessageChunk(text string) {
	o.mu.Lock()
	o.content += text
	o.mu.Unlock()
	o.forwardThought(middleware.ThoughtTypeThinking, text, "", nil)
	o.signalUpdate()
}

func (o *simpleObserver) forwardToolUpdate(log *slog.Logger, notif acpSessionNotification) {
	text := updateContentText(notif.Update)
	log.Info("tool call update", "event", "tool_call_update", "text_len", len(text))
	thoughtType := middleware.ThoughtTypeToolCall
	if notif.Update.SessionUpdate == "tool_call_update" {
		thoughtType = middleware.ThoughtTypeToolResult
	}
	o.forwardThought(thoughtType, text, notif.Update.Title, toolUpdateMetadata(notif))
}

func (o *simpleObserver) forwardThought(thoughtType middleware.ThoughtUpdateType, content, title string, metadata map[string]interface{}) {
	if o.notifier == nil || !hasThoughtSignal(content, title, metadata) {
		return
	}
	o.notifier.OnThought(middleware.ThoughtUpdate{
		Type:     thoughtType,
		Content:  content,
		Title:    title,
		Metadata: metadata,
	})
}

func hasThoughtSignal(content, title string, metadata map[string]interface{}) bool {
	return strings.TrimSpace(content) != "" || strings.TrimSpace(title) != "" || len(metadata) > 0
}

func (o *simpleObserver) signalUpdate() {
	if o.updates == nil {
		return
	}
	select {
	case o.updates <- struct{}{}:
	default:
	}
}

func (o *simpleObserver) mergeUpdateMetadata(update acpSessionUpdate) {
	title, updatedAt, metadata := updateMetadataPatch(update)
	if title == "" && updatedAt == "" && len(metadata) == 0 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if title != "" {
		o.metadata.Title = title
	}
	if updatedAt != "" {
		o.metadata.UpdatedAt = updatedAt
	}
	o.mergeMetadataMap(metadata)
}

func updateMetadataPatch(update acpSessionUpdate) (string, string, map[string]interface{}) {
	var metadata map[string]interface{}
	if update.CurrentModeID != "" {
		metadata = addMetadataValue(metadata, "current_mode_id", update.CurrentModeID)
	}
	if len(update.ConfigOptions) > 0 {
		metadata = addMetadataValue(metadata, "config_options", update.ConfigOptions)
	}
	if len(update.AvailableCommands) > 0 {
		metadata = addMetadataValue(metadata, "available_commands", update.AvailableCommands)
	}
	if len(update.Entries) > 0 {
		metadata = addMetadataValue(metadata, "plan_entries", update.Entries)
	}
	if len(update.Usage) > 0 {
		metadata = addMetadataValue(metadata, "usage", update.Usage)
	}
	metadata = addMetadataMap(metadata, update.Meta)
	return update.Title, update.UpdatedAt, metadata
}

func addMetadataValue(metadata map[string]interface{}, key string, value interface{}) map[string]interface{} {
	if metadata == nil {
		metadata = make(map[string]interface{}, 1)
	}
	metadata[key] = value
	return metadata
}

func addMetadataMap(metadata, values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return metadata
	}
	if metadata == nil {
		metadata = make(map[string]interface{}, len(values))
	}
	for key, value := range values {
		metadata[key] = value
	}
	return metadata
}

func (o *simpleObserver) mergeMetadataMap(values map[string]interface{}) {
	if len(values) == 0 {
		return
	}
	if o.metadata.Meta == nil {
		o.metadata.Meta = make(map[string]interface{}, len(values))
	}
	for k, v := range values {
		o.metadata.Meta[k] = v
	}
}

func (o *simpleObserver) GetContent() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return stripThinking(o.content)
}

// RawContent returns the unfiltered content (including think blocks) for debugging.
func (o *simpleObserver) RawContent() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.content
}

func (o *simpleObserver) Metadata() middleware.ConversationMetadata {
	o.mu.Lock()
	defer o.mu.Unlock()
	meta := middleware.ConversationMetadata{
		Title:     o.metadata.Title,
		UpdatedAt: o.metadata.UpdatedAt,
		Status:    o.metadata.Status,
	}
	if len(o.metadata.Meta) > 0 {
		meta.Meta = make(map[string]interface{}, len(o.metadata.Meta))
		for k, v := range o.metadata.Meta {
			meta.Meta[k] = v
		}
	}
	return meta
}

func toolUpdateMetadata(notif acpSessionNotification) map[string]interface{} {
	meta := make(map[string]interface{}, len(notif.Update.Meta)+4)
	text := updateContentText(notif.Update)
	meta["source_update_type"] = notif.Update.SessionUpdate
	meta["content_type"] = notif.Update.Content.Type
	meta["protocol"] = "acp"
	meta["protocol_method"] = "session/update"
	meta["acp"] = map[string]interface{}{
		"session_id":     notif.SessionID,
		"session_update": notif.Update.SessionUpdate,
		"tool_call_id":   notif.Update.ToolCallID,
		"tool_kind":      notif.Update.Kind,
		"status":         notif.Update.Status,
		"raw_input":      notif.Update.RawInput,
		"locations":      notif.Update.Locations,
		"content": map[string]interface{}{
			"type": notif.Update.Content.Type,
			"text": text,
		},
		"content_blocks": notif.Update.Contents,
		"title":          notif.Update.Title,
		"updated_at":     notif.Update.UpdatedAt,
		"_meta":          notif.Update.Meta,
	}
	if strings.TrimSpace(notif.Update.Title) != "" {
		meta["title"] = notif.Update.Title
	}
	if strings.TrimSpace(notif.SessionID) != "" {
		meta["remote_session_id"] = notif.SessionID
	}
	if strings.TrimSpace(notif.Update.ToolCallID) != "" {
		meta["tool_call_id"] = notif.Update.ToolCallID
	}
	if strings.TrimSpace(notif.Update.Kind) != "" {
		meta["tool_kind"] = notif.Update.Kind
		meta["acp_tool_kind"] = notif.Update.Kind
	}
	if strings.TrimSpace(notif.Update.Status) != "" {
		meta["status"] = notif.Update.Status
	}
	if len(notif.Update.RawInput) > 0 {
		meta["raw_input"] = notif.Update.RawInput
	}
	if len(notif.Update.Locations) > 0 {
		meta["locations"] = notif.Update.Locations
	}
	for k, v := range notif.Update.Meta {
		meta[k] = v
	}
	return meta
}

func structuralUpdateMetadata(notif acpSessionNotification) map[string]interface{} {
	meta := map[string]interface{}{
		"source_update_type": notif.Update.SessionUpdate,
		"protocol":           "acp",
		"protocol_method":    "session/update",
		"acp": map[string]interface{}{
			"session_id":         notif.SessionID,
			"session_update":     notif.Update.SessionUpdate,
			"entries":            notif.Update.Entries,
			"available_commands": notif.Update.AvailableCommands,
			"current_mode_id":    notif.Update.CurrentModeID,
			"config_options":     notif.Update.ConfigOptions,
			"usage":              notif.Update.Usage,
			"title":              notif.Update.Title,
			"updated_at":         notif.Update.UpdatedAt,
			"_meta":              notif.Update.Meta,
		},
	}
	for k, v := range notif.Update.Meta {
		meta[k] = v
	}
	return meta
}

func updateContentText(update acpSessionUpdate) string {
	if update.Content.Text != "" {
		return update.Content.Text
	}
	if len(update.Contents) == 0 {
		return ""
	}
	var b strings.Builder
	for _, content := range update.Contents {
		if strings.TrimSpace(content.Text) == "" {
			continue
		}
		b.WriteString(content.Text)
	}
	return b.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// stripThinking removes <think...</think...> blocks from agent output.
// Some agents emit reasoning inside these tags; they should not reach the user.
func stripThinking(s string) string {
	return stripTagBlock(s, "<think", "</think")
}

func stripTagBlock(s, openTag, closeTag string) string {
	for {
		start := strings.Index(s, openTag)
		if start == -1 {
			break
		}
		// Find end of the opening tag (skip attributes like <think xmlns=...>)
		tagEnd := strings.Index(s[start:], ">")
		if tagEnd == -1 {
			break
		}
		end := strings.Index(s[start:], closeTag)
		if end == -1 {
			break
		}
		closeEnd := strings.Index(s[start+end:], ">")
		if closeEnd == -1 {
			break
		}
		s = s[:start] + s[start+end+closeEnd+1:]
	}
	return s
}

// WaitIdle blocks until the stream has been silent for the given duration,
// indicating the agent has finished emitting chunks.
func (o *simpleObserver) WaitIdle(ctx context.Context, idle time.Duration) {
	if o.updates == nil {
		return
	}

	timer := time.NewTimer(idle)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			return
		case <-o.updates:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idle)
		}
	}
}
