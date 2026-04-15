package onboarding

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// openrouterAuthHandler implements AuthHandler for OpenRouter (used by opencode agent).
// Supports: api_key (direct key), quick_login (OAuth PKCE).
type openrouterAuthHandler struct {
	wizard *Wizard
}

func (h *openrouterAuthHandler) Methods(_ context.Context) ([]AuthMethod, error) {
	return []AuthMethod{
		{
			ID:          "api_key",
			Name:        "API Key",
			Type:        "env_var",
			Vars:        []string{"OPENROUTER_API_KEY"},
			Description: "Enter your OpenRouter API key directly",
		},
		{
			ID:          "quick_login",
			Name:        "Quick Login (OAuth)",
			Type:        "agent",
			Description: "Authenticate via OpenRouter OAuth in your browser",
		},
	}, nil
}

func (h *openrouterAuthHandler) Authenticate(ctx context.Context, method AuthMethod, input string) (*AuthResult, string, error) {
	switch method.ID {
	case "api_key":
		return h.authenticateAPIKey(ctx, input)
	case "quick_login":
		return h.authenticateOAuth(ctx, input)
	default:
		return nil, "", fmt.Errorf("unknown openrouter auth method: %s", method.ID)
	}
}

func (h *openrouterAuthHandler) authenticateAPIKey(_ context.Context, input string) (*AuthResult, string, error) {
	if input == "" {
		return nil, "Enter your OpenRouter API key:", nil
	}
	return &AuthResult{
		Env: map[string]string{"OPENROUTER_API_KEY": input},
	}, "", nil
}

func (h *openrouterAuthHandler) authenticateOAuth(_ context.Context, input string) (*AuthResult, string, error) {
	if input == "" {
		// Generate auth URL — the verifier is stored in wizard state by the caller
		return nil, "OAUTH_URL_NEEDED", nil
	}
	return nil, "", fmt.Errorf("use HandleAuthCallback for OAuth flow completion")
}

// generateOpenRouterAuthURL creates an OAuth PKCE URL for OpenRouter.
// The state parameter includes an HMAC to prevent CSRF attacks.
func (h *openrouterAuthHandler) generateAuthURL(channelID string) (string, string, error) {
	verifier := generateRandomString(64)
	challenge := buildPKCEChallenge(verifier)
	publicIP := h.wizard.getPublicIP()
	if publicIP == "" {
		publicIP = "your-server-ip"
	}

	callbackURL := fmt.Sprintf("http://%s:9091/v1/auth/openrouter/callback", publicIP)
	csrfState := h.wizard.generateCSRFState(channelID, verifier)
	authURL := fmt.Sprintf(
		"https://openrouter.ai/auth?callback_url=%s&code_challenge=%s&code_challenge_method=S256&state=%s",
		callbackURL,
		challenge,
		csrfState,
	)
	return authURL, verifier, nil
}

// HandleAuthCallback handles the OAuth callback from OpenRouter.
// The channelID parameter is the raw state value from the callback URL,
// which includes an HMAC token for CSRF validation.
func (w *Wizard) HandleAuthCallback(channelID, _ string, code string) (string, error) {
	// channelID here is the CSRF state from the OAuth callback.
	// The actual channel ID and PKCE verifier are stored in wizard state.
	// We validate the CSRF token against the stored verifier.
	state, stateKey, err := w.loadWizardStateForCSRF(channelID)
	if err != nil {
		return "", err
	}
	if state.Context["auth_method"] != "quick_login" {
		return "", fmt.Errorf("wizard not in quick_login state")
	}

	verifier := state.Context["pkce_verifier"]
	if verifier == "" {
		return "", fmt.Errorf("pkce verifier not found")
	}

	// Verify CSRF state matches
	if !w.verifyCSRFState(channelID, verifier) {
		return "", fmt.Errorf("invalid CSRF state — possible tampering")
	}

	apiKey, err := w.exchangeOpenRouterCode(code, verifier)
	if err != nil {
		return "", fmt.Errorf("failed to exchange code: %w", err)
	}

	state.Context["api_key"] = apiKey
	state.Step = 5
	w.saveState(stateKey, state)
	return "✅ Authentication successful! Please go back to your chat and type 'done' to continue.", nil
}

func (w *Wizard) loadWizardState(channelID string) (WizardState, string, error) {
	stateKey := "wizard.state." + channelID
	data, err := w.storage.Get(stateKey)
	if err != nil || len(data) == 0 {
		return WizardState{}, "", fmt.Errorf("wizard state not found for channel %s", channelID)
	}

	var state WizardState
	if err := json.Unmarshal(data, &state); err != nil {
		return WizardState{}, "", fmt.Errorf("failed to unmarshal wizard state: %w", err)
	}
	return state, stateKey, nil
}

func (w *Wizard) exchangeOpenRouterCode(code, verifier string) (string, error) {
	body := map[string]string{
		"code":          code,
		"code_verifier": verifier,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data, statusCode, err := w.net.PostJSON(ctx, "https://openrouter.ai/api/v1/auth/keys", body)
	if err != nil {
		return "", err
	}
	if statusCode != 200 {
		return "", fmt.Errorf("openrouter returned status %d: %s", statusCode, string(data))
	}

	var result struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	return result.Key, nil
}

// --- Utility functions ---

func buildPKCEChallenge(verifier string) string {
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	b := make([]byte, length)
	randBytes := make([]byte, length)
	if _, err := rand.Read(randBytes); err != nil {
		// crypto/rand should never fail on modern systems; if it does,
		// the PKCE flow cannot proceed securely — panic is appropriate.
		panic("crypto/rand failed: " + err.Error())
	}
	for i := range b {
		b[i] = charset[int(randBytes[i])%len(charset)]
	}
	return string(b)
}

func (w *Wizard) getPublicIP() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := w.net.Fetch(ctx, "https://api.ipify.org")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// --- CSRF State Protection ---

// csrfHMACKey is a per-session key stored in the vault to bind OAuth state tokens.
const csrfHMACKeyVaultKey = "system.oauth_csrf_key"

// generateCSRFState creates a state parameter that binds the channelID to the PKCE verifier
// using an HMAC. Format: <channelID>.<hmac_hex>
func (w *Wizard) generateCSRFState(channelID, verifier string) string {
	key := w.getOrCreateCSRFKey()
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(channelID))
	mac.Write([]byte(verifier))
	sig := hex.EncodeToString(mac.Sum(nil))
	return channelID + "." + sig
}

// verifyCSRFState checks that the state parameter was generated by this server.
func (w *Wizard) verifyCSRFState(state, verifier string) bool {
	// Extract channelID from state (format: <channelID>.<hmac_hex>)
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return false
	}
	channelID := parts[0]

	key := w.getOrCreateCSRFKey()
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(channelID))
	mac.Write([]byte(verifier))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}

// getOrCreateCSRFKey returns the HMAC key for CSRF state, creating one if needed.
func (w *Wizard) getOrCreateCSRFKey() []byte {
	data, err := w.storage.Get(csrfHMACKeyVaultKey)
	if err == nil && len(data) > 0 {
		var key []byte
		if json.Unmarshal(data, &key) == nil && len(key) == 32 {
			return key
		}
	}

	// Generate new key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		// Should never happen with crypto/rand
		panic("crypto/rand failed: " + err.Error())
	}
	keyData, _ := json.Marshal(key)
	_ = w.storage.Set(csrfHMACKeyVaultKey, keyData)
	return key
}

// loadWizardStateForCSRF loads the wizard state by extracting the channelID from the CSRF state.
func (w *Wizard) loadWizardStateForCSRF(csrfState string) (WizardState, string, error) {
	channelID, _, ok := strings.Cut(csrfState, ".")
	if !ok {
		return WizardState{}, "", fmt.Errorf("invalid OAuth state format")
	}
	return w.loadWizardState(channelID)
}
