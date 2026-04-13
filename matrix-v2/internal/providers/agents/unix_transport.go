package agents

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// UnixTransport implements middleware.AgentTransport using a Unix domain socket
// with JSON-RPC 2.0 newline-delimited framing.
type UnixTransport struct {
	conn   net.Conn
	mu     sync.Mutex
	reader *bufio.Reader
}

// NewUnixTransport connects to a Unix domain socket at socketPath and returns a new transport.
func NewUnixTransport(ctx context.Context, socketPath string) (*UnixTransport, error) {
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("unix socket dial error to %s: %w", socketPath, err)
	}

	return &UnixTransport{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

// Send writes a JSON-RPC message followed by a newline to the Unix socket.
// It is safe for concurrent use.
func (t *UnixTransport) Send(_ context.Context, message []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	msg := append(append([]byte{}, message...), '\n')
	_, err := t.conn.Write(msg)
	if err != nil {
		return fmt.Errorf("unix socket write error: %w", err)
	}
	return nil
}

// Receive blocks until a complete newline-delimited JSON message is read from the socket
// or the context is cancelled.
func (t *UnixTransport) Receive(ctx context.Context) ([]byte, error) {
	type readResult struct {
		data []byte
		err  error
	}
	ch := make(chan readResult, 1)

	go func() {
		line, err := t.reader.ReadBytes('\n')
		if err != nil {
			ch <- readResult{nil, err}
			return
		}
		// Strip trailing \r\n or \n
		line = bytes.TrimRight(line, "\r\n")
		ch <- readResult{line, nil}
	}()

	select {
	case <-ctx.Done():
		// Close the connection to unblock the ReadBytes goroutine
		_ = t.conn.Close()
		<-ch // wait for goroutine to finish
		return nil, ctx.Err()
	case result := <-ch:
		if result.err != nil {
			return nil, fmt.Errorf("unix socket read error: %w", result.err)
		}
		return result.data, nil
	}
}

// Close closes the underlying Unix socket connection.
func (t *UnixTransport) Close() error {
	return t.conn.Close()
}
