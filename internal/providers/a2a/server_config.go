package a2a

import (
	"context"
	"mime"
	"net/http"
	"strings"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"
)

func (s *Server) WithAPIKey(key string) *Server {
	s.apiKey = strings.TrimSpace(key)
	return s
}

func (s *Server) WithPushNotifications(store push.ConfigStore, sender push.Sender) *Server {
	s.pushStore = store
	s.pushSender = sender
	return s
}

func (s *Server) WithExtendedAgentCard(card *a2asdk.AgentCard) *Server {
	s.extendedCard = card
	return s
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	capabilities := s.capabilities()
	options := []a2asrv.RequestHandlerOption{
		a2asrv.WithCapabilityChecks(&capabilities),
		a2asrv.WithCallInterceptors(&matrixAuthenticatedUserInterceptor{userName: s.taskOwner()}),
	}
	if s.pushStore != nil && s.pushSender != nil {
		options = append(options, a2asrv.WithPushNotifications(s.pushStore, s.pushSender))
	}
	if s.extendedCard != nil {
		options = append(options, a2asrv.WithExtendedAgentCard(s.extendedCard))
	}
	handler := a2asrv.NewHandler(&executor{router: s.router, defaultAgent: s.defaultAgent}, options...)
	mux.Handle("/a2a", s.authMiddleware(a2asrv.NewJSONRPCHandler(handler), true))
	mux.Handle("/a2a/rest/", http.StripPrefix("/a2a/rest", s.authMiddleware(a2asrv.NewRESTHandler(handler), false)))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(s.agentCard()))
}

func (s *Server) taskOwner() string {
	if s.apiKey != "" {
		return "matrix-api-key"
	}
	return "matrix-local"
}

type matrixAuthenticatedUserInterceptor struct {
	a2asrv.PassthroughCallInterceptor
	userName string
}

func (i *matrixAuthenticatedUserInterceptor) Before(ctx context.Context, callCtx *a2asrv.CallContext, _ *a2asrv.Request) (context.Context, any, error) {
	callCtx.User = a2asrv.NewAuthenticatedUser(i.userName, nil)
	return ctx, nil, nil
}

func (s *Server) authMiddleware(next http.Handler, requireJSON bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requireJSON && !requireJSONContentType(w, r) {
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
	return a2asdk.AgentCapabilities{Streaming: true, PushNotifications: s.pushStore != nil && s.pushSender != nil, ExtendedAgentCard: s.extendedCard != nil}
}

func (s *Server) agentCard() *a2asdk.AgentCard {
	card := &a2asdk.AgentCard{
		Name:               "Matrix",
		Description:        "Protocol-neutral local-first orchestration runtime",
		Version:            "2",
		Capabilities:       s.capabilities(),
		DefaultInputModes:  []string{"text/plain", "application/json", "application/octet-stream", "image/*", "audio/*"},
		DefaultOutputModes: []string{"text/plain"},
		Skills: []a2asdk.AgentSkill{{
			ID: "route-message", Name: "Route Message",
			Description: "Routes a message into the Matrix session runtime",
			Tags:        []string{"orchestration", "runtime", "chat"},
		}},
		SupportedInterfaces: []*a2asdk.AgentInterface{
			a2asdk.NewAgentInterface(s.baseURL+"/a2a", a2asdk.TransportProtocolJSONRPC),
			a2asdk.NewAgentInterface(s.baseURL+"/a2a/rest", a2asdk.TransportProtocolHTTPJSON),
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
		a2aMatrixAPIKeyScheme: a2asdk.APIKeySecurityScheme{Description: "Matrix local A2A API key", Location: a2asdk.APIKeySecuritySchemeLocationHeader, Name: "X-Matrix-Key"},
		a2aMatrixBearerScheme: a2asdk.HTTPAuthSecurityScheme{Description: "Matrix local A2A bearer token", Scheme: "Bearer"},
	}
	card.SecurityRequirements = a2asdk.SecurityRequirementsOptions{
		a2asdk.SecurityRequirements{a2aMatrixAPIKeyScheme: {}},
		a2asdk.SecurityRequirements{a2aMatrixBearerScheme: {}},
	}
}
