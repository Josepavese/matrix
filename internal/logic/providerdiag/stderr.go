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
const maxLogLines = 64
const maxLineBytes = 4096

var (
	bearerSecretPattern   = regexp.MustCompile(`(?i)\bbearer\s+[^\s,;]+`)
	knownSecretPattern    = regexp.MustCompile(`(?i)\b(?:sk|ghp|github_pat|xox[baprs])[-_][A-Za-z0-9_-]{8,}`)
	jsonSecretPattern     = regexp.MustCompile(`(?i)("(?:authorization|(?:api[_-]?)?key|(?:access[_-]?)?token|secret|password)"\s*:\s*)"(?:[^"\\]|\\.)*(?:"|$)`)
	assignedSecretPattern = regexp.MustCompile(`(?i)\b([A-Z][A-Z0-9_]*(?:TOKEN|KEY|SECRET|PASSWORD)[A-Z0-9_]*)\s*[:=]\s*[^\s,;]+`)
)

type StderrCapture struct {
	mu         sync.RWMutex
	limit      int
	agent      string
	data       []byte
	line       []byte
	logged     int
	suppressed int
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
	lines := w.takeLines(p)
	w.mu.Unlock()
	w.logLines(lines)
	return len(p), nil
}

func (w *StderrCapture) takeLines(p []byte) []string {
	var lines []string
	for len(p) > 0 {
		newline := bytes.IndexByte(p, '\n')
		end := len(p)
		if newline >= 0 {
			end = newline
		}
		remaining := maxLineBytes - len(w.line)
		w.line = append(w.line, p[:min(end, remaining)]...)
		if newline < 0 {
			break
		}
		if w.logged < maxLogLines {
			lines = append(lines, string(w.line))
			w.logged++
		} else {
			w.suppressed++
		}
		w.line = w.line[:0]
		p = p[newline+1:]
	}
	return lines
}

func (w *StderrCapture) Flush() {
	w.mu.Lock()
	lines := w.takeLines([]byte("\n"))
	suppressed := w.suppressed
	w.suppressed = 0
	w.mu.Unlock()
	w.logLines(lines)
	if suppressed > 0 {
		slog.Info("agent stderr suppressed", "agent", w.agent, "lines", suppressed)
	}
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

func Redact(raw string) string {
	if len(raw) > maxLineBytes {
		raw = raw[:maxLineBytes] + "..."
	}
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
	raw = jsonSecretPattern.ReplaceAllString(raw, `${1}"<redacted>"`)
	raw = strings.Join(strings.Fields(raw), " ")
	return strings.ToValidUTF8(raw, "")
}

func Sanitize(raw string) string {
	raw = Redact(raw)

	if len(raw) > MaxStderr {
		raw = raw[:MaxStderr-3] + "..."
	}
	return raw
}
