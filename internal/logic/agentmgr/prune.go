package agentmgr

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentinstall"
	"github.com/Josepavese/matrix/internal/logic/agentstore"
)

// PruneResult reports one agent's per-home software copy and what happened to it.
type PruneResult struct {
	ID       string `json:"id"`
	AgentDir string `json:"agent_dir,omitempty"`
	Bytes    int64  `json:"bytes"`
	Removed  bool   `json:"removed"`
	Detail   string `json:"detail,omitempty"`
}

// PruneCopy reclaims the per-home software copy of an agent whose verified
// software lives in a shared store. Reclaiming is only safe once the store
// generation has verified: the copy is what makes the agent runnable today, so a
// store that no longer verifies must keep it in place.
//
// It never touches configuration or the vault: the point is to give disk space
// back to a backup, not to uninstall the agent.
func (inst *Installer) PruneCopy(agentID string, apply bool) (PruneResult, error) {
	result := PruneResult{ID: agentID}
	meta, err := agentcfg.LoadMeta(inst.storage, agentID)
	if err != nil {
		return result, fmt.Errorf("cannot read metadata for agent %q: %w", agentID, err)
	}
	if meta.Reference == nil {
		return result, fmt.Errorf("agent %q has no verified store reference, so it has no verified replacement; refusing to reclaim its per-home copy (install it from a shared store first with `matrix install %s --store <root>`)", agentID, agentID)
	}
	if strings.TrimSpace(inst.home) == "" {
		return result, fmt.Errorf("this installer has no PAL home, so it cannot resolve the agent reference for %q", agentID)
	}
	if err := inst.requireStoreGeneration(agentID, meta.Reference); err != nil {
		return result, err
	}
	agentDir, err := agentinstall.AgentDir(inst.baseDir, agentID)
	if err != nil {
		return result, err
	}
	result.AgentDir = agentDir
	result.Bytes, err = softwareDirBytes(agentDir)
	if err != nil {
		return result, fmt.Errorf("cannot measure the per-home copy at %s: %w", agentDir, err)
	}
	if result.Bytes == 0 {
		result.Detail = fmt.Sprintf("no per-home copy at %s; store generation %s verified, nothing to reclaim", agentDir, meta.Reference.Digest)
		return result, nil
	}
	if !apply {
		result.Detail = fmt.Sprintf("would remove %d bytes from %s; store generation %s verified", result.Bytes, agentDir, meta.Reference.Digest)
		return result, nil
	}
	if err := inst.fs.RemoveAll(agentDir); err != nil {
		return result, fmt.Errorf("cannot remove the per-home copy at %s: %w", agentDir, err)
	}
	result.Removed = true
	// The reference deliberately stays: the agent remains installed and its Vault
	// record still points at the store generation, so the reference is what keeps
	// a later `agent store gc` from reclaiming the bytes the agent needs.
	result.Detail = fmt.Sprintf("removed %d bytes from %s; store generation %s verified; configs, vault and store reference untouched", result.Bytes, agentDir, meta.Reference.Digest)
	return result, nil
}

// requireStoreGeneration proves the recorded generation still verifies before
// anything on disk is touched.
func (inst *Installer) requireStoreGeneration(agentID string, ref *agentcfg.ArtifactReference) error {
	store, err := agentstore.OpenConsumer(ref.Store, inst.home)
	if err != nil {
		return fmt.Errorf("cannot open the recorded store %s as a consumer, so the per-home copy of %q must be kept: %w", ref.Store, agentID, err)
	}
	if err := store.Verify(ref.Digest); err != nil {
		return fmt.Errorf("store generation %s does not verify, so the per-home copy of %q must be kept: %w", ref.Digest, agentID, err)
	}
	return nil
}

// softwareDirBytes sums the size of the regular files under a per-home agent
// directory. A missing directory is zero bytes, not an error.
func softwareDirBytes(dir string) (int64, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("%s is not a directory", dir)
	}
	var total int64
	err = filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		total += fileInfo.Size()
		return nil
	})
	return total, err
}
