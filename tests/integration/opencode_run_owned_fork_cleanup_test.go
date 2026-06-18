package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/onboarding"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/session"
	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/agents"
	execprov "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/matrixapi"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

type staticLocalizer struct{}

func (staticLocalizer) GetString(_, key string) string { return key }

func TestOpenCode_RunTimeoutCleanupThenImmediateJudgeRun_DoesNotPreflightFail(t *testing.T) {
	requireSmokeTest(t)
	if os.Getenv("MATRIX_OPENCODE_ACP_STDIO") != "1" {
		t.Skip("Set MATRIX_OPENCODE_ACP_STDIO=1 to run the real OpenCode ACP cancellation recovery smoke.")
	}
	opencodePath := lookPath(t, "opencode")
	workspaceA := t.TempDir()
	workspaceB := t.TempDir()
	writeFile(t, workspaceA+"/README.md", "matrix cancellation workspace A\n")
	writeFile(t, workspaceB+"/README.md", "matrix cancellation workspace B\n")
	beforeProcesses := opencodeACPProcesses(t)

	store := memstore.New()
	if err := store.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("set configured flag: %v", err)
	}
	router := agents.NewRouter(&realAgentResolver{
		protocol: "stdio",
		bin:      opencodePath,
		args:     []string{"acp", "--pure"},
	})
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(execprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), workspaceA)
	mgr := session.NewManager(store, router, onboarding.NewWizard(onboarding.WizardDependencies{
		Storage:   store,
		Localizer: staticLocalizer{},
	}), nil)

	mux := http.NewServeMux()
	api := matrixapi.NewServer(mgr).WithTraceStorage(store)
	api.RegisterRoutes(mux)
	traceStore := runtrace.NewStore(store)
	server := httptest.NewServer(mux)
	defer server.Close()

	const policy = middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal
	asyncRun := httpRun(t, server.URL, map[string]interface{}{
		"channel_id":     "smoke.opencode.cancel-recovery-a",
		"agent_id":       "opencode",
		"workspace_path": workspaceA,
		"execution_mode": "async",
		"input": "This is a Matrix cancellation recovery test. Start working, then run a slow command like " +
			"`python3 -c \"import time; time.sleep(90)\"`, and do not give the final answer until the command is done.",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": policy,
	})
	runID, _ := asyncRun["run_id"].(string)
	if runID == "" {
		t.Fatalf("async run response missing run_id: %+v", asyncRun)
	}
	if remoteID := waitForRunRemoteSessionID(t, traceStore, runID, 90*time.Second); remoteID == "" {
		t.Fatalf("async run did not expose remote session id")
	}
	time.Sleep(2 * time.Second)
	httpRunAction(t, server.URL, runID, map[string]interface{}{
		"action": "cancel",
		"reason": "matrix cancellation recovery smoke",
	})
	cleanup := waitForRunCleanup(t, traceStore, runID, 2*time.Minute)
	assertStrongRunCleanup(t, cleanup)
	cancelled := waitForRunTerminal(t, traceStore, runID, 2*time.Minute)
	if cancelled.Status != runtrace.StatusCancelled {
		t.Fatalf("expected cancelled async run, got %+v", cancelled)
	}
	assertRunTraceHasNoPreflightPoison(t, traceStore, runID)

	for _, workspace := range []string{workspaceB, workspaceA} {
		judge := httpRun(t, server.URL, map[string]interface{}{
			"channel_id":     "smoke.opencode.cancel-recovery-judge-" + filepath.Base(workspace),
			"agent_id":       "opencode",
			"workspace_path": workspace,
			"input":          "Reply exactly MATRIX_JUDGE_OK. Do not edit files.",
		})
		if output, _ := judge["output"].(string); !strings.Contains(output, "MATRIX_JUDGE_OK") {
			t.Fatalf("judge-like request did not prove prompt processing for %s, output=%q", workspace, output)
		}
		judgeRunID, _ := judge["run_id"].(string)
		assertRunTraceHasNoPreflightPoison(t, traceStore, judgeRunID)
	}

	router.Close()
	waitForNoNewOpenCodeACPProcesses(t, beforeProcesses, 10*time.Second)
}

func TestOpenCodeRunOwnedForkChildrenCleanupRemediatesParentOwnerLikeNoemaActiveResume(t *testing.T) {
	requireSmokeTest(t)
	if os.Getenv("MATRIX_OPENCODE_ACP_STDIO") != "1" {
		t.Skip("Set MATRIX_OPENCODE_ACP_STDIO=1 to run the real OpenCode ACP fork cleanup smoke.")
	}
	opencodePath := lookPath(t, "opencode")
	workspace := t.TempDir()
	writeFile(t, workspace+"/README.md", "matrix run-owned fork cleanup smoke\n")
	beforeProcesses := opencodeACPProcesses(t)

	store := memstore.New()
	if err := store.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("set configured flag: %v", err)
	}
	router := agents.NewRouter(&realAgentResolver{
		protocol: "stdio",
		bin:      opencodePath,
		args:     []string{"acp", "--pure"},
	})
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(execprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), workspace)

	mgr := session.NewManager(store, router, onboarding.NewWizard(onboarding.WizardDependencies{
		Storage:   store,
		Localizer: staticLocalizer{},
	}), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	const channelID = "smoke.opencode.run-owned-fork-cleanup"
	policy := middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal

	parent := createRunOwnedOpenCodeParent(ctx, t, mgr, channelID, workspace, policy)
	routeParentOpenCodeSession(ctx, t, mgr, channelID, workspace, parent)

	for i := 1; i <= 2; i++ {
		result := runOpenCodeForkArtifactCleanup(ctx, t, mgr, channelID, workspace, parent, policy, i)
		assertStrongForkCleanup(t, result.Fork.Cleanup, parent)
		if result.ActiveSessionID != parent || !result.Fork.ParentRestored {
			t.Fatalf("fork child cleanup must restore the run-owned parent until final cleanup, got active=%q fork=%+v", result.ActiveSessionID, result.Fork)
		}
	}

	parentCleanup := cleanupOpenCodeSession(ctx, t, mgr, channelID, parent, policy)
	if !parentCleanup.Clean || !parentCleanup.StrongCleanup || parentCleanup.CleanupStrength != sessioncleanup.StrengthStrong ||
		parentCleanup.ProcessRetained || parentCleanup.FailureCode != "" {
		t.Fatalf("parent cleanup must be strong and non-retained, got %+v", parentCleanup)
	}

	router.Close()
	waitForNoNewOpenCodeACPProcesses(t, beforeProcesses, 10*time.Second)
}

func TestOpenCodeHTTPStandaloneForkCleanupPromotesLiveParentOwnerLikeNoemaActiveResume(t *testing.T) {
	requireSmokeTest(t)
	if os.Getenv("MATRIX_OPENCODE_ACP_STDIO") != "1" {
		t.Skip("Set MATRIX_OPENCODE_ACP_STDIO=1 to run the real OpenCode ACP HTTP fork cleanup smoke.")
	}
	opencodePath := lookPath(t, "opencode")
	workspace := t.TempDir()
	writeFile(t, workspace+"/README.md", "matrix http fork cleanup smoke\n")
	beforeProcesses := opencodeACPProcesses(t)

	store := memstore.New()
	if err := store.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("set configured flag: %v", err)
	}
	router := agents.NewRouter(&realAgentResolver{
		protocol: "stdio",
		bin:      opencodePath,
		args:     []string{"acp", "--pure"},
	})
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(execprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), workspace)
	mgr := session.NewManager(store, router, onboarding.NewWizard(onboarding.WizardDependencies{
		Storage:   store,
		Localizer: staticLocalizer{},
	}), nil)

	mux := http.NewServeMux()
	matrixapi.NewServer(mgr).RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	const channelID = "smoke.opencode.http-standalone-fork-cleanup"
	policy := middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal
	parent := httpSessionAction(t, server.URL, map[string]interface{}{
		"channel_id":     channelID,
		"action":         "new",
		"target":         "opencode",
		"workspace_path": workspace,
	})
	parentID := parent.ActiveSessionID
	if parentID == "" {
		t.Fatalf("expected HTTP parent session, got %+v", parent)
	}
	parentRun := httpRun(t, server.URL, map[string]interface{}{
		"channel_id":     channelID,
		"agent_id":       "opencode",
		"workspace_path": workspace,
		"input":          "Reply exactly MATRIX_HTTP_PARENT_READY. Do not edit files.",
	})
	if output, _ := parentRun["output"].(string); !strings.Contains(output, "MATRIX_HTTP_PARENT_READY") {
		t.Fatalf("HTTP parent route did not prove prompt processing, output=%q", output)
	}

	for i := 1; i <= 5; i++ {
		makeActive := true
		fork := httpSessionAction(t, server.URL, map[string]interface{}{
			"channel_id":     channelID,
			"action":         "fork",
			"target":         parentID,
			"make_active":    makeActive,
			"restore_parent": false,
			"workspace_path": workspace,
			"ephemeral":      true,
			"cleanup_policy": policy,
		})
		if fork.Fork == nil || fork.Fork.ChildLogicalSessionID == "" {
			t.Fatalf("HTTP fork child %d missing child id: %+v", i, fork)
		}
		childID := fork.Fork.ChildLogicalSessionID
		childRun := httpRun(t, server.URL, map[string]interface{}{
			"channel_id":     channelID,
			"agent_id":       "opencode",
			"workspace_path": workspace,
			"input":          fmt.Sprintf("Reply exactly MATRIX_HTTP_CHILD_%d_OK. Do not edit files.", i),
		})
		if output, _ := childRun["output"].(string); !strings.Contains(output, fmt.Sprintf("MATRIX_HTTP_CHILD_%d_OK", i)) {
			t.Fatalf("HTTP child %d route did not prove prompt processing, output=%q", i, output)
		}
		cleanup := httpSessionAction(t, server.URL, map[string]interface{}{
			"channel_id":         channelID,
			"action":             "cleanup",
			"target":             childID,
			"cleanup_policy":     policy,
			"force_forget_local": true,
		})
		if cleanup.Error != nil {
			t.Fatalf("HTTP child %d cleanup returned typed error: %+v cleanup=%+v", i, cleanup.Error, cleanup.Cleanup)
		}
		assertStrongForkCleanup(t, cleanup.Cleanup, parentID)
		switched := httpSessionAction(t, server.URL, map[string]interface{}{
			"channel_id": channelID,
			"action":     "switch",
			"target":     parentID,
		})
		if switched.ActiveSessionID != parentID {
			t.Fatalf("HTTP parent restore after child %d failed, got %+v", i, switched)
		}
	}

	parentCleanup := httpSessionAction(t, server.URL, map[string]interface{}{
		"channel_id":         channelID,
		"action":             "cleanup",
		"target":             parentID,
		"cleanup_policy":     policy,
		"force_forget_local": true,
	})
	if parentCleanup.Error != nil || parentCleanup.Cleanup == nil || !parentCleanup.Cleanup.Clean ||
		!parentCleanup.Cleanup.StrongCleanup || parentCleanup.Cleanup.ProcessRetained {
		t.Fatalf("HTTP parent cleanup must be strong and non-retained, got %+v", parentCleanup)
	}

	router.Close()
	waitForNoNewOpenCodeACPProcesses(t, beforeProcesses, 10*time.Second)
}

func TestOpenCodeHTTPFinalRunCleanupScopesReconcileLikeNoemaResume(t *testing.T) {
	runOpenCodeHTTPFinalRunCleanupScopesReconcileLikeNoemaResume(t)
}

func TestOpenCode_RunOwnedForkResume_FinalCleanupStrongAfterInitialCleanup(t *testing.T) {
	runOpenCodeHTTPFinalRunCleanupScopesReconcileLikeNoemaResume(t)
}

func TestOpenCode_SequentialRunOwnedForkResume_SecondInitialCleanupDoesNotRetainProcess(t *testing.T) {
	requireSmokeTest(t)
	if os.Getenv("MATRIX_OPENCODE_ACP_STDIO") != "1" {
		t.Skip("Set MATRIX_OPENCODE_ACP_STDIO=1 to run the real OpenCode ACP sequential cleanup smoke.")
	}
	opencodePath := lookPath(t, "opencode")
	workspaceA := t.TempDir()
	workspaceB := t.TempDir()
	writeFile(t, workspaceA+"/README.md", "matrix sequential active resume workspace A\n")
	writeFile(t, workspaceB+"/README.md", "matrix sequential active resume workspace B\n")
	beforeProcesses := opencodeACPProcesses(t)

	store := memstore.New()
	if err := store.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("set configured flag: %v", err)
	}
	router := agents.NewRouter(&realAgentResolver{
		protocol: "stdio",
		bin:      opencodePath,
		args:     []string{"acp", "--pure"},
	})
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(execprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), workspaceA)
	mgr := session.NewManager(store, router, onboarding.NewWizard(onboarding.WizardDependencies{
		Storage:   store,
		Localizer: staticLocalizer{},
	}), nil)

	mux := http.NewServeMux()
	api := matrixapi.NewServer(mgr).WithTraceStorage(store)
	api.RegisterRoutes(mux)
	traceStore := runtrace.NewStore(store)
	server := httptest.NewServer(mux)
	defer server.Close()

	const policy = middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal
	runOpenCodeCancelledAsyncCycle(t, server.URL, traceStore, "smoke.opencode.seq-a-cancel", workspaceA, policy)
	cleanupA := runOpenCodeSharedOwnerInitialCleanupCycle(t, server.URL, "a", workspaceA, policy)
	assertStrongSessionCleanupNoRetained(t, "sequence A initial cleanup", cleanupA)
	cleanupB := runOpenCodeSharedOwnerInitialCleanupCycle(t, server.URL, "b", workspaceB, policy)
	assertStrongSessionCleanupNoRetained(t, "sequence B initial cleanup", cleanupB)

	router.Close()
	waitForNoNewOpenCodeACPProcesses(t, beforeProcesses, 10*time.Second)
}

func TestOpenCode_CancelDuringSessionCreate_CleanupUsesSelectedSessionAndJudgeDoesNotPreflightFail(t *testing.T) {
	requireSmokeTest(t)
	if os.Getenv("MATRIX_OPENCODE_ACP_STDIO") != "1" {
		t.Skip("Set MATRIX_OPENCODE_ACP_STDIO=1 to run the real OpenCode ACP cancel/create race smoke.")
	}
	opencodePath := lookPath(t, "opencode")
	workspace := t.TempDir()
	writeFile(t, workspace+"/README.md", "matrix cancel/create race workspace\n")
	beforeProcesses := opencodeACPProcesses(t)

	store := memstore.New()
	if err := store.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("set configured flag: %v", err)
	}
	router := agents.NewRouter(&realAgentResolver{
		protocol: "stdio",
		bin:      opencodePath,
		args:     []string{"acp", "--pure"},
	})
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(execprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), workspace)
	mgr := session.NewManager(store, router, onboarding.NewWizard(onboarding.WizardDependencies{
		Storage:   store,
		Localizer: staticLocalizer{},
	}), nil)

	mux := http.NewServeMux()
	matrixapi.NewServer(mgr).WithTraceStorage(store).RegisterRoutes(mux)
	traceStore := runtrace.NewStore(store)
	server := httptest.NewServer(mux)
	defer server.Close()

	const policy = middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal
	asyncRun := httpRun(t, server.URL, map[string]interface{}{
		"channel_id":     "smoke.opencode.cancel-create-race",
		"agent_id":       "opencode",
		"workspace_path": workspace,
		"execution_mode": "async",
		"input": "This is a Matrix cancel/create race test. Start working, then run a slow command like " +
			"`python3 -c \"import time; time.sleep(90)\"`, and do not give the final answer until the command is done.",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": policy,
	})
	runID, _ := asyncRun["run_id"].(string)
	if runID == "" {
		t.Fatalf("async run response missing run_id: %+v", asyncRun)
	}
	time.Sleep(250 * time.Millisecond)
	httpRunAction(t, server.URL, runID, map[string]interface{}{
		"action": "cancel",
		"reason": "matrix cancel/create race smoke",
	})
	cleanup := waitForRunCleanup(t, traceStore, runID, 3*time.Minute)
	assertStrongRunCleanup(t, cleanup)
	assertCleanupMatchesLateSelectedSessionWhenPresent(t, traceStore, runID, cleanup)
	assertRunTraceHasNoPreflightPoison(t, traceStore, runID)

	judge := httpRun(t, server.URL, map[string]interface{}{
		"channel_id":     "smoke.opencode.cancel-create-race-judge",
		"agent_id":       "opencode",
		"workspace_path": workspace,
		"input":          "Reply exactly MATRIX_CANCEL_RACE_JUDGE_OK. Do not edit files.",
	})
	if output, _ := judge["output"].(string); !strings.Contains(output, "MATRIX_CANCEL_RACE_JUDGE_OK") {
		t.Fatalf("judge-like request did not prove prompt processing after cancel/create race, output=%q", output)
	}
	judgeRunID, _ := judge["run_id"].(string)
	assertRunTraceHasNoPreflightPoison(t, traceStore, judgeRunID)

	router.Close()
	waitForNoNewOpenCodeACPProcesses(t, beforeProcesses, 10*time.Second)
}

func runOpenCodeHTTPFinalRunCleanupScopesReconcileLikeNoemaResume(t *testing.T) {
	requireSmokeTest(t)
	if os.Getenv("MATRIX_OPENCODE_ACP_STDIO") != "1" {
		t.Skip("Set MATRIX_OPENCODE_ACP_STDIO=1 to run the real OpenCode ACP final resume cleanup smoke.")
	}
	opencodePath := lookPath(t, "opencode")
	parentWorkspace := t.TempDir()
	resumeWorkspace := t.TempDir()
	writeFile(t, parentWorkspace+"/README.md", "matrix parent workspace\n")
	writeFile(t, resumeWorkspace+"/README.md", "matrix resumed run workspace\n")
	beforeProcesses := opencodeACPProcesses(t)

	store := memstore.New()
	if err := store.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("set configured flag: %v", err)
	}
	router := agents.NewRouter(&realAgentResolver{
		protocol: "stdio",
		bin:      opencodePath,
		args:     []string{"acp", "--pure"},
	})
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(execprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), parentWorkspace)
	mgr := session.NewManager(store, router, onboarding.NewWizard(onboarding.WizardDependencies{
		Storage:   store,
		Localizer: staticLocalizer{},
	}), nil)

	mux := http.NewServeMux()
	matrixapi.NewServer(mgr).WithTraceStorage(store).RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	const initialChannel = "smoke.opencode.noema-initial-parent"
	const resumeChannel = "smoke.opencode.noema-final-resume"
	policy := middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal
	parent := httpSessionAction(t, server.URL, map[string]interface{}{
		"channel_id": initialChannel,
		"action":     "new",
		"target":     "opencode",
	})
	parentID := parent.ActiveSessionID
	if parentID == "" {
		t.Fatalf("expected parent session, got %+v", parent)
	}
	parentRun := httpRun(t, server.URL, map[string]interface{}{
		"channel_id": initialChannel,
		"agent_id":   "opencode",
		"input":      "Reply exactly MATRIX_SCOPE_PARENT_READY. Do not edit files.",
	})
	if output, _ := parentRun["output"].(string); !strings.Contains(output, "MATRIX_SCOPE_PARENT_READY") {
		t.Fatalf("parent run did not prove prompt processing, output=%q", output)
	}

	makeActive := true
	fork := httpSessionAction(t, server.URL, map[string]interface{}{
		"channel_id":     initialChannel,
		"action":         "fork",
		"target":         parentID,
		"make_active":    makeActive,
		"restore_parent": false,
		"workspace_path": parentWorkspace,
		"ephemeral":      true,
		"cleanup_policy": policy,
	})
	if fork.Fork == nil || fork.Fork.ChildLogicalSessionID == "" {
		t.Fatalf("expected fork child, got %+v", fork)
	}
	childRun := httpRun(t, server.URL, map[string]interface{}{
		"channel_id":     initialChannel,
		"agent_id":       "opencode",
		"workspace_path": parentWorkspace,
		"input":          "Reply exactly MATRIX_SCOPE_CHILD_READY. Do not edit files.",
	})
	if output, _ := childRun["output"].(string); !strings.Contains(output, "MATRIX_SCOPE_CHILD_READY") {
		t.Fatalf("child run did not prove prompt processing, output=%q", output)
	}
	childCleanup := httpSessionAction(t, server.URL, map[string]interface{}{
		"channel_id":         initialChannel,
		"action":             "cleanup",
		"target":             fork.Fork.ChildLogicalSessionID,
		"cleanup_policy":     policy,
		"force_forget_local": true,
	})
	if childCleanup.Error != nil {
		t.Fatalf("child cleanup returned typed error: %+v cleanup=%+v", childCleanup.Error, childCleanup.Cleanup)
	}
	assertStrongForkCleanup(t, childCleanup.Cleanup, parentID)

	finalRun := httpRun(t, server.URL, map[string]interface{}{
		"channel_id":     resumeChannel,
		"agent_id":       "opencode",
		"workspace_path": resumeWorkspace,
		"input":          "Reply exactly MATRIX_SCOPE_FINAL_READY. Do not edit files.",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": policy,
	})
	if output, _ := finalRun["output"].(string); !strings.Contains(output, "MATRIX_SCOPE_FINAL_READY") {
		t.Fatalf("final resumed run did not prove prompt processing, output=%q", output)
	}
	finalCleanup := httpRunCleanup(t, finalRun)
	if !finalCleanup.Clean || !finalCleanup.StrongCleanup || finalCleanup.CleanupStrength != sessioncleanup.StrengthStrong ||
		finalCleanup.ProcessRetained || finalCleanup.FailureCode != "" {
		t.Fatalf("final resumed run cleanup must be strong and non-retained, got %+v", finalCleanup)
	}
	for _, related := range finalCleanup.RelatedSessions {
		if related.Retained {
			t.Fatalf("final cleanup must not retain related sessions, got %+v", finalCleanup.RelatedSessions)
		}
	}

	parentCleanup := httpSessionAction(t, server.URL, map[string]interface{}{
		"channel_id":         initialChannel,
		"action":             "cleanup",
		"target":             parentID,
		"cleanup_policy":     policy,
		"force_forget_local": true,
	})
	if parentCleanup.Error != nil || parentCleanup.Cleanup == nil || !parentCleanup.Cleanup.Clean ||
		!parentCleanup.Cleanup.StrongCleanup || parentCleanup.Cleanup.ProcessRetained {
		t.Fatalf("parent cleanup after final run must be strong and non-retained, got %+v", parentCleanup)
	}

	router.Close()
	waitForNoNewOpenCodeACPProcesses(t, beforeProcesses, 10*time.Second)
}

func runOpenCodeCancelledAsyncCycle(t *testing.T, baseURL string, traceStore *runtrace.Store, channelID, workspace, policy string) {
	t.Helper()
	asyncRun := httpRun(t, baseURL, map[string]interface{}{
		"channel_id":     channelID,
		"agent_id":       "opencode",
		"workspace_path": workspace,
		"execution_mode": "async",
		"input": "This is a Matrix sequential cleanup cancellation test. Start working, then run a slow command like " +
			"`python3 -c \"import time; time.sleep(90)\"`, and do not give the final answer until the command is done.",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": policy,
	})
	runID, _ := asyncRun["run_id"].(string)
	if runID == "" {
		t.Fatalf("async run response missing run_id: %+v", asyncRun)
	}
	if remoteID := waitForRunRemoteSessionID(t, traceStore, runID, 90*time.Second); remoteID == "" {
		t.Fatalf("async run did not expose remote session id")
	}
	time.Sleep(2 * time.Second)
	httpRunAction(t, baseURL, runID, map[string]interface{}{
		"action": "cancel",
		"reason": "matrix sequential cleanup smoke",
	})
	cleanup := waitForRunCleanup(t, traceStore, runID, 2*time.Minute)
	assertStrongRunCleanup(t, cleanup)
	cancelled := waitForRunTerminal(t, traceStore, runID, 2*time.Minute)
	if cancelled.Status != runtrace.StatusCancelled {
		t.Fatalf("expected cancelled async run, got %+v", cancelled)
	}
	assertRunTraceHasNoPreflightPoison(t, traceStore, runID)

	judge := httpRun(t, baseURL, map[string]interface{}{
		"channel_id":     channelID + "-judge",
		"agent_id":       "opencode",
		"workspace_path": workspace,
		"input":          "Reply exactly MATRIX_SEQ_JUDGE_OK. Do not edit files.",
	})
	if output, _ := judge["output"].(string); !strings.Contains(output, "MATRIX_SEQ_JUDGE_OK") {
		t.Fatalf("judge-like request did not prove prompt processing, output=%q", output)
	}
	judgeRunID, _ := judge["run_id"].(string)
	assertRunTraceHasNoPreflightPoison(t, traceStore, judgeRunID)
}

func runOpenCodeSharedOwnerInitialCleanupCycle(t *testing.T, baseURL, label, workspace, policy string) *middleware.SessionCleanupResult {
	t.Helper()
	parentChannel := "smoke.opencode.seq-" + label + "-parent"
	peerChannel := "smoke.opencode.seq-" + label + "-peer"
	parent := httpSessionAction(t, baseURL, map[string]interface{}{
		"channel_id":     parentChannel,
		"action":         "new",
		"target":         "opencode",
		"workspace_path": workspace,
	})
	parentID := parent.ActiveSessionID
	if parentID == "" {
		t.Fatalf("expected parent session, got %+v", parent)
	}
	parentRun := httpRun(t, baseURL, map[string]interface{}{
		"channel_id":     parentChannel,
		"agent_id":       "opencode",
		"workspace_path": workspace,
		"input":          "Reply exactly MATRIX_SEQ_" + strings.ToUpper(label) + "_PARENT_READY. Do not edit files.",
	})
	if output, _ := parentRun["output"].(string); !strings.Contains(output, "MATRIX_SEQ_"+strings.ToUpper(label)+"_PARENT_READY") {
		t.Fatalf("parent run did not prove prompt processing, output=%q", output)
	}

	peer := httpSessionAction(t, baseURL, map[string]interface{}{
		"channel_id":     peerChannel,
		"action":         "new",
		"target":         "opencode",
		"workspace_path": workspace,
	})
	peerID := peer.ActiveSessionID
	if peerID == "" {
		t.Fatalf("expected peer owner session, got %+v", peer)
	}
	peerRun := httpRun(t, baseURL, map[string]interface{}{
		"channel_id":     peerChannel,
		"agent_id":       "opencode",
		"workspace_path": workspace,
		"input":          "Reply exactly MATRIX_SEQ_" + strings.ToUpper(label) + "_PEER_READY. Do not edit files.",
	})
	if output, _ := peerRun["output"].(string); !strings.Contains(output, "MATRIX_SEQ_"+strings.ToUpper(label)+"_PEER_READY") {
		t.Fatalf("peer owner run did not prove prompt processing, output=%q", output)
	}

	makeActive := true
	fork := httpSessionAction(t, baseURL, map[string]interface{}{
		"channel_id":     parentChannel,
		"action":         "fork",
		"target":         parentID,
		"make_active":    makeActive,
		"restore_parent": false,
		"workspace_path": workspace,
		"ephemeral":      true,
		"cleanup_policy": policy,
	})
	if fork.Fork == nil || fork.Fork.ChildLogicalSessionID == "" {
		t.Fatalf("expected fork child, got %+v", fork)
	}
	childRun := httpRun(t, baseURL, map[string]interface{}{
		"channel_id":     parentChannel,
		"agent_id":       "opencode",
		"workspace_path": workspace,
		"input":          "Reply exactly MATRIX_SEQ_" + strings.ToUpper(label) + "_CHILD_READY. Do not edit files.",
	})
	if output, _ := childRun["output"].(string); !strings.Contains(output, "MATRIX_SEQ_"+strings.ToUpper(label)+"_CHILD_READY") {
		t.Fatalf("child run did not prove prompt processing, output=%q", output)
	}
	childCleanup := httpSessionAction(t, baseURL, map[string]interface{}{
		"channel_id":         parentChannel,
		"action":             "cleanup",
		"target":             fork.Fork.ChildLogicalSessionID,
		"cleanup_policy":     policy,
		"force_forget_local": true,
	})
	if childCleanup.Error != nil {
		t.Fatalf("child cleanup returned typed error: %+v cleanup=%+v", childCleanup.Error, childCleanup.Cleanup)
	}
	assertStrongForkCleanup(t, childCleanup.Cleanup, parentID)

	parentCleanup := httpSessionAction(t, baseURL, map[string]interface{}{
		"channel_id":         parentChannel,
		"action":             "cleanup",
		"target":             parentID,
		"cleanup_policy":     policy,
		"force_forget_local": true,
	})
	if parentCleanup.Error != nil {
		t.Fatalf("parent initial cleanup returned typed error: %+v cleanup=%+v", parentCleanup.Error, parentCleanup.Cleanup)
	}
	assertSharedOwnerEvidence(t, parentCleanup.Cleanup, peerID)

	peerCleanup := httpSessionAction(t, baseURL, map[string]interface{}{
		"channel_id":         peerChannel,
		"action":             "cleanup",
		"target":             peerID,
		"cleanup_policy":     policy,
		"force_forget_local": true,
	})
	if peerCleanup.Error != nil {
		t.Fatalf("peer cleanup returned typed error: %+v cleanup=%+v", peerCleanup.Error, peerCleanup.Cleanup)
	}
	assertStrongSessionCleanupNoRetained(t, "peer cleanup", peerCleanup.Cleanup)
	return parentCleanup.Cleanup
}

func createRunOwnedOpenCodeParent(ctx context.Context, t *testing.T, mgr *session.Manager, channelID, workspace, policy string) string {
	t.Helper()
	result, err := mgr.HandleSessionActionTyped(ctx, middleware.SessionActionRequest{
		ChannelID:     channelID,
		Action:        "new",
		OwnerRunID:    "smoke-run-owned-opencode",
		Target:        "opencode",
		WorkspacePath: workspace,
		Ephemeral:     true,
		CleanupPolicy: policy,
	})
	if err != nil {
		t.Fatalf("create run-owned parent: %v", err)
	}
	if result.Session == nil || strings.TrimSpace(result.ActiveSessionID) == "" {
		t.Fatalf("expected active parent session, got %+v", result)
	}
	return result.ActiveSessionID
}

func routeParentOpenCodeSession(ctx context.Context, t *testing.T, mgr *session.Manager, channelID, workspace, parent string) {
	t.Helper()
	output, err := mgr.RouteConversation(ctx, middleware.ConversationRequest{
		ChannelID:        channelID,
		AgentID:          "opencode",
		LogicalSessionID: parent,
		WorkspacePath:    workspace,
		Input:            "Reply exactly MATRIX_PARENT_READY. Do not edit files.",
		NonInteractive:   true,
	})
	if err != nil {
		t.Fatalf("route parent session: %v", err)
	}
	if !strings.Contains(output, "MATRIX_PARENT_READY") {
		t.Fatalf("parent route did not prove prompt processing, output=%q", output)
	}
	status, err := mgr.HandleSessionActionTyped(ctx, middleware.SessionActionRequest{
		ChannelID: channelID,
		Action:    "status",
		Target:    parent,
	})
	if err != nil {
		t.Fatalf("parent status: %v", err)
	}
	if status.Session == nil || strings.TrimSpace(status.Session.RemoteSessionID) == "" {
		t.Fatalf("parent route must persist remote session id before fork, got %+v", status)
	}
}

func runOpenCodeForkArtifactCleanup(ctx context.Context, t *testing.T, mgr *session.Manager, channelID, workspace, parent, policy string, index int) middleware.SessionActionResult {
	t.Helper()
	makeActive := false
	result, err := mgr.HandleSessionActionTyped(ctx, middleware.SessionActionRequest{
		ChannelID:     channelID,
		Action:        "fork",
		Target:        parent,
		Input:         fmt.Sprintf("Reply exactly MATRIX_CHILD_%d_OK. Do not edit files.", index),
		MakeActive:    &makeActive,
		RestoreParent: true,
		WorkspacePath: workspace,
		Ephemeral:     true,
		CleanupPolicy: policy,
	})
	if err != nil {
		t.Fatalf("fork child %d: %v", index, err)
	}
	if result.Error != nil {
		t.Fatalf("fork child %d returned typed error: %+v cleanup=%+v", index, result.Error, result.Fork)
	}
	if result.Fork == nil || result.Fork.Artifact == nil || result.Fork.Cleanup == nil {
		t.Fatalf("fork child %d must include artifact and cleanup proof, got %+v", index, result)
	}
	if !strings.Contains(result.Fork.Artifact.Content, fmt.Sprintf("MATRIX_CHILD_%d_OK", index)) {
		t.Fatalf("fork child %d did not prove prompt processing, artifact=%q", index, result.Fork.Artifact.Content)
	}
	return result
}

func assertStrongForkCleanup(t *testing.T, cleanup *middleware.SessionCleanupResult, parent string) {
	t.Helper()
	if cleanup == nil || !cleanup.Clean || !cleanup.StrongCleanup || cleanup.CleanupStrength != sessioncleanup.StrengthStrong {
		t.Fatalf("fork child cleanup must be strong, got %+v", cleanup)
	}
	if cleanup.ProcessRetained || cleanup.ProcessRetentionReason != "" || cleanup.FailureCode != "" || cleanup.Error != "" {
		t.Fatalf("fork child cleanup must not retain parent client, got %+v", cleanup)
	}
	if len(cleanup.RelatedSessions) != 1 || cleanup.RelatedSessions[0].Retained ||
		cleanup.RelatedSessions[0].LogicalSessionID != parent ||
		cleanup.RelatedSessions[0].Reason != sessioncleanup.ReasonForkParentAgentClientOwner {
		t.Fatalf("fork child cleanup must include non-retained parent owner proof, got %+v", cleanup.RelatedSessions)
	}
}

func cleanupOpenCodeSession(ctx context.Context, t *testing.T, mgr *session.Manager, channelID, target, policy string) *middleware.SessionCleanupResult {
	t.Helper()
	result, err := mgr.HandleSessionActionTyped(ctx, middleware.SessionActionRequest{
		ChannelID:        channelID,
		Action:           "cleanup",
		Target:           target,
		CleanupPolicy:    policy,
		ForceForgetLocal: true,
	})
	if err != nil {
		t.Fatalf("cleanup %s: %v", target, err)
	}
	if result.Error != nil {
		t.Fatalf("cleanup %s returned typed error: %+v cleanup=%+v", target, result.Error, result.Cleanup)
	}
	if result.Cleanup == nil {
		t.Fatalf("cleanup %s returned no proof", target)
	}
	return result.Cleanup
}

func httpSessionAction(t *testing.T, baseURL string, payload map[string]interface{}) middleware.SessionActionResult {
	t.Helper()
	var result middleware.SessionActionResult
	status := postJSON(t, baseURL+matrixapi.SessionActionPathV1, payload, &result)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		t.Fatalf("session action HTTP status=%d result=%+v", status, result)
	}
	return result
}

func httpRun(t *testing.T, baseURL string, payload map[string]interface{}) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	status := postJSON(t, baseURL+matrixapi.RunPathV1, payload, &result)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		t.Fatalf("run HTTP status=%d result=%+v", status, result)
	}
	if errText, _ := result["error"].(string); strings.TrimSpace(errText) != "" {
		t.Fatalf("run returned error: %+v", result)
	}
	return result
}

func httpRunCleanup(t *testing.T, result map[string]interface{}) middleware.SessionCleanupResult {
	t.Helper()
	raw, ok := result["cleanup"]
	if !ok || raw == nil {
		t.Fatalf("run response has no cleanup proof: %+v", result)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal cleanup proof: %v", err)
	}
	var cleanup middleware.SessionCleanupResult
	if err := json.Unmarshal(data, &cleanup); err != nil {
		t.Fatalf("decode cleanup proof: %v", err)
	}
	return cleanup
}

func httpRunAction(t *testing.T, baseURL string, runID string, payload map[string]interface{}) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	status := postJSON(t, baseURL+matrixapi.RunResourcePrefixV1+runID+"/actions", payload, &result)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		t.Fatalf("run action HTTP status=%d result=%+v", status, result)
	}
	return result
}

func waitForRunRemoteSessionID(t *testing.T, store *runtrace.Store, runID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		run, found, err := store.LoadRun(runID)
		if err != nil {
			t.Fatalf("LoadRun: %v", err)
		}
		if found && strings.TrimSpace(run.RemoteSessionID) != "" {
			return run.RemoteSessionID
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("remote session id not observed for run %s", runID)
	return ""
}

func waitForRunTerminal(t *testing.T, store *runtrace.Store, runID string, timeout time.Duration) runtrace.Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		run, found, err := store.LoadRun(runID)
		if err != nil {
			t.Fatalf("LoadRun: %v", err)
		}
		if found && (run.Status == runtrace.StatusCompleted || run.Status == runtrace.StatusCancelled || run.Status == runtrace.StatusFailed) {
			return run
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("terminal run status not observed for run %s", runID)
	return runtrace.Run{}
}

func waitForRunCleanup(t *testing.T, store *runtrace.Store, runID string, timeout time.Duration) middleware.SessionCleanupResult {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events, err := store.LoadEvents(runID, 200)
		if err != nil {
			t.Fatalf("LoadEvents: %v", err)
		}
		for _, event := range events {
			if event.Kind != "session.cleanup" {
				continue
			}
			data, err := json.Marshal(event.Metadata)
			if err != nil {
				t.Fatalf("marshal cleanup metadata: %v", err)
			}
			var cleanup middleware.SessionCleanupResult
			if err := json.Unmarshal(data, &cleanup); err != nil {
				t.Fatalf("decode cleanup metadata: %v", err)
			}
			return cleanup
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("session cleanup event not observed for run %s", runID)
	return middleware.SessionCleanupResult{}
}

func assertStrongRunCleanup(t *testing.T, cleanup middleware.SessionCleanupResult) {
	t.Helper()
	if !cleanup.Clean || !cleanup.StrongCleanup || cleanup.CleanupStrength != sessioncleanup.StrengthStrong ||
		cleanup.ProcessRetained || strings.TrimSpace(cleanup.FailureCode) != "" {
		t.Fatalf("run cleanup must be strong and non-retained, got %+v", cleanup)
	}
	for _, related := range cleanup.RelatedSessions {
		if related.Retained {
			t.Fatalf("run cleanup must not retain related sessions, got %+v", cleanup.RelatedSessions)
		}
	}
}

func assertStrongSessionCleanupNoRetained(t *testing.T, label string, cleanup *middleware.SessionCleanupResult) {
	t.Helper()
	if cleanup == nil || !cleanup.Clean || !cleanup.StrongCleanup || cleanup.CleanupStrength != sessioncleanup.StrengthStrong ||
		cleanup.ProcessRetained || strings.TrimSpace(cleanup.FailureCode) != "" {
		t.Fatalf("%s must be strong and non-retained, got %+v", label, cleanup)
	}
	for _, related := range cleanup.RelatedSessions {
		if related.Retained {
			t.Fatalf("%s must not retain related sessions, got %+v", label, cleanup.RelatedSessions)
		}
	}
}

func assertSharedOwnerEvidence(t *testing.T, cleanup *middleware.SessionCleanupResult, peerID string) {
	t.Helper()
	assertStrongSessionCleanupNoRetained(t, "shared owner cleanup", cleanup)
	for _, related := range cleanup.RelatedSessions {
		if related.LogicalSessionID == peerID && !related.Retained &&
			related.Reason == sessioncleanup.ReasonSharedAgentClientOwner && related.Active {
			return
		}
	}
	t.Fatalf("expected active shared owner evidence for %s, got %+v", peerID, cleanup.RelatedSessions)
}

func assertCleanupMatchesLateSelectedSessionWhenPresent(t *testing.T, store *runtrace.Store, runID string, cleanup middleware.SessionCleanupResult) {
	t.Helper()
	trace, found, err := store.Trace(runID)
	if err != nil || !found {
		t.Fatalf("Trace(%s): found=%v err=%v", runID, found, err)
	}
	var selectedLogical, selectedRemote string
	for _, event := range trace.Events {
		if event.Kind != "session.created" && event.Kind != "session.resumed" {
			continue
		}
		logical, _ := event.Metadata["logical_session_id"].(string)
		remote, _ := event.Metadata["remote_session_id"].(string)
		if strings.TrimSpace(remote) == "" {
			continue
		}
		selectedLogical = strings.TrimSpace(logical)
		selectedRemote = strings.TrimSpace(remote)
	}
	if selectedRemote == "" {
		return
	}
	if cleanup.LogicalSessionID != selectedLogical || cleanup.RemoteSessionID != selectedRemote {
		t.Fatalf("cleanup must match late selected session logical=%q remote=%q, got %+v", selectedLogical, selectedRemote, cleanup)
	}
}

func assertRunTraceHasNoPreflightPoison(t *testing.T, store *runtrace.Store, runID string) {
	t.Helper()
	if strings.TrimSpace(runID) == "" {
		return
	}
	trace, found, err := store.Trace(runID)
	if err != nil || !found {
		t.Fatalf("Trace(%s): found=%v err=%v", runID, found, err)
	}
	for _, event := range trace.Events {
		text := strings.ToLower(event.Kind + " " + event.Message + " " + event.Summary)
		if event.Metadata != nil {
			raw, _ := json.Marshal(event.Metadata)
			text += " " + strings.ToLower(string(raw))
		}
		if strings.Contains(text, "provider.preflight.failed") ||
			strings.Contains(text, "agent_preflight_failed") ||
			strings.Contains(text, "provider_client_context_cancelled") ||
			strings.Contains(text, "acp prompt failed: context canceled") {
			t.Fatalf("trace %s contains provider preflight poison in event %+v", runID, event)
		}
	}
}

func postJSON(t *testing.T, url string, payload map[string]interface{}, result interface{}) int {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s payload: %v", url, err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		t.Fatalf("decode %s response status=%d: %v", url, resp.StatusCode, err)
	}
	return resp.StatusCode
}

func opencodeACPProcesses(t *testing.T) map[int]string {
	t.Helper()
	if _, err := exec.LookPath("ps"); err != nil {
		t.Skip("ps binary not found; cannot verify OpenCode process cleanup")
	}
	out, err := exec.Command("ps", "-eo", "pid=,ppid=,command=").Output()
	if err != nil {
		t.Fatalf("ps failed: %v", err)
	}
	rootPID := os.Getpid()
	parents := map[int]int{}
	processes := map[int]string{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 4 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		parents[pid] = ppid
		cmd := strings.Join(fields[2:], " ")
		if strings.Contains(cmd, "opencode acp") {
			processes[pid] = cmd
		}
	}
	for pid := range processes {
		if !hasAncestorPID(pid, rootPID, parents) {
			delete(processes, pid)
		}
	}
	return processes
}

func hasAncestorPID(pid, ancestor int, parents map[int]int) bool {
	for pid > 1 {
		ppid, ok := parents[pid]
		if !ok {
			return false
		}
		if ppid == ancestor {
			return true
		}
		pid = ppid
	}
	return false
}

func waitForNoNewOpenCodeACPProcesses(t *testing.T, before map[int]string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var retained map[int]string
	for time.Now().Before(deadline) {
		retained = map[int]string{}
		for pid, cmd := range opencodeACPProcesses(t) {
			if _, existed := before[pid]; !existed {
				retained[pid] = cmd
			}
		}
		if len(retained) == 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("new OpenCode ACP processes retained after cleanup: %+v", retained)
}
