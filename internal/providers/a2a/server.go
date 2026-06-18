package a2a

import (
	"context"
	"fmt"
	"iter"
	"mime"
	"net/http"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
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

func (s *Server) WithAPIKey(key string) *Server {
	s.apiKey = strings.TrimSpace(key)
	return s
}

// RegisterRoutes attaches the A2A JSON-RPC endpoint and agent card to the mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	capabilities := s.capabilities()
	handler := a2asrv.NewHandler(&executor{
		router:       s.router,
		defaultAgent: s.defaultAgent,
	}, a2asrv.WithCapabilityChecks(&capabilities))
	mux.Handle("/a2a", s.authMiddleware(a2asrv.NewJSONRPCHandler(handler)))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(s.agentCard()))
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireJSONContentType(w, r) {
			return
		}
		if s.apiKey != "" && requestAPIKey(r) != s.apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestAPIKey(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get("X-Matrix-Key")); key != "" {
		return key
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("bearer "):])
	}
	return ""
}

func requireJSONContentType(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !isA2AJSONMediaType(mediaType) {
		http.Error(w, "Unsupported Media Type: application/json or application/a2a+json required", http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

func isA2AJSONMediaType(mediaType string) bool {
	switch strings.ToLower(mediaType) {
	case "application/json", "application/a2a+json":
		return true
	default:
		return false
	}
}

func (s *Server) capabilities() a2asdk.AgentCapabilities {
	return a2asdk.AgentCapabilities{
		Streaming:         false,
		PushNotifications: false,
		ExtendedAgentCard: false,
	}
}

func (s *Server) agentCard() *a2asdk.AgentCard {
	card := &a2asdk.AgentCard{
		Name:               "Matrix",
		Description:        "Protocol-neutral local-first orchestration runtime",
		Version:            "2",
		Capabilities:       s.capabilities(),
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Skills: []a2asdk.AgentSkill{
			{
				ID:          "route-message",
				Name:        "Route Message",
				Description: "Routes a text prompt into the Matrix session runtime",
				Tags:        []string{"orchestration", "runtime", "chat"},
			},
		},
		SupportedInterfaces: []*a2asdk.AgentInterface{
			a2asdk.NewAgentInterface(s.baseURL+"/a2a", a2asdk.TransportProtocolJSONRPC),
		},
	}
	s.applySecurity(card)
	return card
}

func (s *Server) applySecurity(card *a2asdk.AgentCard) {
	if s.apiKey == "" {
		return
	}
	card.SecuritySchemes = a2asdk.NamedSecuritySchemes{
		a2aMatrixAPIKeyScheme: a2asdk.APIKeySecurityScheme{
			Description: "Matrix local A2A API key",
			Location:    a2asdk.APIKeySecuritySchemeLocationHeader,
			Name:        "X-Matrix-Key",
		},
		a2aMatrixBearerScheme: a2asdk.HTTPAuthSecurityScheme{
			Description: "Matrix local A2A bearer token",
			Scheme:      "Bearer",
		},
	}
	card.SecurityRequirements = a2asdk.SecurityRequirementsOptions{
		a2asdk.SecurityRequirements{a2aMatrixAPIKeyScheme: {}},
		a2asdk.SecurityRequirements{a2aMatrixBearerScheme: {}},
	}
}

type executor struct {
	router       middleware.ConversationRouter
	defaultAgent string
}

func (e *executor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2asdk.Event, error] {
	return func(yield func(a2asdk.Event, error) bool) {
		input, unsupported := textInput(execCtx.Message.Parts)
		if unsupported {
			yield(nil, &a2asdk.Error{
				Err:     a2asdk.ErrUnsupportedContentType,
				Message: "Matrix A2A server currently accepts text/plain input only",
			})
			return
		}
		if input == "" {
			yield(a2asdk.NewMessage(a2asdk.MessageRoleAgent, a2asdk.NewTextPart("empty message")), nil)
			return
		}

		channelID := messageChannelID(execCtx)
		agentID := messageAgentID(execCtx, e.defaultAgent)

		output, err := e.router.Route(ctx, channelID, agentID, input, nil)
		if err != nil {
			yield(a2asdk.NewStatusUpdateEvent(execCtx, a2asdk.TaskStateFailed, a2asdk.NewMessageForTask(a2asdk.MessageRoleAgent, execCtx, a2asdk.NewTextPart(err.Error()))), nil)
			return
		}

		yield(a2asdk.NewMessageForTask(a2asdk.MessageRoleAgent, execCtx, a2asdk.NewTextPart(output)), nil)
	}
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
	text, _ := textInput(parts)
	return text
}

func textInput(parts a2asdk.ContentParts) (string, bool) {
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == nil {
			continue
		}
		if _, ok := part.Content.(a2asdk.Text); !ok {
			if part.Content != nil {
				return "", true
			}
			continue
		}
		if text := strings.TrimSpace(part.Text()); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.Join(lines, "\n"), false
}
