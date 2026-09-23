package main

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/providers/agents"
	"github.com/Josepavese/matrix/internal/providers/matrixapi"
)

// configureAgentCapabilities wires the ACP client capabilities that are opt-in.
//
// Both are off unless an operator turns them on, because advertising either one
// changes what a real agent does. Elicitation decides where codex-acp routes
// MCP-server input requests; capabilities.auth.terminal tells a v2 agent that
// Matrix can reproduce the agent's own invocation to complete a login. Keeping
// the default off leaves every installed agent on exactly the surface it had.
// The elicitation service is returned because the HTTP API and the channel
// runtime must share the one instance that answers pending questions.
func configureAgentCapabilities(router *agents.Router, get func(key, fallback string) string, log *slog.Logger) *elicitation.Service {
	var service *elicitation.Service
	if elicitationEnabled(get) {
		service = elicitation.NewService(elicitTimeout(get))
		router.SetElicitationFrontend(service)
		log.Info("agent elicitation enabled", "event", "elicitation_enabled", "http_path", matrixapi.ElicitationPathV1, "timeout_key", "agent.elicitation_timeout_seconds")
	} else {
		log.Info("agent elicitation disabled", "event", "elicitation_disabled", "enable_with", "matrix config set agent.elicitation_enabled true")
	}
	if terminalAuthEnabled(get) {
		router.SetTerminalAuth(true)
		log.Info("agent terminal authentication enabled", "event", "acp_terminal_auth_enabled")
	} else {
		log.Info("agent terminal authentication disabled", "event", "acp_terminal_auth_disabled", "enable_with", "matrix config set agent.terminal_auth_enabled true")
	}
	return service
}

// elicitationEnabled reads agent.elicitation_enabled. Elicitation is off
// unless the operator opts in, because advertising the capability changes how
// conforming agents (codex-acp among them) route input requests.
func elicitationEnabled(get func(key, fallback string) string) bool {
	return strings.EqualFold(strings.TrimSpace(get("agent.elicitation_enabled", "false")), "true")
}

// terminalAuthEnabled reads agent.terminal_auth_enabled. ACP v2 terminal
// authentication is off unless the operator opts in: advertising
// capabilities.auth.terminal changes the authentication methods a real agent
// offers, and completing one runs the agent program interactively rather than
// inside the already-running connection.
func terminalAuthEnabled(get func(key, fallback string) string) bool {
	return strings.EqualFold(strings.TrimSpace(get("agent.terminal_auth_enabled", "false")), "true")
}

// elicitTimeout reads agent.elicitation_timeout_seconds through the given
// getter. Zero or invalid keeps the elicitation service default, so a bad
// config value degrades to the safe bounded window instead of failing startup.
func elicitTimeout(get func(key, fallback string) string) time.Duration {
	seconds, err := strconv.Atoi(get("agent.elicitation_timeout_seconds", "0"))
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
