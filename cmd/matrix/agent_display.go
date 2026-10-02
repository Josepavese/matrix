package main

import "github.com/Josepavese/matrix/internal/logic/agentcfg"

// The agent reports print a configuration, and a configuration carries the
// environment and the endpoint headers, which is where credentials live. The
// reports carry their names and counts unless the operator asked for the values
// with --reveal-values: this output reaches logs, shell history and shared
// screens, and a value printed there has left the vault for good. Doctor already
// speaks this way - it prints counts - and the endpoint report already prints
// header names; these three surfaces are the rest of that one rule.

// addAgentConfigReport fills the fields of an agent report that could carry
// values: the effective configuration, the override, and the environment the
// launch applies.
func addAgentConfigReport(payload map[string]any, cfg agentcfg.Config, override agentcfg.Override, revealValues bool) error {
	effective, err := agentcfg.DisplayConfig(cfg, revealValues)
	if err != nil {
		return err
	}
	shown, err := agentcfg.DisplayOverride(override, revealValues)
	if err != nil {
		return err
	}
	payload["effective"] = effective
	payload["override"] = shown
	payload["env_effect"] = agentcfg.DisplayEnv(cfg.Env, revealValues)
	return nil
}

// agentOverrideReport is the whole report `agent override show` prints.
func agentOverrideReport(agentID string, override agentcfg.Override, revealValues bool) (map[string]any, error) {
	shown, err := agentcfg.DisplayOverride(override, revealValues)
	if err != nil {
		return nil, err
	}
	return map[string]any{"agent_id": agentID, "override": shown}, nil
}

// agentEnvLines is what `agent env list` prints: the names the agent's override
// sets, one per line, and the entries themselves only on request.
func agentEnvLines(env []string, revealValues bool) []string {
	if revealValues {
		return env
	}
	return agentcfg.EnvNames(env)
}
