package agentcfg

import (
	"encoding/json"
	"sort"
	"strings"
)

// EnvNames returns the names an environment list sets, sorted and without
// duplicates, and never a value: a name is what a report can carry without
// turning a screen, a log line or a shell history entry into a leak.
func EnvNames(env []string) []string {
	seen := make(map[string]struct{}, len(env))
	names := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DisplayEnv is the display form of an environment list: the names it sets and
// how many entries it holds, plus the entries themselves only when the caller
// says the values were asked for.
func DisplayEnv(env []string, revealValues bool) map[string]any {
	out := map[string]any{"env_names": EnvNames(env), "env_count": len(env)}
	if revealValues {
		out["env"] = env
	}
	return out
}

// DisplayConfig is the display form of an agent configuration: every field as it
// is, with the environment and the endpoint headers - the two that hold values -
// reduced to names and counts. A caller that was told to show values gets the
// raw fields back as well.
func DisplayConfig(cfg Config, revealValues bool) (map[string]any, error) {
	out, err := displayMap(cfg)
	if err != nil {
		return nil, err
	}
	out["env_names"] = EnvNames(cfg.Env)
	out["env_count"] = len(cfg.Env)
	out["header_names"] = HeaderNames(cfg.Headers)
	out["header_count"] = len(cfg.Headers)
	if !revealValues {
		delete(out, "env")
		delete(out, "headers")
	}
	return out, nil
}

// DisplayOverride applies the same rule to the override part of an agent entry.
func DisplayOverride(override Override, revealValues bool) (map[string]any, error) {
	out, err := displayMap(override)
	if err != nil {
		return nil, err
	}
	out["env_names"] = EnvNames(override.Env)
	out["env_count"] = len(override.Env)
	if !revealValues {
		delete(out, "env")
	}
	return out, nil
}

// displayMap is the shared step: a configuration's own JSON as a map, so a
// report adds the names and counts to what the type already says instead of
// restating every field it has. A field added to a configuration is then carried
// by every report, and the fields that hold values are the only ones this file
// has to know about.
func displayMap(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil, err
	}
	return out, nil
}
