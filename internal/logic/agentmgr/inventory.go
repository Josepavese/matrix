package agentmgr

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
)

// InventorySchemaVersion is the version of the inventory document. A consumer
// that excludes regenerable software from a backup reads it back, so the shape
// is versioned rather than inferred from the fields that happen to be present.
const InventorySchemaVersion = 1

// Inventory is the deterministic, secret-free description of the software this
// PAL home has installed. It carries no timestamps and no environment values:
// two homes with the same agents produce the same document, which is what lets a
// consumer compare inventories across workers and back up only the rest.
type Inventory struct {
	SchemaVersion int              `json:"schema_version"`
	Agents        []InventoryAgent `json:"agents"`
}

// InventoryArtifact is the install evidence reduced to the facts about the
// bytes. The recording time is deliberately absent: it is a fact about one home,
// and a backup contract compares inventories across homes.
type InventoryArtifact struct {
	Verified bool                                `json:"verified"`
	Status   agentcfg.ArtifactVerificationStatus `json:"status"`
	Platform string                              `json:"platform,omitempty"`
	Artifact string                              `json:"artifact,omitempty"`
	Expected string                              `json:"expected_sha256,omitempty"`
	Actual   string                              `json:"actual_sha256,omitempty"`
	Override string                              `json:"override,omitempty"`
}

// InventoryAgent is one installed agent. Regenerable software can be restored
// from the registry or from a verified store generation, so it can be excluded
// from a data backup; anything else has no source to reinstall from.
type InventoryAgent struct {
	ID             string             `json:"id"`
	Name           string             `json:"name,omitempty"`
	Version        string             `json:"version,omitempty"`
	Kind           string             `json:"kind,omitempty"`
	Transport      string             `json:"transport,omitempty"`
	Distribution   string             `json:"distribution,omitempty"`
	Launcher       string             `json:"launcher,omitempty"`
	Args           []string           `json:"args,omitempty"`
	EnvKeys        []string           `json:"env_keys,omitempty"`
	Artifact       *InventoryArtifact `json:"artifact,omitempty"`
	Store          *StoreReference    `json:"store,omitempty"`
	Regenerable    bool               `json:"regenerable"`
	RestoreCommand string             `json:"restore_command,omitempty"`
}

// StoreReference names the verified store generation an agent runs from. It is
// the inventory form of agentcfg.ArtifactReference: the install timestamp is left
// out because it would differ between homes for the same software.
type StoreReference struct {
	Store              string `json:"store"`
	Digest             string `json:"digest"`
	TreeManifestSHA256 string `json:"tree_manifest_sha256,omitempty"`
	Launcher           string `json:"launcher,omitempty"`
	Version            string `json:"version,omitempty"`
	Platform           string `json:"platform,omitempty"`
}

// Inventory reads every registered agent from the vault and describes its
// software and recoverability. It is read-only and never returns secrets: the
// environment is reduced to names and the install evidence is reduced to its
// outcome.
func (inst *Installer) Inventory() (Inventory, error) {
	ids, err := agentcfg.ListAgentIDs(inst.storage)
	if err != nil {
		return Inventory{}, err
	}
	sort.Strings(ids)
	inventory := Inventory{SchemaVersion: InventorySchemaVersion, Agents: make([]InventoryAgent, 0, len(ids))}
	for _, id := range ids {
		agent, err := inst.inventoryAgent(id)
		if err != nil {
			return Inventory{}, err
		}
		inventory.Agents = append(inventory.Agents, agent)
	}
	return inventory, nil
}

// inventoryAgent describes one agent from its vault entry and metadata. A missing
// metadata record is not an error: seeds and remote endpoints have no artifact to
// describe and are reported as not regenerable.
func (inst *Installer) inventoryAgent(agentID string) (InventoryAgent, error) {
	entry, err := agentcfg.LoadEntry(inst.storage, agentID)
	if err != nil {
		return InventoryAgent{}, err
	}
	meta, err := agentcfg.LoadMeta(inst.storage, agentID)
	if err != nil {
		return InventoryAgent{}, fmt.Errorf("read metadata for agent %q: %w", agentID, err)
	}
	agent := InventoryAgent{
		ID:           agentID,
		Name:         displayName(agentID, meta.Name),
		Version:      meta.Version,
		Kind:         entry.Config.Kind,
		Transport:    entry.Config.Transport,
		Distribution: strings.Join(meta.DistTypes, ","),
		Launcher:     entry.Config.Command,
		Args:         append([]string(nil), entry.Config.Args...),
		EnvKeys:      envKeys(entry.Config.Env),
		Artifact:     recordedArtifact(meta.ArtifactVerification),
		Store:        storeReference(meta.Reference),
	}
	agent.Regenerable = agent.Store != nil || len(meta.DistTypes) > 0
	agent.RestoreCommand = restoreCommand(agent)
	return agent, nil
}

func displayName(agentID, name string) string {
	if strings.TrimSpace(name) == "" {
		return agentID
	}
	return name
}

// envKeys returns the sorted, unique names of the environment entries. Values
// never enter the inventory, not even truncated.
func envKeys(env []string) []string {
	seen := make(map[string]struct{}, len(env))
	keys := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	return keys
}

// recordedArtifact keeps the install evidence but not the time it was recorded:
// the evidence is a fact about the bytes, the timestamp is a fact about this home.
func recordedArtifact(verification *agentcfg.ArtifactVerification) *InventoryArtifact {
	if verification == nil {
		return nil
	}
	return &InventoryArtifact{
		Verified: verification.Verified, Status: verification.Status, Platform: verification.Platform,
		Artifact: verification.Artifact, Expected: verification.Expected, Actual: verification.Actual,
		Override: verification.Override,
	}
}

func storeReference(reference *agentcfg.ArtifactReference) *StoreReference {
	if reference == nil {
		return nil
	}
	return &StoreReference{
		Store:              reference.Store,
		Digest:             reference.Digest,
		TreeManifestSHA256: reference.TreeManifestSHA256,
		Launcher:           reference.Launcher,
		Version:            reference.Version,
		Platform:           reference.Platform,
	}
}

// restoreCommand is the command the operator can run to reinstall this agent. It
// is empty for software with no source: a seeded definition or a remote endpoint
// has nothing to reinstall.
func restoreCommand(agent InventoryAgent) string {
	if !agent.Regenerable {
		return ""
	}
	command := "matrix install " + agent.ID
	if agent.Store != nil {
		command += " --store " + agent.Store.Store + " --digest " + agent.Store.Digest
	}
	return command
}
