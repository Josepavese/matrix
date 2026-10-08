package agentdoctor

import (
	"context"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

func TestContainerDoctorNeverRunsItsCommandOnHost(t *testing.T) {
	called := false
	endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: "host-command-must-not-run", Env: []string{agentlaunch.SandboxEnv + `={"container":{"engine":"docker","image":"cached","workspace_access":"read-only","network":"none","state_dir":"explicit","user":"1000:1000","memory_bytes":67108864,"cpus":1,"pids":32}}`}}
	result, warnings := InspectACP(endpoint, func(context.Context, middleware.ProtocolEndpoint) error { called = true; return nil })
	if called || result["provider_status"] != "sandbox_requires_workspace_probe" || len(warnings) == 0 {
		t.Fatal("container doctor used host authority", result)
	}
}
