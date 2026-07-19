package a2aclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/a2astate"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

type a2aEventProjection struct {
	Text      string
	Blocks    []middleware.Content
	State     a2astate.State
	TaskState a2a.TaskState
	Output    bool
	Final     bool
	Kind      string
	Metadata  map[string]any
}

func (c *a2aConversationClient) streamA2A(ctx context.Context, req *a2a.SendMessageRequest, turn middleware.ConversationTurn) (string, []middleware.Content, middleware.ConversationMetadata, a2astate.State, error) {
	var builder strings.Builder
	var blocks []middleware.Content
	state := a2astate.Decode(turn.RemoteSessionID)
	metadata := middleware.ConversationMetadata{Status: "active", Meta: map[string]interface{}{"protocol": "a2a"}}
	for event, err := range c.client.SendStreamingMessage(ctx, req) {
		if err != nil {
			return "", nil, middleware.ConversationMetadata{}, a2astate.State{}, fmt.Errorf("A2A streaming failed: %w", err)
		}
		projection := projectA2AEvent(event)
		if projection.State.TaskID != "" {
			state = projection.State
			if turn.ThoughtNotifier != nil {
				turn.ThoughtNotifier.SetHeader(turn.AgentID, a2astate.Encode(state))
			}
		}
		mergeA2AMetadata(&metadata, projection)
		if projection.Output {
			builder.WriteString(projection.Text)
			blocks = append(blocks, projection.Blocks...)
		}
		forwardA2AProgress(turn.ThoughtNotifier, projection)
	}
	return strings.TrimSpace(builder.String()), blocks, metadata, state, nil
}

func a2aResultFromSendMessage(resp a2a.SendMessageResult) middleware.ConversationResult {
	projection := projectA2AEvent(resp)
	metadata := middleware.ConversationMetadata{Status: "active", Meta: map[string]interface{}{"protocol": "a2a"}}
	mergeA2AMetadata(&metadata, projection)
	return middleware.ConversationResult{Output: strings.TrimSpace(projection.Text), ContentBlocks: projection.Blocks, RemoteSessionID: a2astate.Encode(projection.State), Metadata: metadata}
}

func projectA2AEvent(event a2a.Event) a2aEventProjection {
	switch value := event.(type) {
	case *a2a.Message:
		return a2aEventProjection{Text: a2aPartsText(value.Parts), Blocks: a2aPartsContent(value.Parts), State: a2astate.State{TaskID: string(value.TaskID), ContextID: value.ContextID}, Output: true, Kind: "message", Metadata: value.Metadata}
	case *a2a.Task:
		return projectA2ATask(value)
	case *a2a.TaskArtifactUpdateEvent:
		return projectA2AArtifact(value)
	case *a2a.TaskStatusUpdateEvent:
		return projectA2AStatus(value)
	default:
		return a2aEventProjection{}
	}
}

func projectA2ATask(task *a2a.Task) a2aEventProjection {
	var texts []string
	var blocks []middleware.Content
	if task.Status.Message != nil {
		texts = append(texts, a2aPartsText(task.Status.Message.Parts))
		blocks = append(blocks, a2aPartsContent(task.Status.Message.Parts)...)
	}
	for _, artifact := range task.Artifacts {
		texts = append(texts, a2aPartsText(artifact.Parts))
		blocks = append(blocks, a2aPartsContent(artifact.Parts)...)
	}
	final := a2aTaskStateFinal(task.Status.State)
	return a2aEventProjection{Text: strings.TrimSpace(strings.Join(texts, "\n")), Blocks: blocks, State: a2astate.State{TaskID: string(task.ID), ContextID: task.ContextID}, TaskState: task.Status.State, Output: final || len(task.Artifacts) > 0, Final: final, Kind: "task", Metadata: task.Metadata}
}

func projectA2AArtifact(event *a2a.TaskArtifactUpdateEvent) a2aEventProjection {
	metadata := cloneA2AMetadata(event.Metadata)
	state := a2astate.State{TaskID: string(event.TaskID), ContextID: event.ContextID}
	if event.Artifact == nil {
		return a2aEventProjection{State: state, Kind: "artifact_update", Metadata: metadata}
	}
	metadata["artifact_id"] = string(event.Artifact.ID)
	metadata["artifact_name"] = event.Artifact.Name
	metadata["artifact_description"] = event.Artifact.Description
	return a2aEventProjection{Text: a2aPartsText(event.Artifact.Parts), Blocks: a2aPartsContent(event.Artifact.Parts), State: state, Output: true, Kind: "artifact_update", Metadata: metadata}
}

func projectA2AStatus(event *a2a.TaskStatusUpdateEvent) a2aEventProjection {
	projection := a2aEventProjection{State: a2astate.State{TaskID: string(event.TaskID), ContextID: event.ContextID}, TaskState: event.Status.State, Final: a2aTaskStateFinal(event.Status.State), Kind: "status_update", Metadata: event.Metadata}
	if event.Status.Message != nil {
		projection.Text = a2aPartsText(event.Status.Message.Parts)
		projection.Blocks = a2aPartsContent(event.Status.Message.Parts)
		projection.Output = projection.Final
	}
	return projection
}

func a2aPartsText(parts a2a.ContentParts) string {
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if text := strings.TrimSpace(part.Text()); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func a2aPartsContent(parts a2a.ContentParts) []middleware.Content {
	out := make([]middleware.Content, 0, len(parts))
	for _, part := range parts {
		if content, ok := a2aPartContent(part); ok {
			out = append(out, content)
		}
	}
	return out
}

func a2aPartContent(part *a2a.Part) (middleware.Content, bool) {
	if part == nil || part.Content == nil {
		return middleware.Content{}, false
	}
	content := middleware.Content{MimeType: part.MediaType, Name: part.Filename, Meta: cloneA2AMetadata(part.Metadata)}
	switch value := part.Content.(type) {
	case a2a.Text:
		content.Type, content.Text = "text", string(value)
	case a2a.Raw:
		content.Type, content.Data = contentTypeForMedia(part.MediaType), base64.StdEncoding.EncodeToString([]byte(value))
	case a2a.URL:
		content.Type, content.URI = contentTypeForMedia(part.MediaType), string(value)
	case a2a.Data:
		content.Type = "data"
		encoded, err := json.Marshal(value.Value)
		if err != nil {
			return middleware.Content{}, false
		}
		content.Data = string(encoded)
	default:
		return middleware.Content{}, false
	}
	return content, true
}

func contentTypeForMedia(mediaType string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(mediaType), "image/"):
		return "image"
	case strings.HasPrefix(strings.ToLower(mediaType), "audio/"):
		return "audio"
	default:
		return "file"
	}
}

func cloneA2AMetadata(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func a2aTaskStateFinal(state a2a.TaskState) bool {
	switch state {
	case a2a.TaskStateCompleted, a2a.TaskStateCanceled, a2a.TaskStateFailed, a2a.TaskStateRejected:
		return true
	default:
		return false
	}
}

func mergeA2AMetadata(metadata *middleware.ConversationMetadata, projection a2aEventProjection) {
	if projection.TaskState != "" {
		metadata.Status = string(projection.TaskState)
		metadata.Meta["task_state"] = string(projection.TaskState)
	}
	if projection.Kind != "" {
		metadata.Meta["event_kind"] = projection.Kind
	}
	if projection.State.TaskID != "" {
		metadata.Meta["task_id"] = projection.State.TaskID
	}
	if projection.State.ContextID != "" {
		metadata.Meta["context_id"] = projection.State.ContextID
	}
	if projection.Final {
		metadata.Meta["final"] = true
	}
	for key, value := range projection.Metadata {
		metadata.Meta[key] = value
	}
}

func forwardA2AProgress(notifier middleware.ThoughtNotifier, projection a2aEventProjection) {
	if notifier == nil || projection.Output || (projection.Text == "" && projection.TaskState == "") {
		return
	}
	metadata := cloneA2AMetadata(projection.Metadata)
	metadata["protocol"] = "a2a"
	metadata["event_kind"] = projection.Kind
	if projection.TaskState != "" {
		metadata["task_state"] = string(projection.TaskState)
	}
	notifier.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeThinking, Content: projection.Text, Metadata: metadata})
}
