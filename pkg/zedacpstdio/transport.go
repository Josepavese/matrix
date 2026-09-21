package zedacpstdio

import (
	"bufio"
	"context"
	"errors"
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
	// sendMu serialises writes. The ACP client writes from the read loop
	// (responses) and from caller goroutines (requests) at the same time, and
	// a pipe does not keep two concurrent writes whole: frames larger than
	// PIPE_BUF can interleave inside a single line.
	sendMu sync.Mutex
	// maxFrameBytes bounds one inbound frame so a peer cannot allocate without
	// limit before the decoder ever runs.
	maxFrameBytes int
	wg            sync.WaitGroup

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
		// Otherwise the stdin pipe leaks on every failed launch, and agent
		// retries would eventually exhaust the descriptor table.
		_ = stdin.Close()
		return nil, fmt.Errorf("failed to get stdout pipe for %s: %w", executable, err)
	}
	stderr := providerdiag.NewStderrCapture(8192, executable)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("failed to start agent %s: %w", executable, err)
	}
	t := &Transport{cmd: cmd, stdin: stdin, stdout: stdout, reader: bufio.NewReader(stdout), stderr: stderr, maxFrameBytes: DefaultMaxFrameBytes, waitDone: make(chan struct{})}
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
	t.sendMu.Lock()
	defer t.sendMu.Unlock()
	if _, err := t.stdin.Write(msgWithNewline); err != nil {
		return fmt.Errorf("stdio send error: %w", err)
	}
	return nil
}

// DefaultMaxFrameBytes bounds a single ACP frame. Real frames are far smaller;
// the limit exists so a hostile or broken peer cannot exhaust memory with one
// unterminated line.
const DefaultMaxFrameBytes = 32 << 20

// ErrFrameTooLarge is returned when a peer sends a frame above the limit.
var ErrFrameTooLarge = errors.New("acp frame exceeds the maximum size")

func (t *Transport) Receive(_ context.Context) ([]byte, error) {
	limit := t.maxFrameBytes
	if limit <= 0 {
		limit = DefaultMaxFrameBytes
	}
	line, err := readBoundedLine(t.reader, limit)
	if err != nil {
		return nil, fmt.Errorf("stdio receive error: %w", err)
	}
	return bytesTrimLineEnding(line), nil
}

// readBoundedLine reads until a newline, refusing to buffer more than limit
// bytes so an oversized frame fails loudly instead of growing without bound.
func readBoundedLine(reader *bufio.Reader, limit int) ([]byte, error) {
	var buffer []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		buffer = append(buffer, chunk...)
		if len(buffer) > limit {
			return nil, ErrFrameTooLarge
		}
		switch {
		case err == nil:
			return buffer, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return nil, err
		}
	}
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
