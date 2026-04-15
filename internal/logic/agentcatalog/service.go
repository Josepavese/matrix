package agentcatalog

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/agentcfg"
	"github.com/jose/matrix-v2/internal/logic/agentdiscovery"
	"github.com/jose/matrix-v2/internal/logic/agentmgr"
	"github.com/jose/matrix-v2/internal/middleware"
)

type Entry struct {
	ID              string                  `json:"id"`
	Name            string                  `json:"name"`
	Description     string                  `json:"description,omitempty"`
	Version         string                  `json:"version,omitempty"`
	ProtocolVersion string                  `json:"protocol_version,omitempty"`
	Source          agentdiscovery.Source   `json:"source"`
	Kind            middleware.ProtocolKind `json:"kind"`
	Transport       string                  `json:"transport,omitempty"`
	Address         string                  `json:"address,omitempty"`
	CardURL         string                  `json:"card_url,omitempty"`
	DistTypes       []string                `json:"dist_types,omitempty"`
	Installed       bool                    `json:"installed"`
}

type Discovery interface {
	List(ctx context.Context) ([]Entry, error)
}

type Activator interface {
	Activate(ctx context.Context, entry Entry) error
}

type Config struct {
	Storage        middleware.Storage
	Net            middleware.Network
	Installer      *agentmgr.Installer
	Sources        []agentdiscovery.Source
	RegistryURL    string
	A2ACatalogURLs []string
}

type Service struct {
	storage        middleware.Storage
	net            middleware.Network
	installer      *agentmgr.Installer
	sources        []agentdiscovery.Source
	registryURL    string
	a2aCatalogURLs []string
}

func NewService(cfg Config) *Service {
	sources := append([]agentdiscovery.Source{}, cfg.Sources...)
	if len(sources) == 0 {
		sources = DefaultSources()
	}
	return &Service{
		storage:        cfg.Storage,
		net:            cfg.Net,
		installer:      cfg.Installer,
		sources:        sources,
		registryURL:    cfg.RegistryURL,
		a2aCatalogURLs: append([]string{}, cfg.A2ACatalogURLs...),
	}
}

func DefaultSources() []agentdiscovery.Source {
	return []agentdiscovery.Source{
		agentdiscovery.SourceLocal,
		agentdiscovery.SourceACPRegistry,
	}
}

func (s *Service) List(ctx context.Context) ([]Entry, error) {
	installed := s.installedIDSet()
	merged := map[string]Entry{}

	for _, source := range s.sources {
		records, err := s.listSource(ctx, source)
		if err != nil {
			if source == agentdiscovery.SourceLocal {
				return nil, err
			}
			continue
		}
		for _, record := range records {
			record.Installed = installed[record.ID]
			current, ok := merged[record.ID]
			if !ok {
				merged[record.ID] = record
				continue
			}
			merged[record.ID] = mergeEntry(current, record)
		}
	}

	if len(merged) == 0 {
		return nil, nil
	}
	entries := make([]Entry, 0, len(merged))
	for _, entry := range merged {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries, nil
}

func (s *Service) Activate(ctx context.Context, entry Entry) error {
	if entry.Installed {
		return nil
	}
	switch entry.Source {
	case agentdiscovery.SourceACPRegistry:
		if s.installer == nil {
			return fmt.Errorf("ACP activation requires installer")
		}
		return s.installer.Install(ctx, entry.ID)
	case agentdiscovery.SourceA2ACatalog, agentdiscovery.SourceA2ACard:
		return RegisterRemote(s.storage, entry)
	case agentdiscovery.SourceLocal:
		return nil
	default:
		return fmt.Errorf("unsupported activation source %q", entry.Source)
	}
}

func RegisterRemote(storage middleware.Storage, entry Entry) error {
	if storage == nil {
		return fmt.Errorf("storage not available")
	}
	if strings.TrimSpace(entry.ID) == "" {
		return fmt.Errorf("agent ID is required")
	}
	if entry.Kind == "" {
		return fmt.Errorf("protocol kind is required for remote registration")
	}
	transport := strings.TrimSpace(entry.Transport)
	if transport == "" {
		switch entry.Kind {
		case middleware.ProtocolKindA2A:
			transport = "JSONRPC"
		case middleware.ProtocolKindACP:
			transport = "ws"
		}
	}

	current, err := agentcfg.LoadEntry(storage, entry.ID)
	if err != nil {
		return err
	}
	current.Config.Kind = string(entry.Kind)
	current.Config.Protocol = string(entry.Kind)
	current.Config.Transport = transport
	current.Config.Address = strings.TrimSpace(entry.Address)
	current.Config.CardURL = strings.TrimSpace(entry.CardURL)
	current.Config.ProtocolVersion = strings.TrimSpace(entry.ProtocolVersion)
	current.Config.Command = ""
	current.Config.Args = nil
	current.Config.Env = nil
	current.Config.EnvIsolation = false

	if err := agentcfg.SaveEntry(storage, entry.ID, current); err != nil {
		return err
	}

	meta := agentcfg.Meta{
		ID:          entry.ID,
		Name:        entry.Name,
		Description: entry.Description,
		Version:     entry.Version,
		DistTypes:   append([]string{}, entry.DistTypes...),
	}
	return agentcfg.SaveMeta(storage, entry.ID, meta)
}

func (s *Service) listSource(ctx context.Context, source agentdiscovery.Source) ([]Entry, error) {
	if source == agentdiscovery.SourceLocal {
		return s.listLocal(), nil
	}

	opts := agentdiscovery.Options{
		Net:         s.net,
		Storage:     s.storage,
		RegistryURL: s.registryURL,
	}
	switch source {
	case agentdiscovery.SourceACPRegistry:
		provider, err := agentdiscovery.NewProvider(source, opts)
		if err != nil {
			return nil, err
		}
		records, err := provider.Search(ctx, "")
		if err != nil {
			return nil, err
		}
		return entriesFromRecords(records), nil
	case agentdiscovery.SourceA2ACatalog:
		entries := make([]Entry, 0)
		for _, catalogURL := range s.a2aCatalogURLs {
			provider, err := agentdiscovery.NewProvider(source, agentdiscovery.Options{
				Net:        s.net,
				CatalogURL: catalogURL,
			})
			if err != nil {
				return nil, err
			}
			records, err := provider.Search(ctx, "")
			if err != nil {
				return nil, err
			}
			entries = append(entries, entriesFromRecords(records)...)
		}
		return entries, nil
	default:
		return nil, fmt.Errorf("list unsupported for discovery source %q", source)
	}
}

func (s *Service) listLocal() []Entry {
	ids, err := agentcfg.ListMetaIDs(s.storage)
	if err != nil {
		return nil
	}
	entries := make([]Entry, 0, len(ids))
	for _, id := range ids {
		meta, _ := agentcfg.LoadMeta(s.storage, id)
		entry, _ := agentcfg.LoadEntry(s.storage, id)
		endpoint := agentcfg.NormalizeEndpoint(entry.Config)
		address := endpoint.Address
		if endpoint.Kind == middleware.ProtocolKindACP && endpoint.Transport == "stdio" {
			address = endpoint.Command
		}
		entries = append(entries, Entry{
			ID:              id,
			Name:            firstNonEmpty(meta.Name, id),
			Description:     meta.Description,
			Version:         meta.Version,
			ProtocolVersion: endpoint.ProtocolVersion,
			Source:          agentdiscovery.SourceLocal,
			Kind:            endpoint.Kind,
			Transport:       endpoint.Transport,
			Address:         address,
			CardURL:         endpoint.CardURL,
			DistTypes:       append([]string{}, meta.DistTypes...),
			Installed:       true,
		})
	}
	return entries
}

func (s *Service) installedIDSet() map[string]bool {
	ids, err := agentcfg.ListMetaIDs(s.storage)
	if err != nil {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func entriesFromRecords(records []agentdiscovery.Record) []Entry {
	entries := make([]Entry, 0, len(records))
	for _, record := range records {
		entries = append(entries, Entry{
			ID:              record.ID,
			Name:            firstNonEmpty(record.Name, record.ID),
			Description:     record.Description,
			Version:         record.Version,
			ProtocolVersion: record.ProtocolVersion,
			Source:          record.Source,
			Kind:            record.Kind,
			Transport:       record.Transport,
			Address:         record.Address,
			CardURL:         record.CardURL,
			DistTypes:       append([]string{}, record.Distribution...),
		})
	}
	return entries
}

func mergeEntry(current, candidate Entry) Entry {
	if current.Source == agentdiscovery.SourceLocal {
		current.Installed = current.Installed || candidate.Installed
		if len(current.DistTypes) == 0 && len(candidate.DistTypes) > 0 {
			current.DistTypes = append([]string{}, candidate.DistTypes...)
		}
		return current
	}
	if candidate.Source == agentdiscovery.SourceLocal {
		candidate.Installed = candidate.Installed || current.Installed
		if len(candidate.DistTypes) == 0 && len(current.DistTypes) > 0 {
			candidate.DistTypes = append([]string{}, current.DistTypes...)
		}
		return candidate
	}
	current.Installed = current.Installed || candidate.Installed
	if current.Name == "" {
		current.Name = candidate.Name
	}
	if current.Description == "" {
		current.Description = candidate.Description
	}
	if current.Version == "" {
		current.Version = candidate.Version
	}
	if current.ProtocolVersion == "" {
		current.ProtocolVersion = candidate.ProtocolVersion
	}
	if current.Kind == "" {
		current.Kind = candidate.Kind
	}
	if current.Transport == "" {
		current.Transport = candidate.Transport
	}
	if current.Address == "" {
		current.Address = candidate.Address
	}
	if current.CardURL == "" {
		current.CardURL = candidate.CardURL
	}
	if len(current.DistTypes) == 0 && len(candidate.DistTypes) > 0 {
		current.DistTypes = append([]string{}, candidate.DistTypes...)
	}
	return current
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
