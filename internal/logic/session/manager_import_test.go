package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestImportRemoteSessionRequiresProviderProofAndKeepsStrictIdentity(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &mockStorage{}
	if err := store.Set("system.configured", []byte("true")); err != nil {
		t.Fatal(err)
	}
	router := &mockRouter{attachRemote: middleware.RemoteSessionInfo{
		RemoteSessionID: "external-1", Cwd: workspace, ProtocolKind: middleware.ProtocolKindACP,
	}}
	mgr := NewManager(store, router, newTestWizard(store), nil)
	req := middleware.SessionActionRequest{ChannelID: "test", Action: "import", AgentID: "dsh", Target: "external-1", WorkspacePath: workspace}
	result, err := mgr.HandleSessionActionTyped(context.Background(), req)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.ActiveSessionID == "" || router.attachPath != workspace {
		t.Fatalf("missing attach proof: %+v path=%s", result, router.attachPath)
	}
	meta, found, err := mgr.InspectSession(result.ActiveSessionID)
	if err != nil || !found || !meta.StrictRemote || meta.AgentSessionID != "external-1" || meta.WorkspacePath != workspace {
		t.Fatalf("wrong mirror: %+v found=%v err=%v", meta, found, err)
	}
	if _, err := mgr.Route(context.Background(), "test", "dsh", "next turn", nil); err != nil {
		t.Fatal(err)
	}
	if !router.lastStrict || router.lastRemote != "external-1" {
		t.Fatalf("route lost remote identity: strict=%v remote=%s", router.lastStrict, router.lastRemote)
	}
	// Recreate the manager over the persisted store, as daemon startup does.
	restarted := NewManager(store, router, newTestWizard(store), nil)
	if _, err := restarted.Route(context.Background(), "test", "dsh", "after restart", nil); err != nil {
		t.Fatal(err)
	}
	if !router.lastStrict || router.lastRemote != "external-1" {
		t.Fatalf("restart lost imported remote identity: strict=%v remote=%s", router.lastStrict, router.lastRemote)
	}
}

func TestImportRemoteSessionRefusesUnverifiedOrDifferentWorkspace(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		remote    middleware.RemoteSessionInfo
		attachErr error
		want      string
	}{
		{"provider failure", middleware.RemoteSessionInfo{}, errors.New("auth required"), "auth required"},
		{"different ID", middleware.RemoteSessionInfo{RemoteSessionID: "other", Cwd: workspace}, nil, "instead of"},
		{"different cwd", middleware.RemoteSessionInfo{RemoteSessionID: "external-1", Cwd: t.TempDir()}, nil, "workspace_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStorage{}
			router := &mockRouter{attachRemote: tc.remote, attachErr: tc.attachErr}
			mgr := NewManager(store, router, newTestWizard(store), nil)
			_, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{ChannelID: "test", Action: "import", AgentID: "dsh", Target: "external-1", WorkspacePath: workspace})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
			keys, err := store.List("session.meta.")
			if err != nil || len(keys) != 0 {
				t.Fatalf("failed import persisted mirror: %v %v", keys, err)
			}
		})
	}
}

func TestImportRemoteSessionValidatesAndPreservesProviderDirectories(t *testing.T) {
	workspace := t.TempDir()
	inside := filepath.Join(workspace, "inside")
	outside := t.TempDir()
	if err := os.Mkdir(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, _ = filepath.EvalSymlinks(workspace)
	inside, _ = filepath.EvalSymlinks(inside)
	outside, _ = filepath.EvalSymlinks(outside)
	remote := middleware.RemoteSessionInfo{RemoteSessionID: "external-1", Cwd: workspace,
		ProtocolKind: middleware.ProtocolKindACP, AdditionalDirectories: []string{inside, outside}}
	store := &mockStorage{}
	router := &mockRouter{attachRemote: remote}
	mgr := NewManager(store, router, newTestWizard(store), nil)
	req := middleware.SessionActionRequest{ChannelID: "test", Action: "import", AgentID: "dsh", Target: "external-1", WorkspacePath: workspace}
	if _, err := mgr.HandleSessionActionTyped(context.Background(), req); err == nil || !strings.Contains(err.Error(), "workspace_mismatch") {
		t.Fatalf("expected refusal of unapproved outside directory, got %v", err)
	}
	req.AdditionalDirectories = []string{outside}
	result, err := mgr.HandleSessionActionTyped(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	meta, found, err := mgr.InspectSession(result.ActiveSessionID)
	if err != nil || !found || len(meta.AdditionalDirectories) != 2 || meta.AdditionalDirectories[0] != inside || meta.AdditionalDirectories[1] != outside {
		t.Fatalf("reported directories not preserved canonically: %+v found=%v err=%v", meta, found, err)
	}
	routed := buildRouteRequest(middleware.ConversationRequest{}, meta, meta.ID, "next")
	if len(routed.AdditionalDirectories) != 2 {
		t.Fatalf("strict route lost provider directories: %+v", routed.AdditionalDirectories)
	}
}
