package agentmgr

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentstore"
)

// Verification methods. They say what was actually proven: a full tree
// verification of a store generation, the same tree verification in a store
// whose role cannot be trusted, or the evidence the per-home install recorded.
// The three are never conflated, because only the first re-reads bytes and only
// the first is a guarantee that survives the check.
const (
	// VerifyMethodStoreTree re-read the published generation and proved its tree
	// matches the recorded manifest. The store is read-only for the runtime user,
	// so the proof survives the check.
	VerifyMethodStoreTree = "store_tree"
	// VerifyMethodStoreWritable re-read the same tree and found it matching, but
	// the store layout is writable by the runtime user: those bytes can be
	// swapped after the check, so the match is a point-in-time observation and
	// never a guarantee. The bytes are not wrong; the store's role is.
	VerifyMethodStoreWritable = "store_writable"
	// VerifyMethodRecordedEvidence read the install evidence and checked the
	// launcher exists. The bytes were not re-proven offline.
	VerifyMethodRecordedEvidence = "recorded_evidence"
)

// storeRoleRemedy is the deployment fix for a store the runtime user can write.
// It is offered wherever that role is reported, because the failure is about the
// deployment and not about the agent.
const storeRoleRemedy = "publish from a different account or mount the store read-only for the runtime user, then run this check again"

// VerifyResult is the outcome of verifying one installed agent.
//
// Remedy names the command sequence or deployment change that would turn this
// result into a re-proven one, when Matrix knows one. It is empty when the
// result needs no remedy, and it never claims a fix Matrix cannot perform.
type VerifyResult struct {
	ID       string `json:"id"`
	Method   string `json:"method"`
	Verified bool   `json:"verified"`
	Detail   string `json:"detail"`
	Remedy   string `json:"remedy,omitempty"`
}

// storeProof says how a generation that verified must be reported: under which
// method, with which remedy, and whether the result survives the store being
// modified after the check.
type storeProof struct {
	Method  string
	Remedy  string
	Durable bool
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
	return inst.verifyRecordedEvidence(agentID, entry.Config.Command, meta)
}

// verifyStoreCopy proves a store-backed agent still matches the generation its
// reference names. A sealed store is opened in the consumer role and a match is
// a guarantee. A store this user can write is still measured, because "the store
// role is wrong" and "the bytes are wrong" are different facts that need
// different remedies, but the match is reported as a point-in-time observation.
func (inst *Installer) verifyStoreCopy(agentID string, ref *agentcfg.ArtifactReference) VerifyResult {
	store, err := agentstore.OpenConsumer(ref.Store, inst.home)
	if err == nil {
		return proveStoreGeneration(store, agentID, ref, storeProof{Method: VerifyMethodStoreTree, Durable: true})
	}
	if !errors.Is(err, agentstore.ErrLayoutWritable) {
		return VerifyResult{
			ID:     agentID,
			Method: VerifyMethodStoreTree,
			Detail: fmt.Sprintf("cannot open the recorded store %s as a consumer: %v", ref.Store, err),
		}
	}
	inspection, inspectErr := agentstore.OpenInspection(ref.Store, inst.home)
	if inspectErr != nil {
		return VerifyResult{
			ID:     agentID,
			Method: VerifyMethodStoreWritable,
			Detail: fmt.Sprintf("the recorded store %s is writable by the current user and cannot be inspected: %v", ref.Store, inspectErr),
			Remedy: storeRoleRemedy,
		}
	}
	return proveStoreGeneration(inspection, agentID, ref, storeProof{Method: VerifyMethodStoreWritable, Remedy: storeRoleRemedy})
}

// proveStoreGeneration proves the generation a reference names: it must be
// readable, it must verify against its published tree manifest, and that
// manifest must be the one the agent reference recorded. A proof the store's own
// writability undermines is reported as measured but never as verified.
func proveStoreGeneration(store *agentstore.Store, agentID string, ref *agentcfg.ArtifactReference, proof storeProof) VerifyResult {
	result := VerifyResult{ID: agentID, Method: proof.Method, Remedy: proof.Remedy}
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
	if !proof.Durable {
		result.Detail = fmt.Sprintf("store generation %s matches its recorded tree manifest %s at the time of this check, but the store layout is writable by the current user, so the match cannot be relied on: those bytes can be swapped after the check", generation.Digest, generation.TreeManifestSHA256)
		return result
	}
	result.Verified = true
	result.Detail = fmt.Sprintf("store generation %s verified against its recorded tree manifest %s", generation.Digest, generation.TreeManifestSHA256)
	return result
}

// verifyRecordedEvidence reports what the per-home install recorded. It checks
// the launcher exists because a record whose program is gone is not an
// installation, but it cannot re-prove the bytes offline and says so. The remedy
// names the only path Matrix offers from that evidence to a re-proven tree.
func (inst *Installer) verifyRecordedEvidence(agentID, launcher string, meta agentcfg.Meta) VerifyResult {
	result := VerifyResult{ID: agentID, Method: VerifyMethodRecordedEvidence, Remedy: perHomeRemedy(meta.DistTypes)}
	launcherDetail := inst.launcherEvidence(launcher)
	verification := meta.ArtifactVerification
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

// perHomeRemedy names the only path Matrix offers from recorded evidence to a
// re-proven tree: publish the verified registry artifact into a shared store and
// reinstall from it. An agent installed by its package manager has no archive to
// publish, and an agent registered by path was never installed from a registry
// artifact: the remedy says so instead of promising a command that cannot work.
func perHomeRemedy(distTypes []string) string {
	switch {
	case slices.Contains(distTypes, "binary"):
		return "publish the verified registry artifact into a shared store and reinstall from it: `matrix agent store publish <agent-id> --store <root>`, then `matrix install <agent-id> --store <root>`"
	case slices.Contains(distTypes, "npx"), slices.Contains(distTypes, "uvx"):
		return "this agent is installed by its package manager, so there is no archive for Matrix to publish or to re-prove; a shared store cannot carry it"
	default:
		return "this agent was registered by path, not installed from a registry artifact, so there is no recorded artifact for Matrix to re-prove"
	}
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
