package providerdiag

import (
	"bytes"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
)

const MaxStderr = 400

var (
	bearerSecretPattern   = regexp.MustCompile(`(?i)\bbearer\s+[^\s,;]+`)
	knownSecretPattern    = regexp.MustCompile(`(?i)\b(?:sk|ghp|github_pat|xox[baprs])[-_][A-Za-z0-9_-]{8,}`)
	assignedSecretPattern = regexp.MustCompile(`(?i)\b([A-Z][A-Z0-9_]*(?:TOKEN|KEY|SECRET|PASSWORD)[A-Z0-9_]*)\s*[:=]\s*[^\s,;]+`)
)

type StderrCapture struct {
	mu    sync.RWMutex
	limit int
	agent string
	data  []byte
	line  []byte
}

func NewStderrCapture(limit int, agent string) *StderrCapture {
	return &StderrCapture{limit: limit, agent: agent}
}

func (w *StderrCapture) Write(p []byte) (int, error) {
	w.mu.Lock()
	remaining := min(w.limit-len(w.data), len(p))
	if remaining > 0 {
		w.data = append(w.data, p[:remaining]...)
	}
	w.line = append(w.line, p...)
	lines := w.takeLines()
	w.mu.Unlock()
	w.logLines(lines)
	return len(p), nil
}

func (w *StderrCapture) takeLines() []string {
	var lines []string
	for {
		newline := bytes.IndexByte(w.line, '\n')
		if newline < 0 {
			break
		}
		lines = append(lines, string(w.line[:newline]))
		w.line = w.line[newline+1:]
	}
	if len(w.line) > 4096 {
		lines = append(lines, string(w.line))
		w.line = nil
	}
	return lines
}

func (w *StderrCapture) Flush() {
	w.mu.Lock()
	line := string(w.line)
	w.line = nil
	w.mu.Unlock()
	w.logLines([]string{line})
}

func (w *StderrCapture) logLines(lines []string) {
	for _, line := range lines {
		if stderr := Sanitize(line); stderr != "" {
			slog.Info("agent stderr", "agent", w.agent, "stderr", stderr)
		}
	}
}

func (w *StderrCapture) Sanitized() string {
	w.mu.RLock()
	raw := string(append([]byte{}, w.data...))
	w.mu.RUnlock()
	return Sanitize(raw)
}

func Sanitize(raw string) string {
	raw = strings.ReplaceAll(raw, "\x00", "\\0")
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		home = strings.TrimSpace(os.Getenv("USERPROFILE"))
	}
	if home != "" {
		raw = strings.ReplaceAll(raw, home, "~")
	}
	raw = bearerSecretPattern.ReplaceAllString(raw, "Bearer <redacted>")
	raw = knownSecretPattern.ReplaceAllString(raw, "<redacted>")
	raw = assignedSecretPattern.ReplaceAllString(raw, "$1=<redacted>")
	raw = strings.Join(strings.Fields(raw), " ")
	if len(raw) > MaxStderr {
		raw = raw[:MaxStderr]
	}
	return raw
}
