package zedacp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

type UnixTransport struct {
	conn   net.Conn
	mu     sync.Mutex
	reader *bufio.Reader
}

func NewUnixTransport(ctx context.Context, socketPath string) (*UnixTransport, error) {
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("unix socket dial error to %s: %w", socketPath, err)
	}
	return &UnixTransport{conn: conn, reader: bufio.NewReader(conn)}, nil
}

func (t *UnixTransport) Send(_ context.Context, message []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	msg := append(append([]byte{}, message...), '\n')
	if _, err := t.conn.Write(msg); err != nil {
		return fmt.Errorf("unix socket write error: %w", err)
	}
	return nil
}

func (t *UnixTransport) Receive(ctx context.Context) ([]byte, error) {
	type readResult struct {
		data []byte
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := t.reader.ReadBytes('\n')
		if err != nil {
			ch <- readResult{err: err}
			return
		}
		ch <- readResult{data: bytes.TrimRight(line, "\r\n")}
	}()

	select {
	case <-ctx.Done():
		_ = t.conn.Close()
		<-ch
		return nil, ctx.Err()
	case result := <-ch:
		if result.err != nil {
			return nil, fmt.Errorf("unix socket read error: %w", result.err)
		}
		return result.data, nil
	}
}

func (t *UnixTransport) Close() error {
	return t.conn.Close()
}
