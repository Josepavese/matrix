package agents

import (
	"context"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// Local ACP aliases isolate the Matrix runtime from a concrete Go ACP SDK.
// Today they map to pkg/zedacp; a future migration can replace only this port.
type (
	acpRequestHandler      = zedacp.RequestHandler
	acpSessionObserver     = zedacp.SessionObserver
	acpSessionNotification = zedacp.SessionNotification
	acpSessionUpdate       = zedacp.SessionUpdate

	acpInitializeRequest       = zedacp.InitializeRequest
	acpInitializeResponse      = zedacp.InitializeResponse
	acpClientCapabilities      = zedacp.ClientCapabilities
	acpAuthCapabilities        = zedacp.AuthCapabilities
	acpAuthMethod              = zedacp.AuthMethod
	acpFsCapability            = zedacp.FsCapability
	acpNewSessionRequest       = zedacp.NewSessionRequest
	acpNewSessionResponse      = zedacp.NewSessionResponse
	acpLoadSessionRequest      = zedacp.LoadSessionRequest
	acpLoadSessionResponse     = zedacp.LoadSessionResponse
	acpResumeSessionRequest    = zedacp.ResumeSessionRequest
	acpResumeSessionResponse   = zedacp.ResumeSessionResponse
	acpForkSessionRequest      = zedacp.ForkSessionRequest
	acpForkSessionResponse     = zedacp.ForkSessionResponse
	acpListSessionsRequest     = zedacp.ListSessionsRequest
	acpListSessionsResponse    = zedacp.ListSessionsResponse
	acpSessionInfo             = zedacp.SessionInfo
	acpSetConfigOptionRequest  = zedacp.SetSessionConfigOptionRequest
	acpSetConfigOptionResponse = zedacp.SetSessionConfigOptionResponse
	acpSessionModeState        = zedacp.SessionModeState
	acpConfigOption            = zedacp.ConfigOption
	acpMcpServerConfig         = zedacp.McpServerConfig
	zedacpEnvVar               = zedacp.EnvVar
	zedacpHeader               = zedacp.Header
	acpTool                    = zedacp.Tool
	acpPromptRequest           = zedacp.PromptRequest
	acpPromptResponse          = zedacp.PromptResponse
	acpToolCall                = zedacp.ToolCall
	acpToolCallContent         = zedacp.ToolCallContent
	acpContent                 = zedacp.Content
)

type acpClient interface {
	Context() context.Context
	Close() error
	SetRequestHandler(handler acpRequestHandler)
	Initialize(ctx context.Context, req acpInitializeRequest) (*acpInitializeResponse, error)
	Authenticate(ctx context.Context, methodID string, credentials map[string]string) error
	NewSession(ctx context.Context, req acpNewSessionRequest) (*acpNewSessionResponse, error)
	LoadSession(ctx context.Context, req acpLoadSessionRequest, observer acpSessionObserver) (*acpLoadSessionResponse, error)
	ResumeSession(ctx context.Context, req acpResumeSessionRequest) (*acpResumeSessionResponse, error)
	ListSessions(ctx context.Context) (*acpListSessionsResponse, error)
	ListSessionsWithRequest(ctx context.Context, req acpListSessionsRequest) (*acpListSessionsResponse, error)
	CancelSession(ctx context.Context, sessionID string) error
	CloseSession(ctx context.Context, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
	ForkSession(ctx context.Context, req acpForkSessionRequest) (*acpForkSessionResponse, error)
	Prompt(ctx context.Context, req acpPromptRequest, observer acpSessionObserver) (*acpPromptResponse, error)
	SetMode(ctx context.Context, sessionID, modeID string) error
	SetConfigOption(ctx context.Context, req acpSetConfigOptionRequest) (*acpSetConfigOptionResponse, error)
	ExtRequest(ctx context.Context, method string, params interface{}, result interface{}) error
	ExtNotification(ctx context.Context, method string, params interface{}) error
}

type acpSDK interface {
	NewClient(ctx context.Context, transport middleware.AgentTransport) acpClient
}

type zedACPSDK struct{}

func (zedACPSDK) NewClient(ctx context.Context, transport middleware.AgentTransport) acpClient {
	return zedacp.NewClient(ctx, transport)
}

var defaultACPSDK acpSDK = zedACPSDK{}
