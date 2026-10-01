package workspace

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// Identity is the canonical workspace a run or session is bound to. It is
// resolved from the registry before any grant, session or agent client is
// reused, so the rest of the pipeline works on one agreed answer instead of on
// whichever of the two hints the caller happened to supply.
type Identity struct {
	// ID is the registered workspace id. Empty when the caller supplied a path
	// that no registered workspace owns.
	ID string
	// Path is the canonical absolute root path. Empty when it is unknown: a
	// registered workspace without a recorded root, or no hint at all.
	Path string
	// Registered reports whether the identity came from a registered workspace.
	Registered bool
	// Source names the hint the identity was resolved from.
	Source string
}

const (
	// IdentitySourceWorkspaceID means only a workspace id was supplied and the
	// canonical path was taken from the registry.
	IdentitySourceWorkspaceID = "workspace_id"
	// IdentitySourceWorkspacePath means only a path was supplied.
	IdentitySourceWorkspacePath = "workspace_path"
	// IdentitySourceIDAndPath means both hints were supplied and they agreed.
	IdentitySourceIDAndPath = "workspace_id+workspace_path"
)

// NotFoundError reports a workspace id with no registry entry.
type NotFoundError struct{ ID string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("workspace %s not found", e.ID) }

// MismatchError reports two hints that denote different workspaces. It is
// refused before a session or agent client is reused: silently binding a
// channel to the id while running in the other directory is how workstreams
// contaminate each other.
type MismatchError struct {
	ID             string
	RegisteredPath string
	RequestedPath  string
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf("workspace_identity_mismatch: workspace %s has root %s, request asked for %s",
		e.ID, e.RegisteredPath, e.RequestedPath)
}

// Empty reports an identity without any workspace hint.
func (i Identity) Empty() bool { return i.ID == "" && i.Path == "" }

// SameWorkspace reports whether a session recorded as bound to
// (workspaceID, workspacePath) denotes the same workspace as this identity.
// An empty recorded hint is unknown, not a different workspace; two recorded
// hints that disagree are a different workspace.
func (i Identity) SameWorkspace(workspaceID, workspacePath string) bool {
	recordedID := strings.TrimSpace(workspaceID)
	if i.ID != "" && recordedID != "" && i.ID != recordedID {
		return false
	}
	return sameWorkspacePath(i.Path, workspacePath)
}

// sameWorkspacePath compares two recorded paths, treating an empty one as
// unknown rather than as a different workspace.
func sameWorkspacePath(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return true
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

// ResolveIdentity turns the (workspace_id, workspace_path) pair of one request
// into a single canonical identity:
//
//   - workspace_id alone resolves the canonical root path from the registry, so
//     a grant can be evaluated without the caller repeating the path;
//   - workspace_id with a path requires the registry to agree with the path;
//   - a path alone resolves the registered workspace that owns it, or stays an
//     unregistered path (the grant decides whether that is acceptable);
//   - no hints resolve to the empty identity.
func ResolveIdentity(storage middleware.Storage, workspaceID, workspacePath string) (Identity, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	workspacePath = strings.TrimSpace(workspacePath)
	if workspaceID == "" && workspacePath == "" {
		return Identity{}, nil
	}
	if workspaceID != "" {
		return resolveIdentityByID(storage, workspaceID, workspacePath)
	}
	return resolveIdentityByPath(storage, workspacePath)
}

func resolveIdentityByID(storage middleware.Storage, workspaceID, workspacePath string) (Identity, error) {
	meta, found, err := LoadMeta(storage, workspaceID)
	if err != nil {
		return Identity{}, err
	}
	if !found {
		return Identity{}, &NotFoundError{ID: workspaceID}
	}
	registered := strings.TrimSpace(meta.RootPath)
	if registered != "" {
		registered = filepath.Clean(registered)
	}
	if workspacePath == "" {
		return Identity{ID: meta.ID, Path: registered, Registered: true, Source: IdentitySourceWorkspaceID}, nil
	}
	requested := filepath.Clean(workspacePath)
	if registered != "" && registered != requested {
		return Identity{}, &MismatchError{ID: meta.ID, RegisteredPath: registered, RequestedPath: requested}
	}
	return Identity{ID: meta.ID, Path: requested, Registered: true, Source: IdentitySourceIDAndPath}, nil
}

func resolveIdentityByPath(storage middleware.Storage, workspacePath string) (Identity, error) {
	clean := filepath.Clean(workspacePath)
	meta, found, err := ResolveByPath(storage, clean)
	if err != nil {
		return Identity{}, err
	}
	if !found {
		return Identity{Path: clean, Source: IdentitySourceWorkspacePath}, nil
	}
	registered := strings.TrimSpace(meta.RootPath)
	if registered == "" {
		registered = clean
	}
	registered = filepath.Clean(registered)
	if registered != clean {
		// The path index points at a workspace whose recorded root is another
		// directory: the association is ambiguous, so it is refused instead of
		// silently binding the run to the indexed workspace.
		return Identity{}, &MismatchError{ID: meta.ID, RegisteredPath: registered, RequestedPath: clean}
	}
	return Identity{ID: meta.ID, Path: registered, Registered: true, Source: IdentitySourceWorkspacePath}, nil
}
