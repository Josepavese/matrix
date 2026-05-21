package zedacp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
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

const (
	ErrCodeMethodNotFound = -32601
	ErrCodeInternal       = -32603
)

// RPCError lets request handlers return protocol-correct JSON-RPC error codes.
type RPCError struct {
	Code    int
	Message string
	Data    any
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("RPC error %d", e.Code)
}

func NewMethodNotFoundError(method string) error {
	method = strings.TrimSpace(method)
	if method == "" {
		method = "<unknown>"
	}
	return &RPCError{Code: ErrCodeMethodNotFound, Message: "method not found: " + method}
}

func rpcErrorFromError(err error) *jsonRPCError {
	if err == nil {
		return nil
	}
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) && rpcErr != nil {
		return &jsonRPCError{Code: rpcErr.Code, Message: rpcErr.Error(), Data: rpcErr.Data}
	}
	return &jsonRPCError{Code: ErrCodeInternal, Message: err.Error()}
}

// Client multiplexes JSON-RPC 2.0 messages over an ACP transport.
type Client struct {
	transport      Transport
	requestHandler RequestHandler

	mu        sync.RWMutex
	nextID    int64
	pending   map[int64]chan *jsonRPCResponse
	observers map[string]map[uint64]SessionObserver
	nextObsID uint64

	ctx    context.Context
	cancel context.CancelFunc
}

func NewClient(ctx context.Context, transport Transport) *Client {
	cctx, cancel := context.WithCancel(ctx)
	client := &Client{
		transport: transport,
		pending:   make(map[int64]chan *jsonRPCResponse),
		observers: make(map[string]map[uint64]SessionObserver),
		ctx:       cctx,
		cancel:    cancel,
	}
	go client.readLoop()
	return client
}

func (c *Client) Context() context.Context {
	return c.ctx
}

func (c *Client) Close() error {
	c.cancel()
	return c.transport.Close()
}

func (c *Client) SetRequestHandler(handler RequestHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requestHandler = handler
}

func (c *Client) readLoop() {
	log := slog.With("component", "zedacp_client")
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

		if _, ok := raw["method"].(string); ok && raw["id"] != nil {
			var req jsonRPCRequest
			if err := json.Unmarshal(msgBytes, &req); err == nil {
				go c.handleIncomingRequest(req)
				continue
			}
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal(msgBytes, &resp); err != nil {
			continue
		}

		if resp.ID != nil {
			c.mu.RLock()
			ch, ok := c.pending[*resp.ID]
			c.mu.RUnlock()
			if ok {
				ch <- &resp
			}
		} else if resp.Method != nil {
			c.handleNotification(&resp)
		}
	}
}

func (c *Client) handleIncomingRequest(req jsonRPCRequest) {
	log := slog.With("component", "zedacp_client")
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

	resp := jsonRPCResponse{JSONRPC: "2.0", ID: &req.ID}
	if err != nil {
		resp.Error = rpcErrorFromError(err)
	} else {
		resBytes, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			resp.Error = rpcErrorFromError(marshalErr)
		} else {
			resp.Result = json.RawMessage(resBytes)
		}
	}

	respBytes, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		log.Error("failed to marshal response", "event", "response_marshal_failed", "error", marshalErr, "id", req.ID, "method", req.Method)
		return
	}
	if isWireLoggingEnabled() {
		log.Debug("acp wire outbound response", "event", "wire_response_send", "payload", string(respBytes))
	}
	if err := c.transport.Send(c.ctx, respBytes); err != nil {
		log.Error("failed to send response", "event", "response_send_failed", "error", err, "id", req.ID, "method", req.Method)
	}
}

func (c *Client) handleNotification(notif *jsonRPCResponse) {
	if notif.Method == nil {
		return
	}
	if *notif.Method == "session/update" || *notif.Method == "session_info_update" {
		var update SessionNotification
		if err := json.Unmarshal(notif.Params, &update); err == nil {
			if update.SessionID != "" {
				for _, obs := range c.sessionObservers(update.SessionID) {
					obs.OnUpdate(update)
				}
			}
		}
	}
}

func (c *Client) doCall(ctx context.Context, method string, params interface{}) (*jsonRPCResponse, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: json.RawMessage(paramsBytes)}
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

func (c *Client) sendNotification(ctx context.Context, method string, params interface{}) error {
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	req := struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  json.RawMessage(paramsBytes),
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}
	log := slog.With("component", "zedacp_client")
	log.Debug("acp send notification", "event", "notification_send", "method", method)
	if isWireLoggingEnabled() {
		log.Debug("acp wire outbound payload", "event", "wire_outbound", "payload", string(reqBytes))
	}
	return c.transport.Send(ctx, reqBytes)
}

func (c *Client) Initialize(ctx context.Context, req InitializeRequest) (*InitializeResponse, error) {
	resp, err := c.doCall(ctx, "initialize", req)
	if err != nil {
		return nil, err
	}
	var res InitializeResponse
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode initialize response: %w", err)
	}
	return &res, nil
}

func (c *Client) Authenticate(ctx context.Context, methodID string, credentials map[string]string) error {
	_, err := c.doCall(ctx, "authenticate", map[string]interface{}{
		"methodId":    methodID,
		"credentials": credentials,
	})
	return err
}

func (c *Client) NewSession(ctx context.Context, req NewSessionRequest) (*NewSessionResponse, error) {
	resp, err := c.doCall(ctx, "session/new", req)
	if err != nil {
		return nil, err
	}
	var res NewSessionResponse
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode new session response: %w", err)
	}
	return &res, nil
}

func (c *Client) LoadSession(ctx context.Context, req LoadSessionRequest, observer SessionObserver) (*LoadSessionResponse, error) {
	removeObserver := c.registerObserver(req.SessionID, observer)
	defer removeObserver()
	resp, err := c.doCall(ctx, "session/load", req)
	if err != nil {
		return nil, err
	}
	var res LoadSessionResponse
	if err := decodeOptionalResult(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode session/load response: %w", err)
	}
	waitObserverIdle(ctx, observer)
	return &res, nil
}

func (c *Client) ResumeSession(ctx context.Context, req ResumeSessionRequest) (*ResumeSessionResponse, error) {
	resp, err := c.doCall(ctx, "session/resume", req)
	if err != nil {
		return nil, err
	}
	var res ResumeSessionResponse
	if err := decodeOptionalResult(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode session/resume response: %w", err)
	}
	return &res, nil
}

func (c *Client) ListSessions(ctx context.Context) (*ListSessionsResponse, error) {
	return c.ListSessionsWithRequest(ctx, ListSessionsRequest{})
}

func (c *Client) ListSessionsWithRequest(ctx context.Context, req ListSessionsRequest) (*ListSessionsResponse, error) {
	resp, err := c.doCall(ctx, "session/list", req)
	if err != nil {
		return nil, err
	}
	var res ListSessionsResponse
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode session/list response: %w", err)
	}
	return &res, nil
}

func (c *Client) Prompt(ctx context.Context, req PromptRequest, observer SessionObserver) (*PromptResponse, error) {
	removeObserver := c.registerObserver(req.SessionID, observer)
	defer removeObserver()
	resp, err := c.doCall(ctx, "session/prompt", req)
	if err != nil {
		return nil, err
	}
	var res PromptResponse
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode prompt response: %w", err)
	}
	waitObserverIdle(ctx, observer)
	return &res, nil
}

func (c *Client) SetMode(ctx context.Context, sessionID, modeID string) error {
	_, err := c.doCall(ctx, "session/set_mode", map[string]interface{}{
		"sessionId": sessionID,
		"modeId":    modeID,
	})
	return err
}

func (c *Client) SetConfigOption(ctx context.Context, req SetSessionConfigOptionRequest) (*SetSessionConfigOptionResponse, error) {
	resp, err := c.doCall(ctx, "session/set_config_option", req)
	if err != nil {
		return nil, err
	}
	var res SetSessionConfigOptionResponse
	if err := decodeOptionalResult(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode session/set_config_option response: %w", err)
	}
	return &res, nil
}

func (c *Client) SetSessionModel(ctx context.Context, req SetSessionModelRequest) (*SetSessionModelResponse, error) {
	resp, err := c.doCall(ctx, "session/set_model", req)
	if err != nil {
		return nil, err
	}
	var res SetSessionModelResponse
	if err := decodeOptionalResult(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode session/set_model response: %w", err)
	}
	return &res, nil
}

func (c *Client) CancelSession(ctx context.Context, sessionID string) error {
	return c.sendNotification(ctx, "session/cancel", map[string]interface{}{"sessionId": sessionID})
}

func (c *Client) CancelRequest(ctx context.Context, req CancelRequestNotification) error {
	return c.sendNotification(ctx, "$/cancel_request", req)
}

func (c *Client) CloseSession(ctx context.Context, sessionID string) error {
	_, err := c.doCall(ctx, "session/close", map[string]interface{}{"sessionId": sessionID})
	return err
}

func (c *Client) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := c.doCall(ctx, "session/delete", map[string]interface{}{"sessionId": sessionID})
	return err
}

func (c *Client) ForkSession(ctx context.Context, req ForkSessionRequest) (*ForkSessionResponse, error) {
	resp, err := c.doCall(ctx, "session/fork", req)
	if err != nil {
		return nil, err
	}
	var res ForkSessionResponse
	if err := decodeOptionalResult(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode session/fork response: %w", err)
	}
	return &res, nil
}

func (c *Client) ListProviders(ctx context.Context, req ListProvidersRequest) (*ListProvidersResponse, error) {
	resp, err := c.doCall(ctx, "providers/list", req)
	if err != nil {
		return nil, err
	}
	var res ListProvidersResponse
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode providers/list response: %w", err)
	}
	return &res, nil
}

func (c *Client) SetProvider(ctx context.Context, req SetProvidersRequest) (*SetProvidersResponse, error) {
	resp, err := c.doCall(ctx, "providers/set", req)
	if err != nil {
		return nil, err
	}
	var res SetProvidersResponse
	if err := decodeOptionalResult(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode providers/set response: %w", err)
	}
	return &res, nil
}

func (c *Client) DisableProvider(ctx context.Context, req DisableProvidersRequest) (*DisableProvidersResponse, error) {
	resp, err := c.doCall(ctx, "providers/disable", req)
	if err != nil {
		return nil, err
	}
	var res DisableProvidersResponse
	if err := decodeOptionalResult(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode providers/disable response: %w", err)
	}
	return &res, nil
}

func (c *Client) Logout(ctx context.Context, req LogoutRequest) (*LogoutResponse, error) {
	resp, err := c.doCall(ctx, "logout", req)
	if err != nil {
		return nil, err
	}
	var res LogoutResponse
	if err := decodeOptionalResult(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode logout response: %w", err)
	}
	return &res, nil
}

func (c *Client) ExtRequest(ctx context.Context, method string, params interface{}, result interface{}) error {
	method = strings.TrimSpace(method)
	if method == "" {
		return fmt.Errorf("ACP extension method is required")
	}
	resp, err := c.doCall(ctx, method, params)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	if err := decodeOptionalResult(resp.Result, result); err != nil {
		return fmt.Errorf("failed to decode ACP extension response for %s: %w", method, err)
	}
	return nil
}

func (c *Client) ExtNotification(ctx context.Context, method string, params interface{}) error {
	method = strings.TrimSpace(method)
	if method == "" {
		return fmt.Errorf("ACP extension notification method is required")
	}
	return c.sendNotification(ctx, method, params)
}

func decodeOptionalResult(data json.RawMessage, target interface{}) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, target)
}

func (c *Client) logInbound(raw map[string]interface{}, msgBytes []byte) {
	log := slog.With("component", "zedacp_client")
	if method, ok := raw["method"].(string); ok {
		if id, hasID := raw["id"]; hasID && id != nil {
			log.Debug("acp received request", "event", "request_received", "method", method, "id", id)
		} else {
			log.Debug("acp received notification", "event", "notification_received", "method", method)
		}
		if isWireLoggingEnabled() {
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
		if isWireLoggingEnabled() {
			log.Debug("acp wire inbound payload", "event", "wire_inbound", "payload", string(msgBytes))
		}
		return
	}
	if isWireLoggingEnabled() {
		log.Debug("acp wire inbound payload", "event", "wire_inbound", "payload", string(msgBytes))
	}
}

func (c *Client) logOutbound(method string, id int64, reqBytes []byte) {
	log := slog.With("component", "zedacp_client")
	log.Debug("acp send request", "event", "request_send", "method", method, "id", id)
	if isWireLoggingEnabled() {
		log.Debug("acp wire outbound payload", "event", "wire_outbound", "payload", string(reqBytes))
	}
}

func isWireLoggingEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("MATRIX_LOG_ACP_WIRE")), "true")
}
