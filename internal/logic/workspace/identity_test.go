package workspace

import (
	"errors"
	"testing"
)

func identityStorage(t *testing.T, metas ...Meta) *mockStorage {
	t.Helper()
	storage := &mockStorage{}
	for _, meta := range metas {
		if err := SaveMeta(storage, meta); err != nil {
			t.Fatalf("SaveMeta(%s): %v", meta.ID, err)
		}
	}
	return storage
}

func TestResolveIdentityFromWorkspaceIDUsesRegisteredRoot(t *testing.T) {
	storage := identityStorage(t, Meta{ID: "ws-a", RootPath: "/srv/work/a"})
	identity, err := ResolveIdentity(storage, "ws-a", "")
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	if identity.ID != "ws-a" || identity.Path != "/srv/work/a" || !identity.Registered || identity.Source != IdentitySourceWorkspaceID {
		t.Fatalf("workspace id alone must resolve the registered root, got %+v", identity)
	}
}

func TestResolveIdentityAcceptsBothHintsWhenTheyAgree(t *testing.T) {
	storage := identityStorage(t, Meta{ID: "ws-a", RootPath: "/srv/work/a"})
	identity, err := ResolveIdentity(storage, "ws-a", "/srv/work/a/")
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	if identity.ID != "ws-a" || identity.Path != "/srv/work/a" || identity.Source != IdentitySourceIDAndPath {
		t.Fatalf("matching hints must resolve to one canonical identity, got %+v", identity)
	}
}

func TestResolveIdentityRefusesMismatchedHints(t *testing.T) {
	storage := identityStorage(t,
		Meta{ID: "ws-a", RootPath: "/srv/work/a"},
		Meta{ID: "ws-b", RootPath: "/srv/work/b"},
	)
	identity, err := ResolveIdentity(storage, "ws-a", "/srv/work/b")
	var mismatch *MismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("mismatched hints must be a typed refusal, got identity=%+v err=%v", identity, err)
	}
	if mismatch.ID != "ws-a" || mismatch.RegisteredPath != "/srv/work/a" || mismatch.RequestedPath != "/srv/work/b" {
		t.Fatalf("mismatch must name both sides, got %+v", mismatch)
	}
	if !identity.Empty() {
		t.Fatalf("a refused pair must not resolve to an identity, got %+v", identity)
	}
}

func TestResolveIdentityRefusesUnknownWorkspaceID(t *testing.T) {
	storage := identityStorage(t, Meta{ID: "ws-a", RootPath: "/srv/work/a"})
	_, err := ResolveIdentity(storage, "ws-missing", "/srv/work/a")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) || notFound.ID != "ws-missing" {
		t.Fatalf("unknown workspace id must be a typed not-found, got %v", err)
	}
}

func TestResolveIdentityResolvesPathToItsRegisteredWorkspace(t *testing.T) {
	storage := identityStorage(t,
		Meta{ID: "ws-a", RootPath: "/srv/work/a"},
		Meta{ID: "ws-b", RootPath: "/srv/work/b"},
	)
	identity, err := ResolveIdentity(storage, "", "/srv/work/b/")
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	if identity.ID != "ws-b" || identity.Path != "/srv/work/b" || !identity.Registered || identity.Source != IdentitySourceWorkspacePath {
		t.Fatalf("a registered path must resolve its workspace id, got %+v", identity)
	}
}

func TestResolveIdentityKeepsAnUnregisteredPath(t *testing.T) {
	storage := identityStorage(t, Meta{ID: "ws-a", RootPath: "/srv/work/a"})
	identity, err := ResolveIdentity(storage, "", "/srv/elsewhere")
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	if identity.ID != "" || identity.Path != "/srv/elsewhere" || identity.Registered {
		t.Fatalf("an unregistered path stays a path for the grant to judge, got %+v", identity)
	}
}

func TestResolveIdentityWithoutHintsIsEmpty(t *testing.T) {
	identity, err := ResolveIdentity(identityStorage(t), "", "")
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	if !identity.Empty() {
		t.Fatalf("no hints is not a workspace, got %+v", identity)
	}
}

// A stale path index pointing at a workspace whose recorded root is another
// directory is the ambiguous association that used to bind silently.
func TestResolveIdentityRefusesAPathIndexedToAnotherRoot(t *testing.T) {
	storage := identityStorage(t, Meta{ID: "ws-a", RootPath: "/srv/work/a"})
	if err := storage.Set(pathIndexKey("/srv/work/other"), []byte("ws-a")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	_, err := ResolveIdentity(storage, "", "/srv/work/other")
	var mismatch *MismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("an index that contradicts the registered root must be refused, got %v", err)
	}
}

func TestIdentitySameWorkspaceTreatsUnknownAsUnknown(t *testing.T) {
	identity, err := ResolveIdentity(identityStorage(t, Meta{ID: "ws-a", RootPath: "/srv/work/a"}), "ws-a", "")
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	for _, tc := range []struct {
		name          string
		sessionID     string
		sessionPath   string
		sameWorkspace bool
	}{
		{"same id and path", "ws-a", "/srv/work/a", true},
		{"same id, unrecorded path", "ws-a", "", true},
		{"same id, other path", "ws-a", "/srv/work/b", false},
		{"other id", "ws-b", "/srv/work/a", false},
		{"no recorded id, same path", "", "/srv/work/a", true},
	} {
		if got := identity.SameWorkspace(tc.sessionID, tc.sessionPath); got != tc.sameWorkspace {
			t.Fatalf("%s: SameWorkspace=%v want %v", tc.name, got, tc.sameWorkspace)
		}
	}
}
