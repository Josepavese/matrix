package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentinstall"
	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/agents"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

type installedCodexResolver struct {
	endpoint middleware.ProtocolEndpoint
}

func (r installedCodexResolver) GetAgentEndpoint(string) (middleware.ProtocolEndpoint, error) {
	return r.endpoint, nil
}

func TestSmoke_InstalledCanonicalCodexAppliesFullAccessPolicy(t *testing.T) {
	if os.Getenv("MATRIX_CODEX_POLICY_REAL") != "1" {
		t.Skip("Set MATRIX_CODEX_POLICY_REAL=1 to install and probe canonical Codex ACP.")
	}
	packageRef := os.Getenv("MATRIX_CODEX_POLICY_PACKAGE")
	if packageRef == "" {
		packageRef = "@agentclientprotocol/codex-acp@1.1.9"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cfg, err := agentinstall.InstallCanonicalCodex(ctx, agentinstall.Config{
		FS:      osfs.NewFSProvider(),
		Process: execprovider.NewProvider(),
		Target:  filepath.Join(t.TempDir(), "codex"),
		Package: packageRef,
	})
	if err != nil {
		t.Fatalf("install canonical Codex: %v", err)
	}
	// A clean CI runner has no Codex login. Authenticate the local app-server
	// with a non-secret dummy key in an isolated Codex home; session/new does not
	// issue a model request, but it does expose the provider's effective mode.
	cfg.Env = append(cfg.Env,
		"CODEX_HOME="+t.TempDir(),
		"CODEX_API_KEY=matrix-policy-smoke-dummy",
		`DEFAULT_AUTH_REQUEST={"methodId":"api-key"}`,
	)
	cfg.Args = append(cfg.Args,
		"-c", "sandbox_mode=danger-full-access",
		"-c", "approval_policy=never",
	)
	resolved, err := agentlaunch.ResolveEndpoint("codex", agentcfg.NormalizeEndpoint(cfg))
	if err != nil {
		t.Fatalf("resolve installed policy: %v", err)
	}
	if resolved.Metadata["verified"] != true || resolved.Metadata["trusted_terminal"] != true {
		t.Fatalf("installed policy not verified: %#v", resolved.Metadata)
	}

	router := agents.NewRouter(installedCodexResolver{endpoint: agentcfg.NormalizeEndpoint(cfg)})
	defer router.Close()
	info, _, err := router.MaterializeAgentSession(ctx, "codex", middleware.SessionMaterializeRequest{
		LogicalSessionID: "canonical-codex-policy-smoke",
		WorkspacePath:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("canonical Codex session policy probe: %v", err)
	}
	if info.RemoteSessionID == "" {
		t.Fatal("canonical Codex returned empty session id")
	}
}
