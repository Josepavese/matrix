package agentcfg

import "testing"

func TestNormalizeEndpointPreservesEnvironmentIsolation(t *testing.T) {
	endpoint := NormalizeEndpoint(Config{Kind: "acp", Transport: "stdio", Command: "codex-acp", EnvIsolation: true})
	if !endpoint.EnvIsolation {
		t.Fatal("normalized endpoint lost env isolation launch policy")
	}
}
