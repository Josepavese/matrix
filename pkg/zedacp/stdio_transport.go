package zedacp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
)

type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader
	wg     sync.WaitGroup
}

func NewStdioTransport(ctx context.Context, executable string, env []string, args ...string) (*StdioTransport, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe for %s: %w", executable, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe for %s: %w", executable, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr pipe for %s: %w", executable, err)
	}
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			slog.Info("agent stderr", "agent", executable, "stderr", scanner.Text())
		}
	}()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start agent %s: %w", executable, err)
	}

	t := &StdioTransport{cmd: cmd, stdin: stdin, stdout: stdout, reader: bufio.NewReader(stdout)}
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		err := cmd.Wait()
		slog.Info("stdio transport: agent process exited", "agent", executable, "pid", cmd.Process.Pid, "wait_err", err)
	}()
	return t, nil
}

func (t *StdioTransport) Send(_ context.Context, message []byte) error {
	msgWithNewline := append(append([]byte{}, message...), '\n')
	_, err := t.stdin.Write(msgWithNewline)
	if err != nil {
		return fmt.Errorf("stdio send error: %w", err)
	}
	return nil
}

func (t *StdioTransport) Receive(_ context.Context) ([]byte, error) {
	line, err := t.reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("stdio receive error: %w", err)
	}
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line, nil
}

func (t *StdioTransport) Close() error {
	_ = t.stdin.Close()
	_ = t.stdout.Close()
	err := t.cmd.Cancel()
	t.wg.Wait()
	return err
}
