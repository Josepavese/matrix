package zedacp

import (
	"context"
	"encoding/json"
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

// Client multiplexes JSON-RPC 2.0 messages over an ACP transport.
type Client struct {
	transport      Transport
	requestHandler RequestHandler

	mu        sync.RWMutex
	nextID    int64
	pending   map[int64]chan *jsonRPCResponse
	observers map[string]SessionObserver

	ctx    context.Context
	cancel context.CancelFunc
}

func NewClient(ctx context.Context, transport Transport) *Client {
	cctx, cancel := context.WithCancel(ctx)
	client := &Client{
		transport: transport,
		pending:   make(map[int64]chan *jsonRPCResponse),
		observers: make(map[string]SessionObserver),
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

func (c *Client) LoadSession(ctx context.Context, req LoadSessionRequest, observer SessionObserver) error {
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
	_, err := c.doCall(ctx, "session/load", req)
	return err
}

func (c *Client) ListSessions(ctx context.Context) (*ListSessionsResponse, error) {
	resp, err := c.doCall(ctx, "session/list", map[string]interface{}{})
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
	var res PromptResponse
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("failed to decode prompt response: %w", err)
	}
	return &res, nil
}

func (c *Client) SetMode(ctx context.Context, sessionID, modeID string) error {
	_, err := c.doCall(ctx, "session/set_mode", map[string]interface{}{
		"sessionId": sessionID,
		"modeId":    modeID,
	})
	return err
}

func (c *Client) CancelSession(ctx context.Context, sessionID string) error {
	return c.sendNotification(ctx, "session/cancel", map[string]interface{}{"sessionId": sessionID})
}

func (c *Client) CloseSession(ctx context.Context, sessionID string) error {
	_, err := c.doCall(ctx, "session/close", map[string]interface{}{"sessionId": sessionID})
	return err
}

func (c *Client) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := c.doCall(ctx, "session/delete", map[string]interface{}{"sessionId": sessionID})
	return err
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
