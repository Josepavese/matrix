package agentmgr

import (
	"fmt"
	"os"
	"sort"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentstore"
)

// Verification methods. They say what was actually proven: a full tree
// verification of a store generation, or the evidence the per-home install
// recorded. The two are never conflated, because only the first re-reads bytes.
const (
	// VerifyMethodStoreTree re-read the published generation and proved its tree
	// matches the recorded manifest.
	VerifyMethodStoreTree = "store_tree"
	// VerifyMethodRecordedEvidence read the install evidence and checked the
	// launcher exists. The bytes were not re-proven offline.
	VerifyMethodRecordedEvidence = "recorded_evidence"
)

// VerifyResult is the outcome of verifying one installed agent.
type VerifyResult struct {
	ID       string `json:"id"`
	Method   string `json:"method"`
	Verified bool   `json:"verified"`
	Detail   string `json:"detail"`
}

// VerifyInstalled verifies the installed software of the named agents. An empty
// list verifies every registered agent. Unknown ids are reported as a failed
// result instead of an error: one bad name must not hide the others.
func (inst *Installer) VerifyInstalled(ids []string) ([]VerifyResult, error) {
	registered, err := agentcfg.ListAgentIDs(inst.storage)
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(registered))
	for _, id := range registered {
		known[id] = struct{}{}
	}
	targets := verifyTargets(ids, registered)
	results := make([]VerifyResult, 0, len(targets))
	for _, id := range targets {
		results = append(results, inst.verifyInstalledAgent(id, known))
	}
	return results, nil
}

// verifyTargets selects what to verify: the requested ids (deduplicated and
// sorted) or every registered agent when none were named.
func verifyTargets(ids, registered []string) []string {
	if len(ids) == 0 {
		targets := append([]string(nil), registered...)
		sort.Strings(targets)
		return targets
	}
	seen := make(map[string]struct{}, len(ids))
	targets := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		targets = append(targets, id)
	}
	sort.Strings(targets)
	return targets
}

func (inst *Installer) verifyInstalledAgent(agentID string, registered map[string]struct{}) VerifyResult {
	if _, ok := registered[agentID]; !ok {
		return VerifyResult{
			ID:     agentID,
			Method: VerifyMethodRecordedEvidence,
			Detail: fmt.Sprintf("agent %q is not registered in this Matrix instance, so it has no install to verify", agentID),
		}
	}
	entry, err := agentcfg.LoadEntry(inst.storage, agentID)
	if err != nil {
		return VerifyResult{ID: agentID, Method: VerifyMethodRecordedEvidence, Detail: fmt.Sprintf("cannot read the agent entry: %v", err)}
	}
	meta, err := agentcfg.LoadMeta(inst.storage, agentID)
	if err != nil {
		return VerifyResult{ID: agentID, Method: VerifyMethodRecordedEvidence, Detail: fmt.Sprintf("cannot read the agent metadata: %v", err)}
	}
	if meta.Reference != nil {
		return inst.verifyStoreCopy(agentID, meta.Reference)
	}
	return inst.verifyRecordedEvidence(agentID, entry.Config.Command, meta.ArtifactVerification)
}

// verifyStoreCopy proves a store-backed agent still matches the generation its
// reference names: the store must open read-only, the generation must be
// readable, its tree must verify, and the tree manifest must be the recorded one.
func (inst *Installer) verifyStoreCopy(agentID string, ref *agentcfg.ArtifactReference) VerifyResult {
	result := VerifyResult{ID: agentID, Method: VerifyMethodStoreTree}
	store, err := agentstore.OpenConsumer(ref.Store, inst.home)
	if err != nil {
		result.Detail = fmt.Sprintf("cannot open the recorded store %s as a consumer: %v", ref.Store, err)
		return result
	}
	generation, err := store.Get(ref.Digest)
	if err != nil {
		result.Detail = fmt.Sprintf("store generation %s is not readable: %v", ref.Digest, err)
		return result
	}
	if err := store.Verify(ref.Digest); err != nil {
		result.Detail = fmt.Sprintf("store generation %s failed tree verification: %v", ref.Digest, err)
		return result
	}
	if ref.TreeManifestSHA256 != "" && generation.TreeManifestSHA256 != ref.TreeManifestSHA256 {
		result.Detail = fmt.Sprintf("store generation %s records tree manifest %s, but the agent reference names %s; the generation is not the one that was installed", generation.Digest, generation.TreeManifestSHA256, ref.TreeManifestSHA256)
		return result
	}
	result.Verified = true
	result.Detail = fmt.Sprintf("store generation %s verified against its recorded tree manifest %s", generation.Digest, generation.TreeManifestSHA256)
	return result
}

// verifyRecordedEvidence reports what the per-home install recorded. It checks
// the launcher exists because a record whose program is gone is not an
// installation, but it cannot re-prove the bytes offline and says so.
func (inst *Installer) verifyRecordedEvidence(agentID, launcher string, verification *agentcfg.ArtifactVerification) VerifyResult {
	result := VerifyResult{ID: agentID, Method: VerifyMethodRecordedEvidence}
	launcherDetail := inst.launcherEvidence(launcher)
	if verification == nil {
		result.Detail = fmt.Sprintf("no artifact verification was recorded for this per-home install (launcher %s); the bytes were not re-proven offline", launcherDetail)
		return result
	}
	if !verification.Verified {
		result.Detail = fmt.Sprintf("recorded install evidence says not verified: %s (launcher %s); the bytes were not re-proven offline", verification.Describe(), launcherDetail)
		return result
	}
	if launcherDetail != "present" {
		result.Detail = fmt.Sprintf("the recorded install evidence says the artifact was sha256-verified, but the launcher %s; the bytes were not re-proven offline", launcherDetail)
		return result
	}
	result.Verified = true
	result.Detail = fmt.Sprintf("the recorded install evidence says the artifact was sha256-verified for %s; the bytes were not re-proven offline (per-home launcher present)", verification.Platform)
	return result
}

// launcherEvidence describes the launcher the vault entry names: "present" only
// when it is a real, non-directory file.
func (inst *Installer) launcherEvidence(launcher string) string {
	if launcher == "" {
		return "is empty"
	}
	info, err := inst.fs.Stat(launcher)
	if err != nil {
		if os.IsNotExist(err) {
			return "is missing"
		}
		return fmt.Sprintf("cannot be inspected (%v)", err)
	}
	if info.IsDir() {
		return "is a directory, not a program"
	}
	return "present"
}
