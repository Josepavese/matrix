package workspacegrant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

const keyPrefix = "workspace.grant."

// Grant authorizes one repository root and optionally its linked worktrees.
// It does not grant trust in the external agent's own filesystem policy.
type Grant struct {
	ID                  string    `json:"id"`
	RepositoryPath      string    `json:"repository_path"`
	CommonDir           string    `json:"git_common_dir"`
	IncludeGitWorktrees bool      `json:"include_git_worktrees"`
	CreatedAt           time.Time `json:"created_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

type Store struct{ storage middleware.Storage }

func NewStore(storage middleware.Storage) *Store { return &Store{storage: storage} }

func (s *Store) Register(ctx context.Context, path string, includeWorktrees bool, ttl time.Duration) (Grant, error) {
	if ttl < time.Minute || ttl > 30*24*time.Hour {
		return Grant{}, fmt.Errorf("grant TTL must be between one minute and 30 days")
	}
	root, commonDir, err := inspectGitRoot(ctx, path)
	if err != nil {
		return Grant{}, err
	}
	digest := sha256.Sum256([]byte(commonDir))
	now := time.Now().UTC()
	grant := Grant{ID: hex.EncodeToString(digest[:]), RepositoryPath: root,
		CommonDir: commonDir, IncludeGitWorktrees: includeWorktrees,
		CreatedAt: now, ExpiresAt: now.Add(ttl)}
	encoded, err := json.Marshal(grant)
	if err != nil {
		return Grant{}, err
	}
	return grant, s.storage.Set(keyPrefix+grant.ID, encoded)
}

func (s *Store) Authorize(ctx context.Context, path string) (Grant, error) {
	root, commonDir, err := inspectGitRoot(ctx, path)
	if err != nil {
		return Grant{}, err
	}
	digest := sha256.Sum256([]byte(commonDir))
	id := hex.EncodeToString(digest[:])
	grant, found, err := s.Load(id)
	if err != nil {
		return Grant{}, err
	}
	if !found || time.Now().After(grant.ExpiresAt) {
		return Grant{}, fmt.Errorf("workspace_not_granted: repository grant is absent or expired")
	}
	if grant.CommonDir != commonDir || (grant.RepositoryPath != root && !grant.IncludeGitWorktrees) {
		return Grant{}, fmt.Errorf("workspace_not_granted: worktree is not covered by the repository grant")
	}
	return grant, nil
}

func (s *Store) Load(id string) (Grant, bool, error) {
	if len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "" {
		return Grant{}, false, fmt.Errorf("invalid workspace grant ID")
	}
	data, err := s.storage.Get(keyPrefix + id)
	if err != nil || len(data) == 0 {
		return Grant{}, false, err
	}
	var grant Grant
	if err := json.Unmarshal(data, &grant); err != nil {
		return Grant{}, false, err
	}
	return grant, true, nil
}

func (s *Store) List() ([]Grant, error) {
	keys, err := s.storage.List(keyPrefix)
	if err != nil {
		return nil, err
	}
	grants := make([]Grant, 0, len(keys))
	for _, key := range keys {
		grant, found, err := s.Load(strings.TrimPrefix(key, keyPrefix))
		if err != nil {
			return nil, err
		}
		if found {
			grants = append(grants, grant)
		}
	}
	return grants, nil
}

func (s *Store) Revoke(id string) error {
	if _, _, err := s.Load(id); err != nil {
		return err
	}
	return s.storage.Delete(keyPrefix + id)
}

func inspectGitRoot(ctx context.Context, path string) (string, string, error) {
	root, err := canonicalOwnedRoot(path)
	if err != nil {
		return "", "", err
	}
	commonDir, err := gitCommonDirectory(ctx, root)
	if err != nil {
		return "", "", err
	}
	return root, commonDir, nil
}

func canonicalOwnedRoot(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("workspace path must be absolute")
	}
	clean := filepath.Clean(path)
	root, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", fmt.Errorf("workspace path: %w", err)
	}
	if root != clean {
		return "", fmt.Errorf("workspace symlink is not an approvable root")
	}
	if err := requireOwnedDirectory(root); err != nil {
		return "", err
	}
	return root, nil
}

func gitCommonDirectory(ctx context.Context, root string) (string, error) {
	command := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--show-toplevel", "--git-common-dir")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("workspace is not a Git worktree root: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 2 {
		return "", fmt.Errorf("unexpected Git worktree metadata")
	}
	gitRoot, err := filepath.EvalSymlinks(lines[0])
	if err != nil || gitRoot != root {
		return "", fmt.Errorf("workspace path is not the Git worktree root")
	}
	commonDir := lines[1]
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(root, commonDir)
	}
	commonDir, err = filepath.EvalSymlinks(commonDir)
	if err != nil {
		return "", fmt.Errorf("Git common directory: %w", err)
	}
	if err := requireOwnedDirectory(commonDir); err != nil {
		return "", err
	}
	return commonDir, nil
}
