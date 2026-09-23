package zedacp

import "encoding/json"

type NewSessionRequest struct {
	ClientTitle           string                 `json:"clientTitle,omitempty"`
	Cwd                   string                 `json:"cwd"`
	AdditionalDirectories []string               `json:"additionalDirectories,omitempty"`
	McpServers            []McpServerConfig      `json:"mcpServers"`
	Tools                 []Tool                 `json:"tools,omitempty"`
	Meta                  map[string]interface{} `json:"_meta,omitempty"`
}

func (r NewSessionRequest) MarshalJSON() ([]byte, error) {
	type wireRequest NewSessionRequest
	out := wireRequest(r)
	out.McpServers = nonNil(r.McpServers)
	return json.Marshal(out)
}

type McpServerConfig struct {
	Name    string   `json:"name"`
	Type    string   `json:"type,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []EnvVar `json:"env,omitempty"`
	URL     string   `json:"url,omitempty"`
	Headers []Header `json:"headers,omitempty"`
}

func (c McpServerConfig) MarshalJSON() ([]byte, error) {
	out := map[string]interface{}{"name": c.Name}
	switch c.Type {
	case "http", "sse":
		out["type"] = c.Type
		out["url"] = c.URL
		out["headers"] = nonNil(c.Headers)
	default:
		if c.Type != "" && c.Type != "stdio" {
			out["type"] = c.Type
		}
		out["command"] = c.Command
		out["args"] = nonNil(c.Args)
		out["env"] = nonNil(c.Env)
	}
	return json.Marshal(out)
}

// nonNil emits an empty slice where a nil one was supplied, because these
// fields are required on the wire in both generations: an absent array is not
// the same as an empty one to the agents that read them.
func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}
