package zedacpstdio

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/logic/providerdiag"
)

type Transport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader
	wg     sync.WaitGroup

	stderr   *providerdiag.StderrCapture
	waitMu   sync.RWMutex
	waitErr  error
	waitDone chan struct{}
}

func New(ctx context.Context, executable string, env []string, args ...string) (*Transport, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	prepareCommand(cmd)
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
	stderr := providerdiag.NewStderrCapture(8192, executable)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start agent %s: %w", executable, err)
	}
	t := &Transport{cmd: cmd, stdin: stdin, stdout: stdout, reader: bufio.NewReader(stdout), stderr: stderr, waitDone: make(chan struct{})}
	t.wg.Add(1)
	go t.wait(executable)
	return t, nil
}

func (t *Transport) wait(executable string) {
	defer t.wg.Done()
	err := t.cmd.Wait()
	t.stderr.Flush()
	t.waitMu.Lock()
	t.waitErr = err
	t.waitMu.Unlock()
	close(t.waitDone)
	slog.Info("stdio transport: agent process exited", "agent", executable, "pid", t.cmd.Process.Pid, "wait_err", err)
}

func (t *Transport) Failure(receiveErr error) error {
	select {
	case <-t.waitDone:
	case <-time.After(250 * time.Millisecond):
		return receiveErr
	}
	t.waitMu.RLock()
	waitErr := t.waitErr
	t.waitMu.RUnlock()
	exitCode := -1
	if t.cmd.ProcessState != nil {
		exitCode = t.cmd.ProcessState.ExitCode()
	}
	stderr := t.stderr.Sanitized()
	if waitErr == nil && stderr == "" {
		return receiveErr
	}
	return &providerdiag.ProcessFailure{ExitCode: exitCode, Stderr: stderr, Err: waitErr}
}

func (t *Transport) Send(_ context.Context, message []byte) error {
	msgWithNewline := append(append([]byte{}, message...), '\n')
	if _, err := t.stdin.Write(msgWithNewline); err != nil {
		return fmt.Errorf("stdio send error: %w", err)
	}
	return nil
}

func (t *Transport) Receive(_ context.Context) ([]byte, error) {
	line, err := t.reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("stdio receive error: %w", err)
	}
	line = bytesTrimLineEnding(line)
	return line, nil
}

func bytesTrimLineEnding(line []byte) []byte {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line
}

func (t *Transport) Close() error {
	_ = t.stdin.Close()
	_ = t.stdout.Close()
	err := terminateCommand(t.cmd)
	t.wg.Wait()
	return err
}
