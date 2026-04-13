package agentmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

// RegistryIndex represents the top-level structure of the ACP Registry CDN.
type RegistryIndex struct {
	Version string           `json:"version"`
	Agents  []AgentManifest `json:"agents"`
}

// AgentManifest holds all metadata and distribution info for a registry agent.
type AgentManifest struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Version      string               `json:"version"`
	Description  string               `json:"description"`
	Repository   string               `json:"repository,omitempty"`
	Website      string               `json:"website,omitempty"`
	Authors      []string             `json:"authors,omitempty"`
	License      string               `json:"license,omitempty"`
	Icon         string               `json:"icon,omitempty"`
	Distribution RegistryDistribution `json:"distribution"`
}

// DistTypes returns the available distribution types for this agent.
func (m *AgentManifest) DistTypes() []string {
	var types []string
	if len(m.Distribution.Binary) > 0 {
		types = append(types, "binary")
	}
	if m.Distribution.Npx != nil {
		types = append(types, "npx")
	}
	if m.Distribution.Uvx != nil {
		types = append(types, "uvx")
	}
	return types
}

// RegistryDistribution holds all distribution types for a given agent.
type RegistryDistribution struct {
	Binary map[string]BinaryDist `json:"binary,omitempty"`
	Npx    *NpxDist              `json:"npx,omitempty"`
	Uvx    *UvxDist              `json:"uvx,omitempty"`
}

// BinaryDist represents a platform-specific binary distribution.
type BinaryDist struct {
	Archive string   `json:"archive"`
	Cmd     string   `json:"cmd"`
	Args    []string `json:"args,omitempty"`
}

// NpxDist represents an NPX-based distribution.
type NpxDist struct {
	Package string            `json:"package"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// UvxDist represents a UVX-based distribution.
type UvxDist struct {
	Package string            `json:"package"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// ResolvedDist represents a resolved distribution ready for installation.
type ResolvedDist struct {
	Type    string   // "binary", "npx", "uvx"
	Command string   // "" for binary (resolved later), "npx"/"uvx" otherwise
	Args    []string
	Env     []string // flattened KEY=VALUE pairs
}

// registryCache holds a cached registry index with fetch timestamp.
type registryCache struct {
	FetchedAt time.Time     `json:"fetched_at"`
	Index     RegistryIndex `json:"index"`
}

const defaultRegistryURL = "https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json"
const cacheKey = "registry.cache.index"

// RegistryClient interacts with the remote ACP Agent Registry.
type RegistryClient struct {
	net         middleware.Network
	registryURL string
	storage     middleware.Storage // optional, for caching
	cacheTTL    time.Duration
	goos        string // injected platform, defaults to runtime.GOOS
	arch        string // injected platform, defaults to runtime.GOARCH
}

// NewRegistryClient creates a client without caching.
func NewRegistryClient(net middleware.Network, registryURL string) *RegistryClient {
	if registryURL == "" {
		registryURL = defaultRegistryURL
	}
	return &RegistryClient{
		net:         net,
		registryURL: registryURL,
		cacheTTL:    time.Hour,
		goos:        runtime.GOOS,
		arch:        runtime.GOARCH,
	}
}

// NewCachingRegistryClient creates a client that caches the registry index in the vault.
func NewCachingRegistryClient(net middleware.Network, registryURL string, storage middleware.Storage) *RegistryClient {
	if registryURL == "" {
		registryURL = defaultRegistryURL
	}
	return &RegistryClient{
		net:         net,
		registryURL: registryURL,
		storage:     storage,
		cacheTTL:    time.Hour,
		goos:        runtime.GOOS,
		arch:        runtime.GOARCH,
	}
}

// FetchIndex returns the full registry index from the network.
func (c *RegistryClient) FetchIndex(ctx context.Context) (RegistryIndex, error) {
	var index RegistryIndex
	if err := c.net.FetchJSON(ctx, c.registryURL, &index); err != nil {
		return RegistryIndex{}, fmt.Errorf("failed to fetch registry index: %w", err)
	}
	return index, nil
}

// FetchIndexCached tries the cache first, falls back to network, then stale cache.
func (c *RegistryClient) FetchIndexCached(ctx context.Context) (RegistryIndex, error) {
	// Try fresh cache
	if c.storage != nil {
		if cached, ok := c.loadCache(); ok {
			if time.Since(cached.FetchedAt) < c.cacheTTL {
				return cached.Index, nil
			}
		}
	}

	// Fetch from network
	index, err := c.FetchIndex(ctx)
	if err != nil {
		// Return stale cache if available
		if c.storage != nil {
			if cached, ok := c.loadCache(); ok {
				return cached.Index, nil
			}
		}
		return RegistryIndex{}, err
	}

	// Store in cache
	if c.storage != nil {
		c.saveCache(index)
	}

	return index, nil
}

// FetchManifest returns the manifest for a specific agent (no cache).
func (c *RegistryClient) FetchManifest(ctx context.Context, agentID string) (*AgentManifest, error) {
	index, err := c.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	return findAgent(index.Agents, agentID)
}

// FetchManifestCached returns the manifest for a specific agent using the cache.
func (c *RegistryClient) FetchManifestCached(ctx context.Context, agentID string) (*AgentManifest, error) {
	index, err := c.FetchIndexCached(ctx)
	if err != nil {
		return nil, err
	}
	return findAgent(index.Agents, agentID)
}

// ResolveDistribution finds the best binary distribution for the current host.
func (c *RegistryClient) ResolveDistribution(manifest *AgentManifest) (*BinaryDist, error) {
	goos := c.goos
	arch := c.arch

	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	}

	platform := fmt.Sprintf("%s-%s", goos, arch)
	dist, ok := manifest.Distribution.Binary[platform]
	if !ok {
		return nil, fmt.Errorf("no compatible binary distribution found for platform %s", platform)
	}
	return &dist, nil
}

// ResolveAnyDistribution tries binary first, then falls back to npx/uvx.
func (c *RegistryClient) ResolveAnyDistribution(manifest *AgentManifest) (*ResolvedDist, error) {
	// Try binary first
	if _, err := c.ResolveDistribution(manifest); err == nil {
		return &ResolvedDist{Type: "binary"}, nil
	}

	// Fallback to npx
	if manifest.Distribution.Npx != nil {
		npx := manifest.Distribution.Npx
		return &ResolvedDist{
			Type:    "npx",
			Command: "npx",
			Args:    append([]string{"-y", npx.Package}, npx.Args...),
			Env:     flattenEnvMap(npx.Env),
		}, nil
	}

	// Fallback to uvx
	if manifest.Distribution.Uvx != nil {
		uvx := manifest.Distribution.Uvx
		return &ResolvedDist{
			Type:    "uvx",
			Command: "uvx",
			Args:    append([]string{uvx.Package}, uvx.Args...),
			Env:     flattenEnvMap(uvx.Env),
		}, nil
	}

	return nil, fmt.Errorf("no compatible distribution found for agent '%s'", manifest.ID)
}

func findAgent(agents []AgentManifest, agentID string) (*AgentManifest, error) {
	for i := range agents {
		if agents[i].ID == agentID {
			return &agents[i], nil
		}
	}
	return nil, fmt.Errorf("agent '%s' not found in registry", agentID)
}

func (c *RegistryClient) loadCache() (registryCache, bool) {
	data, err := c.storage.Get(cacheKey)
	if err != nil || len(data) == 0 {
		return registryCache{}, false
	}
	var cache registryCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return registryCache{}, false
	}
	return cache, true
}

func (c *RegistryClient) saveCache(index RegistryIndex) {
	cache := registryCache{FetchedAt: time.Now(), Index: index}
	if data, err := json.Marshal(cache); err == nil {
		_ = c.storage.Set(cacheKey, data)
	}
}

func flattenEnvMap(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}
