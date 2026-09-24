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
	mu               sync.Mutex
	content          string
	finalContent     string
	hasExplicitFinal bool
	blocks           []middleware.Content
	finalBlocks      []middleware.Content
	updates          chan struct{}
	notifier         middleware.ThoughtNotifier
	metadata         middleware.ConversationMetadata
	// messages holds what each messageId contributed. A version 2 message upsert
	// replaces that message's content where a chunk appends to it, so the flat
	// accumulators above are rebuilt from these contributions the first time an
	// upsert arrives. Until then they stay authoritative, which keeps a version 1
	// peer's output exactly what it was.
	messages map[string]*acpMessageContribution
	order    []string
	// terminal and stopReason record the version 2 terminal state update, which
	// is what ends a turn that the prompt response only acknowledged.
	terminal   bool
	stopReason string
}

func (o *simpleObserver) OnUpdate(notif acpSessionNotification) {
	log := slog.With("component", "acp_observer", "session", notif.SessionID, "update_type", notif.Update.SessionUpdate)
	text := updateContentText(notif.Update)
	log.Info("session update received", "event", "session_update", "update_type", notif.Update.SessionUpdate, "text_len", len(text))
	if reason, terminal := notif.Update.TurnTerminal(); terminal {
		o.mu.Lock()
		o.stopReason, o.terminal = reason, true
		o.mu.Unlock()
		o.signalUpdate()
	}
	o.handleStreamUpdate(log, notif)
	o.mergeUpdateMetadata(notif.Update)
}

// AwaitTerminal waits for the version 2 terminal state update, the budget, or
// the turn context, and records in the turn metadata how the turn ended: a peer
// that never reports idle has to be visible to the operator instead of looking
// like a complete answer. It reports whether the terminal state arrived.
func (o *simpleObserver) AwaitTerminal(ctx context.Context, budget time.Duration) bool {
	deadline := time.NewTimer(budget)
	defer deadline.Stop()
	for {
		if reason, terminal := o.terminalState(); terminal {
			o.recordCompletion("idle", reason, budget)
			return true
		}
		select {
		case <-ctx.Done():
			o.recordCompletion("context_done", "", budget)
			return false
		case <-deadline.C:
			o.recordCompletion("timeout", "", budget)
			return false
		case <-o.updates:
		}
	}
}

func (o *simpleObserver) terminalState() (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.stopReason, o.terminal
}

func (o *simpleObserver) recordCompletion(outcome, reason string, budget time.Duration) {
	meta := map[string]interface{}{
		"acp_turn_completion": outcome,
		"acp_turn_budget_ms":  budget.Milliseconds(),
	}
	if reason != "" {
		meta["acp_stop_reason"] = reason
	}
	o.mergeMetadataMap(meta)
}

func (o *simpleObserver) handleStreamUpdate(log *slog.Logger, notif acpSessionNotification) {
	text := updateContentText(notif.Update)
	switch notif.Update.SessionUpdate {
	case "agent_message_chunk":
		o.appendMessageChunk(text, notif.Update.Contents, streamUpdateMetadata(notif), notif.Update.MessageID)
	case "agent_message":
		o.applyMessageUpsert(notif, text)
	case "agent_thought_chunk":
		o.forwardThought(middleware.ThoughtTypeThinking, text, "", streamUpdateMetadata(notif))
	case "agent_thought":
		// A version 2 thought upsert patches the stored thought the same way an
		// agent_message upsert patches a message. Matrix keeps no stored thought
		// to patch: reasoning is forwarded live and never becomes turn content,
		// so the update is projected onto the live surface and nothing else.
		o.forwardThought(middleware.ThoughtTypeThinking, text, notif.Update.Title, streamUpdateMetadata(notif))
	case "tool_call", "tool_call_update", "tool_call_content_chunk":
		o.forwardToolUpdate(log, notif)
	default:
		o.handleSessionScopedUpdate(notif)
	}
}

// handleSessionScopedUpdate projects the updates that describe the session or
// its surroundings rather than its answer: plans and commands, configuration and
// usage, and the agent-owned terminals.
func (o *simpleObserver) handleSessionScopedUpdate(notif acpSessionNotification) {
	text := updateContentText(notif.Update)
	switch notif.Update.SessionUpdate {
	case "plan", "plan_update", "available_commands_update", "current_mode_update", "config_option_update", "session_info_update", "usage_update":
		o.forwardThought(middleware.ThoughtTypeThinking, text, notif.Update.Title, structuralUpdateMetadata(notif))
	case "terminal_update", "terminal_output_chunk":
		// Agent-owned terminals are display-only: Matrix never created them and
		// cannot attach to or read them, and their bytes are not part of the
		// turn's answer. They are projected onto the structured surface so the
		// terminal, its command, its exit status and its output snapshot stay
		// visible to the operator instead of vanishing.
		o.forwardThought(middleware.ThoughtTypeToolResult, terminalSummary(notif.Update), notif.Update.TerminalID, terminalUpdateMetadata(notif))
	}
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
	if o.hasExplicitFinal {
		return stripThinking(o.finalContent)
	}
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
