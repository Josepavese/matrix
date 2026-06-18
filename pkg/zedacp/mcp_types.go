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
	out.McpServers = nonNilMCPServers(r.McpServers)
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
		out["headers"] = nonNilHeaders(c.Headers)
	default:
		if c.Type != "" && c.Type != "stdio" {
			out["type"] = c.Type
		}
		out["command"] = c.Command
		out["args"] = nonNilStrings(c.Args)
		out["env"] = nonNilEnvVars(c.Env)
	}
	return json.Marshal(out)
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nonNilEnvVars(values []EnvVar) []EnvVar {
	if values == nil {
		return []EnvVar{}
	}
	return values
}

func nonNilHeaders(values []Header) []Header {
	if values == nil {
		return []Header{}
	}
	return values
}

func nonNilMCPServers(values []McpServerConfig) []McpServerConfig {
	if values == nil {
		return []McpServerConfig{}
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
