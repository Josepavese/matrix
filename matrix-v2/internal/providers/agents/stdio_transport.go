package agents

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

// StdioTransport implements middleware.AgentTransport using the standard I/O streams
// of a spawned child process executing an ACP agent.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader
	wg     sync.WaitGroup
}

// NewStdioTransport starts an agent executable entirely locally, binding to its standard I/O for JSON-RPC communication.
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

	// Capture agent stderr for diagnostics
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

	slog.Info("stdio transport: agent process started", "agent", executable, "pid", cmd.Process.Pid, "args", args)

	// Monitor process exit in background
	t := &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		reader: bufio.NewReader(stdout),
	}
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		err := cmd.Wait()
		slog.Info("stdio transport: agent process exited", "agent", executable, "pid", cmd.Process.Pid, "wait_err", err)
	}()

	return t, nil
}

// Send writes a generic JSON message to the agent over standard input.
// ACP dictates that each message should be a compact JSON ending with a newline.
func (t *StdioTransport) Send(_ context.Context, message []byte) error {
	// Let's ensure there's a newline to flush.
	msgWithNewline := append(append([]byte{}, message...), '\n')
	_, err := t.stdin.Write(msgWithNewline)
	if err != nil {
		return fmt.Errorf("stdio send error: %w", err)
	}
	return nil
}

// Receive blocks until a complete JSON line is read from the agent's standard output.
func (t *StdioTransport) Receive(_ context.Context) ([]byte, error) {
	// ACP over stdio relies on newline delimited JSON (NDJSON) or standard framing.
	line, err := t.reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("stdio receive error: %w", err)
	}

	// Remove trailing newline chars for standard JSON parsing
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}

	return line, nil
}

// Close gracefully kills the child process and cleans up the pipes.
// Waits for the background cmd.Wait() goroutine to finish.
func (t *StdioTransport) Close() error {
	slog.Info("stdio transport: close called", "pid", t.cmd.Process.Pid)
	_ = t.stdin.Close()
	_ = t.stdout.Close()
	err := t.cmd.Cancel()
	t.wg.Wait()
	return err
}
