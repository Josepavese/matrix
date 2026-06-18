package zedacp

import (
	"encoding/json"
	"log/slog"
)

func (c *Client) readLoop() {
	log := slog.With("component", "zedacp_client")
	defer c.cancel()
	for {
		msgBytes, err := c.transport.Receive(c.ctx)
		if err != nil {
			log.Info("acp transport closed", "event", "transport_closed", "error", err)
			return
		}
		c.handleInboundBytes(log, msgBytes)
	}
}

func (c *Client) handleInboundBytes(log *slog.Logger, msgBytes []byte) {
	raw, ok := decodeInboundRaw(log, msgBytes)
	if !ok {
		return
	}
	c.logInbound(raw, msgBytes)
	if c.handleInboundMethodMessage(msgBytes, raw) {
		return
	}
	c.handleInboundResponseMessage(msgBytes)
}

func decodeInboundRaw(log *slog.Logger, msgBytes []byte) (map[string]interface{}, bool) {
	var raw map[string]interface{}
	if err := json.Unmarshal(msgBytes, &raw); err != nil {
		log.Warn("acp transport received invalid json", "event", "invalid_json", "error", err, "bytes_len", len(msgBytes))
		return nil, false
	}
	return raw, true
}

func (c *Client) handleInboundMethodMessage(msgBytes []byte, raw map[string]interface{}) bool {
	if _, ok := raw["method"].(string); !ok {
		return false
	}
	if _, hasID := raw["id"]; !hasID {
		c.handleInboundNotification(msgBytes)
		return true
	}
	var req jsonRPCRequest
	if err := json.Unmarshal(msgBytes, &req); err == nil {
		go c.handleIncomingRequest(req)
	}
	return true
}

func (c *Client) handleInboundNotification(msgBytes []byte) {
	var notif jsonRPCResponse
	if err := json.Unmarshal(msgBytes, &notif); err == nil {
		c.handleNotification(&notif)
	}
}

func (c *Client) handleInboundResponseMessage(msgBytes []byte) {
	var resp jsonRPCResponse
	if err := json.Unmarshal(msgBytes, &resp); err != nil {
		return
	}
	if len(resp.ID) > 0 {
		c.dispatchResponse(&resp)
		return
	}
	if resp.Method != nil {
		c.handleNotification(&resp)
	}
}

func (c *Client) dispatchResponse(resp *jsonRPCResponse) {
	id, ok := jsonRPCIDInt64(resp.ID)
	if !ok {
		return
	}
	c.mu.RLock()
	ch, ok := c.pending[id]
	c.mu.RUnlock()
	if ok {
		ch <- resp
	}
}
