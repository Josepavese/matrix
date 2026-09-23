// Package bootstrap provides startup health-check reporting for the Matrix runtime.
package bootstrap

import (
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/Josepavese/matrix/internal/logic/channelcfg"
	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/logic/setupstate"
	"github.com/Josepavese/matrix/internal/middleware"
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
		"guide": BuildGuide(GuideInput{
			MatrixHTTPAddr:     configuredIngress(cfgMgr),
			SystemConfigured:   systemConfigured,
			TelegramEnabled:    tgCfg.Enabled,
			TelegramConfigured: tgCfg.Token != "",
			ActiveAgents:       activeAgents,
		}),
	}
	return report, nil
}

// BuildGuide returns setup guidance steps based on bootstrap state.
// GuideInput is what the first-run guide needs in order to describe this installation.
// It is a struct because the guide grew a fifth fact - the ingress address - and the
// governance manifest caps a function at four parameters, which is a rule worth keeping:
// a five-boolean call site is unreadable at the point of use anyway.
type GuideInput struct {
	MatrixHTTPAddr     string
	SystemConfigured   bool
	TelegramEnabled    bool
	TelegramConfigured bool
	ActiveAgents       []string
}

// BuildGuide returns the ordered first-run steps. The ingress address is used in the
// step that tells the operator where to POST: the guide named 127.0.0.1:9091
// unconditionally, so anyone who moved the port was sent to an address with nothing
// listening - which is how it was found.
func BuildGuide(in GuideInput) []string {
	matrixHTTPAddr := in.MatrixHTTPAddr
	systemConfigured := in.SystemConfigured
	telegramEnabled := in.TelegramEnabled
	telegramConfigured := in.TelegramConfigured
	activeAgents := in.ActiveAgents
	steps := []string{
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
		steps = append(steps, "First-run onboarding is not complete yet: start `matrix run` and complete setup through an interactive channel, or for a headless provisioned install run `matrix vault set system.configured true` before sending non-interactive `/v1/runs` traffic.")
	}
	steps = append(steps,
		"Run `matrix doctor` before starting the daemon if you want a full local health snapshot.",
		"Start the runtime with `matrix run`.",
		"Validate the path end-to-end with `matrix doctor` and, if needed, a POST to `http://"+ingressAddress(matrixHTTPAddr)+"/v1/runs`.",
	)
	return steps
}

// configuredIngress reads the address the runtime will actually bind.
func configuredIngress(cfgMgr *config.Manager) string {
	if cfgMgr == nil {
		return ""
	}
	return cfgMgr.GetWithDefault("matrix_http_addr", "")
}

// ingressAddress falls back to the shipped default so a caller that has no configured
// address still produces a usable step.
func ingressAddress(configured string) string {
	if addr := strings.TrimSpace(configured); addr != "" {
		return addr
	}
	return "127.0.0.1:9091"
}

func readConfigured(store middleware.Storage) bool {
	data, err := store.Get("system.configured")
	if err != nil || len(data) == 0 {
		return false
	}
	return setupstate.Configured(data)
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
