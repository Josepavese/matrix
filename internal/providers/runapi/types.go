package runapi

import (
	"context"
	"net/http"
	"sync"

	"github.com/jose/matrix-v2/internal/logic/rundelivery"
	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/middleware"
	"github.com/jose/matrix-v2/internal/providers/runsink"
)

const (
	RunPathV1           = "/v1/runs"
	RunResourcePrefixV1 = "/v1/runs/"
	EventSinksPathV1    = "/v1/event-sinks"
)

type Router interface {
	middleware.SessionRouter
}

type Server struct {
	router           Router
	apiKey           string
	defaultAgent     string
	endpointResolver middleware.AgentEndpointResolver
	runStore         *runtrace.Store
	deliveryStore    *rundelivery.Store
	sinkDelivery     *runsink.Service
	runCancels       map[string]context.CancelFunc
	runMu            sync.Mutex
}

type runRequest struct {
	ChannelID            string                 `json:"channel_id"`
	Input                string                 `json:"input"`
	AgentID              string                 `json:"agent_id"`
	WorkspaceID          string                 `json:"workspace_id,omitempty"`
	WorkspacePath        string                 `json:"workspace_path,omitempty"`
	ExecutionMode        string                 `json:"execution_mode,omitempty"`
	EmergencyKillSeconds int                    `json:"emergency_kill_seconds,omitempty"`
	Context              []runtrace.ContextRef  `json:"context,omitempty"`
	ClientMeta           map[string]interface{} `json:"client_meta,omitempty"`
	TracePolicy          runtrace.TracePolicy   `json:"trace_policy,omitempty"`
}

type runResponse struct {
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	Output     string `json:"output,omitempty"`
	TraceURL   string `json:"trace_url"`
	EventsURL  string `json:"events_url"`
	ActionsURL string `json:"actions_url"`
}

type runActionRequest struct {
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

type eventSinkRequest struct {
	URL        string                 `json:"url"`
	EventKinds []string               `json:"event_kinds,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

func NewServer(router Router) *Server {
	storage := runtrace.NewMemoryStorage()
	server := &Server{
		router:       router,
		defaultAgent: "opencode",
		runCancels:   map[string]context.CancelFunc{},
	}
	server.deliveryStore = rundelivery.NewStore(storage)
	return server.withRunStore(runtrace.NewStore(storage))
}

func (s *Server) WithAPIKey(key string) *Server {
	s.apiKey = key
	return s
}

func (s *Server) WithDefaultAgent(agentID string) *Server {
	if agentID != "" {
		s.defaultAgent = agentID
	}
	return s
}

func (s *Server) WithTraceStorage(storage middleware.Storage) *Server {
	if storage != nil {
		s.deliveryStore = rundelivery.NewStore(storage)
		s.withRunStore(runtrace.NewStore(storage))
	}
	return s
}

func (s *Server) WithEndpointResolver(resolver middleware.AgentEndpointResolver) *Server {
	s.endpointResolver = resolver
	return s
}

func (s *Server) Store() *runtrace.Store {
	return s.runStore
}

func (s *Server) DeliveryStore() *rundelivery.Store {
	return s.deliveryStore
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc(RunPathV1, s.HandleRuns)
	mux.HandleFunc(RunResourcePrefixV1, s.HandleRunResource)
	mux.HandleFunc(EventSinksPathV1, s.HandleEventSinks)
}

func (s *Server) withRunStore(store *runtrace.Store) *Server {
	if store != nil {
		s.runStore = store.WithEventDispatcher(s.dispatchRunEvent)
		s.sinkDelivery = runsink.NewService(s.runStore, s.deliveryStore)
	}
	return s
}
