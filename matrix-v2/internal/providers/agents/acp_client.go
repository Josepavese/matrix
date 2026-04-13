// Package agents provides the ACP agent client pool and request routing.
package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jose/matrix-v2/internal/middleware"
)

// jsonRPCRequest represents a JSON-RPC 2.0 request (Client -> Agent or Agent -> Client).
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response or notification.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"` // null for notifications
	Method  *string         `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ACPClient implements middleware.AgentClient. It multiplexes JSON-RPC 2.0 messages over an AgentTransport.
type ACPClient struct {
	transport      middleware.AgentTransport
	requestHandler middleware.RequestHandler

	mu        sync.RWMutex
	nextID    int64
	pending   map[int64]chan *jsonRPCResponse
	observers map[string]middleware.SessionObserver

	ctx    context.Context
	cancel context.CancelFunc
}

// NewACPClient creates a new ACP Multiplexer wrapping a transport.
func NewACPClient(ctx context.Context, transport middleware.AgentTransport) *ACPClient {
	cCtx, cancel := context.WithCancel(ctx)
	client := &ACPClient{
		transport: transport,
		pending:   make(map[int64]chan *jsonRPCResponse),
		observers: make(map[string]middleware.SessionObserver),
		ctx:       cCtx,
		cancel:    cancel,
	}
	go client.readLoop()
	return client
}

// SetRequestHandler sets the handler for incoming JSON-RPC requests from the agent.
func (c *ACPClient) SetRequestHandler(handler middleware.RequestHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requestHandler = handler
}

func (c *ACPClient) readLoop() {
	log := slog.With("component", "acp_client")
	defer c.cancel()
	for {
		msgBytes, err := c.transport.Receive(c.ctx)
		if err != nil {
			log.Info("acp transport closed", "event", "transport_closed", "error", err)
			return
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(msgBytes, &raw); err != nil {
			log.Warn("acp transport received invalid json", "event", "invalid_json", "error", err, "bytes_len", len(msgBytes))
			continue
		}

		c.logInbound(raw, msgBytes)

		// 1. Check if it's an incoming Request (Agent -> Client)
		// A request MUST have "method" and "id"
		if method, ok := raw["method"].(string); ok && raw["id"] != nil {
			var req jsonRPCRequest
			if err := json.Unmarshal(msgBytes, &req); err == nil {
				go c.handleIncomingRequest(req)
				_ = method // idenitified as request
				continue
			}
		}

		// 2. Otherwise try parse as Response or Notification
		var resp jsonRPCResponse
		if err := json.Unmarshal(msgBytes, &resp); err != nil {
			continue
		}

		if resp.ID != nil {
			// This is a response to an outgoing call
			c.mu.RLock()
			ch, ok := c.pending[*resp.ID]
			c.mu.RUnlock()
			if ok {
				ch <- &resp
			}
		} else if resp.Method != nil {
			// This is a notification
			c.handleNotification(&resp)
		}
	}
}

func (c *ACPClient) handleIncomingRequest(req jsonRPCRequest) {
	log := slog.With("component", "acp_client")
	c.mu.RLock()
	handler := c.requestHandler
	c.mu.RUnlock()

	var result interface{}
	var err error

	if handler != nil {
		result, err = handler.HandleRequest(c.ctx, req.Method, req.Params)
	} else {
		err = fmt.Errorf("no request handler registered for method %s", req.Method)
	}

	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      &req.ID,
	}
	if err != nil {
		resp.Error = &jsonRPCError{Code: -32603, Message: err.Error()}
	} else {
		resBytes, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			resp.Error = &jsonRPCError{Code: -32603, Message: marshalErr.Error()}
		} else {
			resp.Result = json.RawMessage(resBytes)
		}
	}

	respBytes, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		log.Error("failed to marshal response", "event", "response_marshal_failed", "error", marshalErr, "id", req.ID, "method", req.Method)
		return
	}
	log.Debug("acp send response", "event", "response_send", "id", req.ID, "method", req.Method)
	if isACPWireLoggingEnabled() {
		log.Debug("acp wire outbound response", "event", "wire_response_send", "payload", string(respBytes))
	}
	if err := c.transport.Send(c.ctx, respBytes); err != nil {
		log.Error("failed to send response", "event", "response_send_failed", "error", err, "id", req.ID, "method", req.Method)
	}
}

func (c *ACPClient) handleNotification(notif *jsonRPCResponse) {
	if notif.Method == nil {
		return
	}
	if *notif.Method == "session/update" {
		var update middleware.SessionNotification
		if err := json.Unmarshal(notif.Params, &update); err == nil {
			if update.SessionID != "" {
				c.mu.RLock()
				obs, ok := c.observers[update.SessionID]
				c.mu.RUnlock()
				if ok && obs != nil {
					obs.OnUpdate(update)
				}
			}
		}
	}
}

func (c *ACPClient) doCall(ctx context.Context, method string, params interface{}) (*jsonRPCResponse, error) {
	id := atomic.AddInt64(&c.nextID, 1)

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  json.RawMessage(paramsBytes),
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	c.logOutbound(method, id, reqBytes)

	ch := make(chan *jsonRPCResponse, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.transport.Send(ctx, reqBytes); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ctx.Done():
		return nil, fmt.Errorf("client context cancelled")
	case resp := <-ch:
		if resp.Error != nil {
			if resp.Error.Data != nil {
				return nil, fmt.Errorf("RPC error %d: %s (%v)", resp.Error.Code, resp.Error.Message, resp.Error.Data)
			}
			return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	}
}

func (c *ACPClient) logInbound(raw map[string]interface{}, msgBytes []byte) {
	log := slog.With("component", "acp_client")
	if method, ok := raw["method"].(string); ok {
		if id, hasID := raw["id"]; hasID && id != nil {
			log.Debug("acp received request", "event", "request_received", "method", method, "id", id)
		} else {
			log.Debug("acp received notification", "event", "notification_received", "method", method)
		}
		if isACPWireLoggingEnabled() {
			log.Debug("acp wire inbound payload", "event", "wire_inbound", "payload", string(msgBytes))
		}
		return
	}

	if id, ok := raw["id"]; ok {
		if _, hasErr := raw["error"]; hasErr {
			log.Debug("acp received error response", "event", "error_response_received", "id", id)
		} else {
			log.Debug("acp received response", "event", "response_received", "id", id)
		}
		if isACPWireLoggingEnabled() {
			log.Debug("acp wire inbound payload", "event", "wire_inbound", "payload", string(msgBytes))
		}
		return
	}

	log.Debug("acp received unclassified message", "event", "message_unclassified", "bytes_len", len(msgBytes))
	if isACPWireLoggingEnabled() {
		log.Debug("acp wire inbound payload", "event", "wire_inbound", "payload", string(msgBytes))
	}
}

func (c *ACPClient) logOutbound(method string, id int64, reqBytes []byte) {
	log := slog.With("component", "acp_client")
	log.Debug("acp send request", "event", "request_send", "method", method, "id", id)
	if isACPWireLoggingEnabled() {
		log.Debug("acp wire outbound payload", "event", "wire_outbound", "payload", string(reqBytes))
	}
}

func isACPWireLoggingEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("MATRIX_LOG_ACP_WIRE")), "true")
}

// Initialize sends an ACP initialize request to the agent.
func (c *ACPClient) Initialize(ctx context.Context, req middleware.InitializeRequest) (*middleware.InitializeResponse, error) {
	resp, err := c.doCall(ctx, "initialize", req)
	if err != nil {
		return nil, err
	}
	var res middleware.InitializeResponse
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode initialize response: %w", err)
	}
	return &res, nil
}

// Authenticate sends an authenticate JSON-RPC call to the agent with the given method and credentials.
func (c *ACPClient) Authenticate(ctx context.Context, methodID string, credentials map[string]string) error {
	params := map[string]interface{}{
		"methodId":    methodID,
		"credentials": credentials,
	}
	_, err := c.doCall(ctx, "authenticate", params)
	return err
}

// NewSession creates a new ACP session with the agent.
func (c *ACPClient) NewSession(ctx context.Context, req middleware.NewSessionRequest) (*middleware.NewSessionResponse, error) {
	resp, err := c.doCall(ctx, "session/new", req)
	if err != nil {
		return nil, err
	}
	var res middleware.NewSessionResponse
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode new session response: %w", err)
	}
	return &res, nil
}

// Prompt sends a prompt to the agent within an existing session.
func (c *ACPClient) Prompt(ctx context.Context, req middleware.PromptRequest, observer middleware.SessionObserver) (*middleware.PromptResponse, error) {
	if observer != nil {
		c.mu.Lock()
		c.observers[req.SessionID] = observer
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			delete(c.observers, req.SessionID)
			c.mu.Unlock()
		}()
	}

	resp, err := c.doCall(ctx, "session/prompt", req)
	if err != nil {
		return nil, err
	}
	var res middleware.PromptResponse
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode prompt response: %w", err)
	}
	return &res, nil
}

// SetMode switches the agent session mode using the ACP session/set_mode method.
// The modeId must be one of the available mode IDs returned by session/new.
func (c *ACPClient) SetMode(ctx context.Context, sessionID, modeID string) error {
	_, err := c.doCall(ctx, "session/set_mode", map[string]interface{}{
		"sessionId": sessionID,
		"modeId":    modeID,
	})
	return err
}
