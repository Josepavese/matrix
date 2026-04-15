package zedacp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"
)

type WSTransport struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func NewWSTransport(ctx context.Context, url string) (*WSTransport, error) {
	dialer := websocket.DefaultDialer
	conn, resp, err := dialer.DialContext(ctx, url, nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("websocket dial error (status %d): %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("websocket dial error to %s: %w", url, err)
	}
	return &WSTransport{conn: conn}, nil
}

func (t *WSTransport) Send(_ context.Context, message []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.conn.WriteMessage(websocket.TextMessage, message); err != nil {
		return fmt.Errorf("websocket write error: %w", err)
	}
	return nil
}

func (t *WSTransport) Receive(ctx context.Context) ([]byte, error) {
	type readResult struct {
		msgType int
		data    []byte
		err     error
	}
	ch := make(chan readResult, 1)
	go func() {
		msgType, p, err := t.conn.ReadMessage()
		ch <- readResult{msgType: msgType, data: p, err: err}
	}()

	select {
	case <-ctx.Done():
		_ = t.conn.Close()
		<-ch
		return nil, ctx.Err()
	case result := <-ch:
		if result.err != nil {
			return nil, fmt.Errorf("websocket read error: %w", result.err)
		}
		if result.msgType != websocket.TextMessage && result.msgType != websocket.BinaryMessage {
			slog.Warn("websocket transport received unsupported message type", "message_type", result.msgType)
		}
		return result.data, nil
	}
}

func (t *WSTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	cm := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	if err := t.conn.WriteMessage(websocket.CloseMessage, cm); err != nil {
		slog.Debug("websocket close frame failed", "error", err)
	}
	return t.conn.Close()
}
