package onboarding

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/jose/matrix-v2/internal/logic/agentcfg"
	"github.com/jose/matrix-v2/internal/middleware"
)

const codexAuthURL = "https://auth.openai.com/codex/device"

// codexAuthHandler implements AuthHandler for Codex agent.
// Supports: chatgpt (device-auth), openai-api-key (env_var), codex-api-key (env_var).
type codexAuthHandler struct {
	wizard *Wizard
}

func (h *codexAuthHandler) Methods(_ context.Context) ([]AuthMethod, error) {
	return []AuthMethod{
		{
			ID:          "chatgpt",
			Name:        "ChatGPT Login",
			Type:        "agent",
			Description: "Authenticate via ChatGPT subscription (device code flow)",
		},
		{
			ID:   "openai-api-key",
			Name: "OpenAI API Key",
			Type: "env_var",
			Vars: []string{"OPENAI_API_KEY"},
		},
		{
			ID:   "codex-api-key",
			Name: "Codex API Key",
			Type: "env_var",
			Vars: []string{"CODEX_API_KEY"},
		},
	}, nil
}

func (h *codexAuthHandler) Authenticate(ctx context.Context, method AuthMethod, input string) (*AuthResult, string, error) {
	switch method.ID {
	case "chatgpt":
		return h.authenticateDeviceAuth(ctx, input)
	case "openai-api-key":
		return h.authenticateAPIKey("OPENAI_API_KEY", input)
	case "codex-api-key":
		return h.authenticateAPIKey("CODEX_API_KEY", input)
	default:
		return nil, "", fmt.Errorf("unknown codex auth method: %s", method.ID)
	}
}

func (h *codexAuthHandler) authenticateAPIKey(envVar, input string) (*AuthResult, string, error) {
	if input == "" {
		return nil, fmt.Sprintf("Enter your %s:", envVar), nil
	}
	return &AuthResult{
		Env: map[string]string{envVar: input},
	}, "", nil
}

func (h *codexAuthHandler) authenticateDeviceAuth(_ context.Context, input string) (*AuthResult, string, error) {
	if input == "" {
		// First call: start device auth flow
		link, code, err := h.startDeviceAuth()
		if err != nil {
			return nil, "", fmt.Errorf("could not start Codex login: %w", err)
		}
		return nil, fmt.Sprintf("🔗 Open this URL: %s\n📝 Enter code: %s\n\nReply 'done' when complete.", link, code), nil
	}

	// User said "done" — check if codex actually authenticated
	if strings.EqualFold(input, "done") || strings.EqualFold(input, "fatto") {
		if h.isCodexAuthenticated() {
			// Codex stores its own auth token; no env vars needed
			return &AuthResult{}, "", nil
		}
		return nil, "Codex authentication not detected. Please try again or use an API key.", nil
	}

	return nil, "Reply 'done' when you have completed login in the browser.", nil
}

// --- Codex-specific helpers (kept from original wizard_codex.go) ---

func (w *Wizard) ensureCodexInstalled() (string, error) {
	if w.proc == nil || w.proc.HasExecutable(agentCodex) {
		return "", nil
	}

	ctx := context.Background()
	if err := w.installer.Install(ctx, agentCodex); err != nil {
		return "", fmt.Errorf("automated codex installation failed: %w", err)
	}

	return "📦 Codex installed automatically via ACP Registry.\n", nil
}

func (h *codexAuthHandler) isCodexAuthenticated() bool {
	w := h.wizard
	homeDir, err := w.fs.UserHomeDir()
	if err == nil {
		authPath := filepath.Join(homeDir, ".codex", "auth.json")
		if _, err := w.fs.Stat(authPath); err == nil {
			return true
		}
	}
	override, err := agentcfg.Load(w.storage, agentCodex)
	if err != nil {
		return false
	}
	for _, env := range override.Env {
		if strings.HasPrefix(env, "OPENAI_API_KEY=") {
			return true
		}
	}
	return false
}

func (h *codexAuthHandler) startDeviceAuth() (string, string, error) {
	w := h.wizard
	spec := middleware.CommandSpec{
		Runner: agentCodex,
		Args:   []string{"login", "--device-auth"},
		Env:    []string{"NO_COLOR=1", "TERM=dumb"},
	}
	pp, err := w.proc.StartPiped(spec)
	if err != nil {
		return "", "", fmt.Errorf("codex binary not found — install codex first: %w", err)
	}

	return h.waitForDeviceCode(pp)
}

func (h *codexAuthHandler) waitForDeviceCode(pp middleware.PipedProcess) (string, string, error) {
	type result struct {
		link string
		code string
	}
	ch := make(chan result, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	go func() {
		ch <- result{link: codexAuthURL, code: scanCodexDeviceCode(pp.Stdout())}
	}()

	select {
	case res := <-ch:
		if res.code == "" {
			return "", "", fmt.Errorf("codex did not output a device code (is codex properly installed?)")
		}
		go logCodexPipedProcess(pp)
		return res.link, res.code, nil
	case <-ctx.Done():
		_ = pp.Kill()
		// Drain the goroutine result so it doesn't leak
		go func() { <-ch }()
		return "", "", fmt.Errorf("timed out (15s) waiting for device code — check that codex is installed and can reach the internet")
	}
}

func scanCodexDeviceCode(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if code := findDeviceCode(stripANSI(scanner.Text())); code != "" {
			return code
		}
	}
	return ""
}

func findDeviceCode(line string) string {
	for _, field := range strings.Fields(line) {
		field = strings.Trim(field, ".,;\"'\r")
		if looksLikeDeviceCode(field) {
			return field
		}
	}
	return ""
}

func looksLikeDeviceCode(field string) bool {
	return len(field) >= 7 &&
		len(field) <= 12 &&
		strings.Contains(field, "-") &&
		field == strings.ToUpper(field) &&
		!strings.Contains(field, "http")
}

func logCodexPipedProcess(pp middleware.PipedProcess) {
	if err := pp.Wait(); err != nil {
		slog.Warn("codex auth process exited with error", "component", "onboarding_wizard", "event", "codex_auth_wait_failed", "error", err)
	}
}

var ansiEscape = strings.NewReplacer(
	"\x1b[0m", "", "\x1b[1m", "", "\x1b[2m", "", "\x1b[3m", "", "\x1b[4m", "",
	"\x1b[31m", "", "\x1b[32m", "", "\x1b[33m", "", "\x1b[34m", "", "\x1b[35m", "",
	"\x1b[36m", "", "\x1b[37m", "", "\x1b[90m", "", "\x1b[91m", "", "\x1b[92m", "",
	"\x1b[93m", "", "\x1b[94m", "", "\x1b[95m", "", "\x1b[96m", "", "\x1b[97m", "",
	"\x1b[1;32m", "", "\x1b[1;33m", "", "\x1b[1;34m", "", "\x1b[1;35m", "", "\x1b[1;36m", "",
	"\x1b[?25l", "", "\x1b[?25h", "", "\x1b[2J", "", "\x1b[H", "",
)

func stripANSI(s string) string {
	clean := ansiEscape.Replace(s)
	return removeEscapeSequences(clean)
}

func removeEscapeSequences(clean string) string {
	out := make([]byte, 0, len(clean))
	for i := 0; i < len(clean); i++ {
		if beginsEscapeSequence(clean, i) {
			i = skipEscapeSequence(clean, i)
			continue
		}
		if isPrintableOrWhitespace(clean[i]) {
			out = append(out, clean[i])
		}
	}
	return string(out)
}

func beginsEscapeSequence(s string, i int) bool {
	return s[i] == 0x1b && i+1 < len(s) && s[i+1] == '['
}

func skipEscapeSequence(s string, i int) int {
	i += 2
	for i < len(s) && !isEscapeTerminator(s[i]) {
		i++
	}
	return i
}

func isEscapeTerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isPrintableOrWhitespace(b byte) bool {
	return b >= 0x20 || b == '\n' || b == '\r' || b == '\t'
}
