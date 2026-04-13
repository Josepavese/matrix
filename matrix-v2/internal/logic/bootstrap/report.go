package bootstrap

import (
	"encoding/json"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/agentmgr"
	"github.com/jose/matrix-v2/internal/logic/channelcfg"
	"github.com/jose/matrix-v2/internal/logic/config"
	"github.com/jose/matrix-v2/internal/middleware"
)

// BuildReport generates a bootstrap readiness report.
func BuildReport(store middleware.Storage, cfgMgr *config.Manager, registry *agentmgr.Registry, cfgReader middleware.ConfigReader) (map[string]any, error) {
	tgCfg, tgSource, err := channelcfg.LoadTelegramConfig(cfgReader, cfgMgr)
	if err != nil {
		return nil, err
	}

	systemConfigured := readConfigured(store)
	activeAgents := activeAgentIDs(registry)
	report := map[string]any{
		"system_configured":   systemConfigured,
		"telegram_enabled":    tgCfg.Enabled,
		"telegram_configured": tgCfg.Token != "",
		"telegram_source":     tgSource,
		"active_agents":       activeAgents,
		"can_run":             len(activeAgents) > 0,
		"guide":               BuildGuide(systemConfigured, tgCfg.Enabled, tgCfg.Token != "", activeAgents),
	}
	return report, nil
}

// BuildGuide returns setup guidance steps based on bootstrap state.
func BuildGuide(systemConfigured, telegramEnabled, telegramConfigured bool, activeAgents []string) []string {
	steps := []string{
		"Build the binary with `go build -o matrix ./cmd/matrix` or use `go run ./cmd/matrix ...`.",
		"Inspect the current bootstrap state with `matrix bootstrap doctor`.",
	}
	if len(activeAgents) == 0 {
		steps = append(steps, "Enable at least one agent with `matrix agent enable <agent_id>`.")
	} else {
		steps = append(steps, "Active agents detected: "+strings.Join(activeAgents, ", ")+".")
	}
	if telegramEnabled && !telegramConfigured {
		steps = append(steps, "Telegram is enabled but not configured: set the token with `printf '...' | matrix channel set telegram token --stdin` or use env overrides.")
	}
	if !telegramEnabled {
		steps = append(steps, "Telegram is optional; leave it disabled unless you want a chat gateway.")
	}
	if !systemConfigured {
		steps = append(steps, "First-run onboarding is not complete yet: start `matrix run` and send the first message through a channel or `/runs`.")
	}
	steps = append(steps,
		"Run `matrix doctor` before starting the daemon if you want a full local health snapshot.",
		"Start the runtime with `matrix run`.",
		"Validate the path end-to-end with `matrix doctor` and, if needed, a POST to `http://127.0.0.1:9091/runs`.",
	)
	return steps
}

func readConfigured(store middleware.Storage) bool {
	data, err := store.Get("system.configured")
	if err != nil || len(data) == 0 {
		return false
	}
	var configured bool
	return json.Unmarshal(data, &configured) == nil && configured
}

func activeAgentIDs(registry *agentmgr.Registry) []string {
	ids := []string{}
	for _, id := range registry.IDs() {
		cfg, err := registry.Get(id)
		if err != nil {
			continue
		}
		if cfg.IsActive() {
			ids = append(ids, id)
		}
	}
	return ids
}
