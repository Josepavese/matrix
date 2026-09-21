package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/providers/agents"
	execprov "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// TestSmoke_RealACPAgentToleratesElicitationCapability is the interoperability
// regression guard for the elicitation rollout: a real ACP agent must be able
// to negotiate a session while the client advertises stable elicitation
// capabilities it did not advertise before. A real peer that rejected the new
// capability object would break every Matrix conversation, not just this
// feature, so this is checked against the real binary rather than a mock.
//
// It also observes, without asserting, whether the real agent ever asks for
// user input through elicitation/create, which is the honest measure of how
// much of the ecosystem implements the stable surface today.
func TestSmoke_RealACPAgentToleratesElicitationCapability(t *testing.T) {
	requireSmokeTest(t)

	specs := realACPProviderSpecs(t)
	if len(specs) == 0 {
		t.Skip("no real ACP provider available")
	}
	for _, spec := range specs {
		t.Run(spec.name, func(t *testing.T) {
			probeRealACPElicitationNegotiation(t, spec)
		})
	}
}

func probeRealACPElicitationNegotiation(t *testing.T, spec realACPProviderSpec) {
	t.Helper()
	workspace := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	service := elicitation.NewService(20 * time.Second)
	args := append([]string{}, spec.args...)
	if spec.name == "opencode" {
		args = append(args, "--cwd", workspace)
	}
	transport, err := zedacp.NewStdioTransport(ctx, spec.bin, spec.env, args...)
	if err != nil {
		t.Fatalf("start provider %s: %v", spec.name, err)
	}
	client := zedacp.NewClient(ctx, transport)
	defer client.Close()

	handler := agents.NewDefaultRequestHandler(func() bool { return true }).
		WithFS(osfs.NewFSProvider(), workspace).
		WithProcess(execprov.NewProvider()).
		WithElicitationFrontend(service).
		WithAgentIdentity(spec.name)
	counter := newCountingACPHandler(handler)
	client.SetRequestHandler(counter)

	initResp, err := client.Initialize(ctx, zedacp.InitializeRequest{
		ProtocolVersion: 1,
		ClientInfo:      map[string]interface{}{"name": "matrix-elicitation-interop", "version": "0.0.0-test"},
		ClientCapabilities: &zedacp.ClientCapabilities{
			Fs:       &zedacp.FsCapability{ReadTextFile: true, WriteTextFile: true},
			Terminal: true,
			Session: &zedacp.ClientSessionCapabilities{
				ConfigOptions: &zedacp.SessionConfigOptionsCapabilities{Boolean: &zedacp.BooleanConfigOptionCapabilities{}},
			},
			Elicitation: &zedacp.ElicitationCapabilities{
				Form: &zedacp.ElicitationModeCapability{},
				URL:  &zedacp.ElicitationModeCapability{},
			},
		},
	})
	if err != nil {
		t.Fatalf("provider %s rejected a client advertising elicitation: %v", spec.name, err)
	}
	if initResp.ProtocolVersion != 1 {
		t.Fatalf("provider %s negotiated unexpected protocol version %d", spec.name, initResp.ProtocolVersion)
	}
	t.Logf("provider=%s negotiated protocol=%d with elicitation advertised; agent capabilities=%v",
		spec.name, initResp.ProtocolVersion, initResp.Capabilities)

	session, err := client.NewSession(ctx, zedacp.NewSessionRequest{Cwd: workspace, McpServers: []zedacp.McpServerConfig{}})
	if err != nil {
		t.Fatalf("provider %s failed session/new with elicitation advertised: %v", spec.name, err)
	}
	if session.SessionID == "" {
		t.Fatalf("provider %s returned empty session id", spec.name)
	}

	// Cancellation keeps the probe free of model credentials while still
	// exercising the live session end to end.
	if err := client.CancelSession(ctx, session.SessionID); err != nil {
		t.Logf("provider=%s session/cancel returned: %v", spec.name, err)
	}
	if supportsSessionCapability(initResp.Capabilities, "close") {
		if err := client.CloseSession(ctx, session.SessionID); err != nil {
			t.Logf("provider=%s session/close returned: %v", spec.name, err)
		}
	}

	asked := counter.Calls()["elicitation/create"]
	pending := service.Pending()
	t.Logf("provider=%s elicitation_requests=%d pending=%d", spec.name, asked, len(pending))
	if asked == 0 {
		t.Logf("provider=%s did not request user input during the probe; elicitation remains client-side ready but unexercised by this peer", spec.name)
	}
	for _, entry := range pending {
		if entry.Request.AgentID != spec.name {
			t.Fatalf("provider %s elicitation lost agent identity: %+v", spec.name, entry.Request)
		}
	}
}
