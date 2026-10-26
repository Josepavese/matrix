package agentmgr

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentstore"
)

// Restore actions. They are the complete vocabulary the CLI prints: an install
// either happened, would happen (dry run), was unnecessary, was refused for a
// version difference, did not apply, or failed.
const (
	RestoreActionInstalled      = "installed"
	RestoreActionWouldInstall   = "would_install"
	RestoreActionSkippedPresent = "skipped_present"
	RestoreActionDrift          = "drift"
	RestoreActionNotApplicable  = "not_applicable"
	RestoreActionFailed         = "failed"
)

// RestoreOptions selects how an inventory is reinstalled.
type RestoreOptions struct {
	// StoreRoot is the shared artifact store to install store-backed agents from.
	// Empty falls back to the governed registry for every agent.
	StoreRoot string
	// AllowVersionDrift accepts a registry version different from the one the
	// inventory recorded instead of refusing the install.
	AllowVersionDrift bool
	// Replace reinstalls agents that are already registered in this home.
	Replace bool
	// DryRun reports the action without downloading, writing, or installing.
	DryRun bool
}

// RestoreResult is the outcome for one inventory agent.
type RestoreResult struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Restore reinstalls the regenerable software an inventory describes. It is the
// other half of excluding regenerable agents from a data backup: the inventory
// says what was installed and this path rebuilds it from a governed source.
//
// It never installs from an arbitrary URL: a store-backed agent comes from its
// verified generation and everything else comes from the governed registry.
func (inst *Installer) Restore(ctx context.Context, inv Inventory, opts RestoreOptions) ([]RestoreResult, error) {
	registered, err := agentcfg.ListAgentIDs(inst.storage)
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(registered))
	for _, id := range registered {
		known[id] = struct{}{}
	}
	results := make([]RestoreResult, 0, len(inv.Agents))
	for _, agent := range inv.Agents {
		results = append(results, inst.restoreAgent(ctx, agent, known, opts))
	}
	return results, nil
}

func (inst *Installer) restoreAgent(ctx context.Context, agent InventoryAgent, registered map[string]struct{}, opts RestoreOptions) RestoreResult {
	if !agent.Regenerable {
		return RestoreResult{
			ID:     agent.ID,
			Action: RestoreActionNotApplicable,
			Detail: "no software to restore: this agent is a seeded definition or a remote endpoint",
		}
	}
	if _, present := registered[agent.ID]; present && !opts.Replace {
		return RestoreResult{
			ID:     agent.ID,
			Action: RestoreActionSkippedPresent,
			Detail: "already registered in this home; pass --replace to reinstall it",
		}
	}
	if agent.Store != nil && strings.TrimSpace(opts.StoreRoot) != "" {
		return inst.restoreFromStore(ctx, agent, opts)
	}
	return inst.restoreFromRegistry(ctx, agent, opts)
}

// restoreFromStore reinstalls an agent from the exact generation its reference
// names. The generation is read through a consumer store, so a writable store is
// refused before anything is written.
func (inst *Installer) restoreFromStore(ctx context.Context, agent InventoryAgent, opts RestoreOptions) RestoreResult {
	result := RestoreResult{ID: agent.ID}
	store, err := agentstore.OpenConsumer(opts.StoreRoot, inst.home)
	if err != nil {
		return restoreFailed(result, err)
	}
	generation, err := store.Get(agent.Store.Digest)
	if err != nil {
		return restoreFailed(result, err)
	}
	if generation.AgentID != agent.ID {
		return restoreFailed(result, fmt.Errorf("store generation %s belongs to agent %q, not %q", generation.Digest, generation.AgentID, agent.ID))
	}
	if opts.DryRun {
		result.Action = RestoreActionWouldInstall
		result.Detail = fmt.Sprintf("would install from store %s generation %s", store.Root(), generation.Digest)
		return result
	}
	if err := inst.InstallFromStore(ctx, agent.ID, opts.StoreRoot, generation.Digest); err != nil {
		return restoreFailed(result, fmt.Errorf("install from store generation %s: %w", generation.Digest, err))
	}
	result.Action = RestoreActionInstalled
	result.Detail = fmt.Sprintf("installed from store %s generation %s", store.Root(), generation.Digest)
	return result
}

// restoreFromRegistry reinstalls an agent from the governed registry, but only at
// the version the inventory recorded: a newer release is a different artifact,
// and adopting it silently would make a restore a version migration.
func (inst *Installer) restoreFromRegistry(ctx context.Context, agent InventoryAgent, opts RestoreOptions) RestoreResult {
	result := RestoreResult{ID: agent.ID}
	if inst.registry == nil {
		return restoreFailed(result, fmt.Errorf("this installer has no registry client, so %q cannot be restored from the governed registry", agent.ID))
	}
	manifest, err := inst.registry.FetchManifest(ctx, agent.ID)
	if err != nil {
		return restoreFailed(result, fmt.Errorf("cannot fetch the governed registry manifest: %w", err))
	}
	if manifest.Version != agent.Version && !opts.AllowVersionDrift {
		result.Action = RestoreActionDrift
		result.Detail = fmt.Sprintf("the registry now publishes version %q, but the inventory recorded %q; pass --allow-version-drift to install the published version", manifest.Version, agent.Version)
		return result
	}
	if opts.DryRun {
		result.Action = RestoreActionWouldInstall
		result.Detail = fmt.Sprintf("would install version %q from the governed registry", manifest.Version)
		return result
	}
	if err := inst.Install(ctx, agent.ID); err != nil {
		return restoreFailed(result, err)
	}
	result.Action = RestoreActionInstalled
	result.Detail = fmt.Sprintf("installed version %q from the governed registry", manifest.Version)
	return result
}

func restoreFailed(result RestoreResult, err error) RestoreResult {
	result.Action = RestoreActionFailed
	result.Error = err.Error()
	return result
}
