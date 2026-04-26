package agents

import (
	"encoding/json"

	"github.com/Josepavese/matrix/internal/logic/frontendevents"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

func (h *defaultRequestHandler) beginClientTool(signal frontendevents.ToolSignal, params json.RawMessage) frontendevents.ToolSignal {
	signal = withRawInput(signal, frontendevents.SanitizedRawInput(params))
	h.notifyToolRequested(signal.Content, signal.Metadata)
	return signal
}

func (h *defaultRequestHandler) completeClientTool(signal frontendevents.ToolSignal) {
	h.notifyToolCompleted(signal.Content, signal.Metadata)
}

func (h *defaultRequestHandler) failClientTool(signal frontendevents.ToolSignal) {
	h.notifyToolFailed(signal.Content, signal.Metadata)
}

func (h *defaultRequestHandler) notifyToolRequested(content string, metadata map[string]interface{}) {
	h.notifyTool(middleware.ThoughtTypeToolCall, content, "pending", metadata)
}

func (h *defaultRequestHandler) notifyToolCompleted(content string, metadata map[string]interface{}) {
	h.notifyTool(middleware.ThoughtTypeToolResult, content, runtrace.StatusCompleted, metadata)
}

func (h *defaultRequestHandler) notifyToolFailed(content string, metadata map[string]interface{}) {
	h.notifyTool(middleware.ThoughtTypeToolResult, content, runtrace.StatusFailed, metadata)
}

func (h *defaultRequestHandler) notifyTool(updateType middleware.ThoughtUpdateType, content, status string, metadata map[string]interface{}) {
	h.notifierMu.Lock()
	notifier := h.notifier
	h.notifierMu.Unlock()
	if notifier == nil {
		return
	}
	metadata = cloneMetadata(metadata)
	metadata["status"] = status
	metadata["protocol"] = "acp"
	metadata["source_update_type"] = "client_tool_request"
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:     updateType,
		Content:  content,
		Metadata: metadata,
	})
}

func cloneMetadata(metadata map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(metadata)+4)
	for k, v := range metadata {
		out[k] = v
	}
	return out
}

func withRawInput(signal frontendevents.ToolSignal, rawInput map[string]interface{}) frontendevents.ToolSignal {
	if len(rawInput) == 0 {
		return signal
	}
	signal.Metadata = cloneMetadata(signal.Metadata)
	signal.Metadata["raw_input"] = rawInput
	return signal
}
