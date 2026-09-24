package runapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/rundelivery"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/workspacegrant"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/runpayload"
	"github.com/Josepavese/matrix/internal/providers/runsink"
)

const (
	RunPathV1             = "/v1/runs"
	RunResourcePrefixV1   = "/v1/runs/"
	EventSinksPathV1      = "/v1/event-sinks"
	ElicitationPathV1     = "/v1/elicitations"
	AgentAuthPathV1       = "/v1/agent-auth"
	AgentAuthLogoutPathV1 = "/v1/agent-auth/logout"
	WorkspaceGrantsPathV1 = "/v1/workspace-grants"
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
	workspaceGrants  *workspacegrant.Store
	deliveryStore    *rundelivery.Store
	sinkDelivery     *runsink.Service
	elicitations     *elicitationsHandler
	agentAuth        *agentAuthHandler
	runCancels       map[string]context.CancelFunc
	runMu            sync.Mutex
	idempotencyMu    sync.Mutex
}

type runRequest struct {
	ChannelID              string                      `json:"channel_id"`
	Input                  runpayload.Input            `json:"input"`
	AgentID                string                      `json:"agent_id"`
	ModelID                string                      `json:"model_id,omitempty"`
	FallbackModelID        string                      `json:"fallback_model_id,omitempty"`
	AgentConfig            runAgentConfig              `json:"agent_config,omitempty"`
	CodexConfig            runAgentConfig              `json:"codex_config,omitempty"`
	WorkspaceID            string                      `json:"workspace_id,omitempty"`
	WorkspacePath          string                      `json:"workspace_path,omitempty"`
	WorkspacePolicy        string                      `json:"workspace_policy,omitempty"`
	ExecutionMode          string                      `json:"execution_mode,omitempty"`
	SessionPolicy          string                      `json:"session_policy,omitempty"`
	CleanupPolicy          string                      `json:"cleanup_policy,omitempty"`
	EmergencyKillSeconds   int                         `json:"emergency_kill_seconds,omitempty"`
	ActivityTimeoutSeconds int                         `json:"activity_timeout_seconds,omitempty"`
	Context                []runtrace.ContextRef       `json:"context,omitempty"`
	SidecarCapsules        []middleware.SidecarCapsule `json:"sidecar_capsules,omitempty"`
	AdditionalDirectories  []string                    `json:"additional_directories,omitempty"`
	ClientMeta             map[string]interface{}      `json:"client_meta,omitempty"`
	TracePolicy            runtrace.TracePolicy        `json:"trace_policy,omitempty"`
	agentLaunchArgs        []string
}

type runAgentConfig struct {
	ModelReasoningEffort string `json:"model_reasoning_effort,omitempty"`
}

type runExecution struct {
	runID            string
	req              runRequest
	agentID          string
	emergencyTimeout time.Duration
	activityTimeout  time.Duration
}

type runExecutionResult struct {
	output  string
	cleanup *middleware.SessionCleanupResult
}

type eventSinkRequest struct {
	URL        string                 `json:"url"`
	EventKinds []string               `json:"event_kinds,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

func NewServer(router Router) *Server {
	storage := memstore.New()
	server := &Server{
		router:       router,
		defaultAgent: "opencode",
		runCancels:   map[string]context.CancelFunc{},
	}
	server.deliveryStore = rundelivery.NewStore(storage)
	server.workspaceGrants = workspacegrant.NewStore(storage)
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
		s.workspaceGrants = workspacegrant.NewStore(storage)
		s.withRunStore(runtrace.NewStore(storage))
	}
	return s
}

func (s *Server) WithEndpointResolver(resolver middleware.AgentEndpointResolver) *Server {
	s.endpointResolver = resolver
	return s
}

// WithElicitationService wires the shared elicitation SSOT service. The same
// instance must also back the agents router frontend so API responses reach
// the waiting protocol handler.
func (s *Server) WithElicitationService(service *elicitation.Service) *Server {
	if service != nil {
		s.elicitations = &elicitationsHandler{service: service}
		s.subscribeElicitationWakeups(service)
	}
	return s
}

// WithAgentAuthController wires the protocol's authentication controls into the
// runtime API, so an operator or orchestrator can read an agent's advertised
// methods and ask it to log out while the daemon owns the agent clients.
func (s *Server) WithAgentAuthController(controller middleware.AgentAuthenticationController) *Server {
	if controller != nil {
		s.agentAuth = &agentAuthHandler{controller: controller}
	}
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
	mux.HandleFunc(ElicitationPathV1, s.HandleElicitations)
	mux.HandleFunc(AgentAuthPathV1, s.HandleAgentAuth)
	mux.HandleFunc(AgentAuthLogoutPathV1, s.HandleAgentAuth)
	mux.HandleFunc(WorkspaceGrantsPathV1, s.HandleWorkspaceGrants)
	mux.HandleFunc(WorkspaceGrantsPathV1+"/", s.HandleWorkspaceGrants)
}

func (s *Server) withRunStore(store *runtrace.Store) *Server {
	if store != nil {
		s.runStore = store.WithEventDispatcher(s.dispatchRunEvent)
		s.sinkDelivery = runsink.NewService(s.runStore, s.deliveryStore)
	}
	return s
}
