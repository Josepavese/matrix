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

	acpInitializeRequest    = zedacp.InitializeRequest
	acpInitializeResponse   = zedacp.InitializeResponse
	acpClientCapabilities   = zedacp.ClientCapabilities
	acpFsCapability         = zedacp.FsCapability
	acpNewSessionRequest    = zedacp.NewSessionRequest
	acpNewSessionResponse   = zedacp.NewSessionResponse
	acpLoadSessionRequest   = zedacp.LoadSessionRequest
	acpForkSessionRequest   = zedacp.ForkSessionRequest
	acpForkSessionResponse  = zedacp.ForkSessionResponse
	acpListSessionsResponse = zedacp.ListSessionsResponse
	acpMcpServerConfig      = zedacp.McpServerConfig
	acpTool                 = zedacp.Tool
	acpPromptRequest        = zedacp.PromptRequest
	acpPromptResponse       = zedacp.PromptResponse
	acpToolCall             = zedacp.ToolCall
	acpContent              = zedacp.Content
)

type acpClient interface {
	Context() context.Context
	Close() error
	SetRequestHandler(handler acpRequestHandler)
	Initialize(ctx context.Context, req acpInitializeRequest) (*acpInitializeResponse, error)
	Authenticate(ctx context.Context, methodID string, credentials map[string]string) error
	NewSession(ctx context.Context, req acpNewSessionRequest) (*acpNewSessionResponse, error)
	LoadSession(ctx context.Context, req acpLoadSessionRequest, observer acpSessionObserver) error
	ListSessions(ctx context.Context) (*acpListSessionsResponse, error)
	CancelSession(ctx context.Context, sessionID string) error
	CloseSession(ctx context.Context, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
	ForkSession(ctx context.Context, req acpForkSessionRequest) (*acpForkSessionResponse, error)
	Prompt(ctx context.Context, req acpPromptRequest, observer acpSessionObserver) (*acpPromptResponse, error)
	SetMode(ctx context.Context, sessionID, modeID string) error
}

type acpSDK interface {
	NewClient(ctx context.Context, transport middleware.AgentTransport) acpClient
}

type zedACPSDK struct{}

func (zedACPSDK) NewClient(ctx context.Context, transport middleware.AgentTransport) acpClient {
	return zedacp.NewClient(ctx, transport)
}

var defaultACPSDK acpSDK = zedACPSDK{}
