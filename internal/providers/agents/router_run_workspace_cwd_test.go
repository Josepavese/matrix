package agents

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/onboarding"
	"github.com/Josepavese/matrix/internal/logic/session"
	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
)

// runWorkspaceLocalizer is the minimal wizard localization reader. The run router
// is exercised with configured storage, so no wizard string is ever rendered and
// returning the key is an honest answer rather than a fake translation.
type runWorkspaceLocalizer struct{}

func (runWorkspaceLocalizer) GetString(_, key string) string { return key }

// TestRunWorkspaceReachesTheAgentChildCwd follows the workspace of a run from the
// request that names it down to the process Matrix starts for the agent.
//
// The other child-cwd tests in this package hand the router a workspace path
// directly, so they pin the router half of the contract: a child starts where
// the router was told. What none of them can see is whether the workspace of the
// run ever arrives at the router. It travels through the session the run creates
// and the request that session builds, and losing it there is silent: every run
// would start its agent in the daemon's own directory while the suite stayed
// green. This test starts where a run starts — the workspace the runtime API was
// asked for — and asserts the child process reports that directory as its own.
func TestRunWorkspaceReachesTheAgentChildCwd(t *testing.T) {
	runWorkspace := t.TempDir()
	prepareCwdProbe(t, runWorkspace)

	const workspaceID = "workspace-of-the-run"
	storage := memstore.New()
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("marking the system configured: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{ID: workspaceID, RootPath: runWorkspace}); err != nil {
		t.Fatalf("registering the run workspace: %v", err)
	}

	router := newCwdProbeRouter()
	defer router.Close()
	manager := session.NewManager(storage, router, onboarding.NewWizard(onboarding.WizardDependencies{
		Storage:   storage,
		Localizer: runWorkspaceLocalizer{},
	}), nil)

	// The run request as the runtime API builds it: the workspace the run asked
	// for and its input. Everything else — the session, the agent, and the
	// directory the child is started in — is the chain this test follows.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := manager.RouteConversation(ctx, middleware.ConversationRequest{
		ChannelID:     "run-workspace-cwd",
		WorkspaceID:   workspaceID,
		WorkspacePath: runWorkspace,
		Input:         "where are you?",
	}); err != nil {
		t.Fatalf("routing the run on %s: %v", runWorkspace, err)
	}

	expected, err := filepath.EvalSymlinks(runWorkspace)
	if err != nil {
		t.Fatalf("the run workspace %q does not resolve: %v", runWorkspace, err)
	}
	if cwd := readChildCwd(t, runWorkspace); cwd != expected {
		t.Fatalf("the agent child of the run started in %s, want the run workspace %s", cwd, expected)
	}
}
