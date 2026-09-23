package a2a

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/a2astate"
	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"
)

const (
	a2aMatrixAPIKeyScheme = "matrixApiKey"
	a2aMatrixBearerScheme = "matrixBearer"
)

// Server exposes Matrix as an A2A-compatible JSON-RPC endpoint.
type Server struct {
	router       middleware.ConversationRouter
	baseURL      string
	defaultAgent string
	apiKey       string
	pushStore    push.ConfigStore
	pushSender   push.Sender
	extendedCard *a2asdk.AgentCard
}

// NewServer creates a new A2A server adapter.
func NewServer(router middleware.ConversationRouter, baseURL string, defaultAgent string) *Server {
	if defaultAgent == "" {
		defaultAgent = "opencode"
	}
	return &Server{
		router:       router,
		baseURL:      strings.TrimRight(baseURL, "/"),
		defaultAgent: defaultAgent,
	}
}

type executor struct {
	router       middleware.ConversationRouter
	defaultAgent string
}

type routeConversationRequest struct {
	execCtx       *a2asrv.ExecutorContext
	channelID     string
	agentID       string
	input         string
	contentBlocks []middleware.Content
	notifier      middleware.ThoughtNotifier
}

func (e *executor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2asdk.Event, error] {
	return func(yield func(a2asdk.Event, error) bool) {
		input, contentBlocks, err := a2aInput(execCtx.Message.Parts)
		if err != nil {
			yield(nil, &a2asdk.Error{
				Err:     a2asdk.ErrUnsupportedContentType,
				Message: err.Error(),
			})
			return
		}
		if input == "" && len(contentBlocks) == 0 {
			yield(a2asdk.NewMessage(a2asdk.MessageRoleAgent, a2asdk.NewTextPart("empty message")), nil)
			return
		}

		channelID := messageChannelID(execCtx)
		agentID := messageAgentID(execCtx, e.defaultAgent)
		if execCtx.StoredTask == nil {
			if !yield(a2asdk.NewSubmittedTask(execCtx, execCtx.Message), nil) {
				return
			}
		}
		if !yield(a2asdk.NewStatusUpdateEvent(execCtx, a2asdk.TaskStateWorking, nil), nil) {
			return
		}
		notifier := &a2aThoughtNotifier{execCtx: execCtx, yield: yield}

		output, err := e.routeConversation(ctx, routeConversationRequest{
			execCtx: execCtx, channelID: channelID, agentID: agentID,
			input: input, contentBlocks: contentBlocks, notifier: notifier,
		})
		if err != nil {
			yield(a2asdk.NewStatusUpdateEvent(execCtx, a2asdk.TaskStateFailed, a2asdk.NewMessageForTask(a2asdk.MessageRoleAgent, execCtx, a2asdk.NewTextPart(err.Error()))), nil)
			return
		}

		if output != "" {
			if !yield(a2asdk.NewArtifactEvent(execCtx, a2asdk.NewTextPart(output)), nil) {
				return
			}
		}
		yield(a2asdk.NewStatusUpdateEvent(execCtx, a2asdk.TaskStateCompleted, nil), nil)
	}
}

func (e *executor) routeConversation(ctx context.Context, req routeConversationRequest) (string, error) {
	if richer, ok := e.router.(middleware.ConversationRequestRouter); ok {
		return richer.RouteConversation(ctx, middleware.ConversationRequest{
			ChannelID:                req.channelID,
			AgentID:                  req.agentID,
			Input:                    req.input,
			ContentBlocks:            req.contentBlocks,
			ExtensionURIs:            append([]string(nil), req.execCtx.Message.Extensions...),
			ReferencedRemoteSessions: a2aReferencedRemoteSessions(req.execCtx.Message.ReferenceTasks),
			Notifier:                 req.notifier,
		})
	}
	if hasNonTextContent(req.contentBlocks) {
		return "", fmt.Errorf("matrix router does not expose rich-content ingress")
	}
	return e.router.Route(ctx, req.channelID, req.agentID, req.input, req.notifier)
}

func a2aReferencedRemoteSessions(taskIDs []a2asdk.TaskID) []string {
	if len(taskIDs) == 0 {
		return nil
	}
	out := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if strings.TrimSpace(string(taskID)) == "" {
			continue
		}
		out = append(out, a2astate.Encode(a2astate.State{TaskID: string(taskID)}))
	}
	return out
}

func (e *executor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2asdk.Event, error] {
	return func(yield func(a2asdk.Event, error) bool) {
		yield(a2asdk.NewStatusUpdateEvent(execCtx, a2asdk.TaskStateCanceled, nil), nil)
	}
}

func messageChannelID(execCtx *a2asrv.ExecutorContext) string {
	if execCtx.Message != nil && execCtx.Message.Metadata != nil {
		if raw, ok := execCtx.Message.Metadata["channel_id"].(string); ok && strings.TrimSpace(raw) != "" {
			return raw
		}
	}
	if execCtx.ContextID != "" {
		return "a2a:" + execCtx.ContextID
	}
	return fmt.Sprintf("a2a:%s", execCtx.TaskID)
}

func messageAgentID(execCtx *a2asrv.ExecutorContext, fallback string) string {
	if execCtx.Message != nil && execCtx.Message.Metadata != nil {
		if raw, ok := execCtx.Message.Metadata["agent_id"].(string); ok && strings.TrimSpace(raw) != "" {
			return raw
		}
	}
	return fallback
}

func partsText(parts a2asdk.ContentParts) string {
	text, _, _ := a2aInput(parts)
	return text
}

type a2aThoughtNotifier struct {
	execCtx *a2asrv.ExecutorContext
	yield   func(a2asdk.Event, error) bool
}

func (n *a2aThoughtNotifier) OnThought(update middleware.ThoughtUpdate) {
	message := a2asdk.NewMessageForTask(a2asdk.MessageRoleAgent, n.execCtx, a2asdk.NewTextPart(update.Content))
	message.Metadata = map[string]any{"matrix.thought_type": int(update.Type), "title": update.Title}
	for key, value := range update.Metadata {
		message.Metadata[key] = value
	}
	event := a2asdk.NewStatusUpdateEvent(n.execCtx, a2asdk.TaskStateWorking, message)
	event.Metadata = map[string]any{"matrix.progress": true}
	n.yield(event, nil)
}

func (n *a2aThoughtNotifier) SetHeader(_, _ string) {}

func (n *a2aThoughtNotifier) FormattedHeader() string { return "" }
