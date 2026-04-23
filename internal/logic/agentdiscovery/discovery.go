package agentdiscovery

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/Josepavese/matrix/internal/middleware"
	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
)

type Source string

const (
	SourceLocal       Source = "local"
	SourceACPRegistry Source = "acp_registry"
	SourceA2ACard     Source = "a2a_card"
	SourceA2ACatalog  Source = "a2a_catalog"
)

var ErrSearchUnsupported = errors.New("search unsupported for discovery source")

type Record struct {
	ID              string                  `json:"id"`
	Name            string                  `json:"name,omitempty"`
	Description     string                  `json:"description,omitempty"`
	Version         string                  `json:"version,omitempty"`
	ProtocolVersion string                  `json:"protocol_version,omitempty"`
	Source          Source                  `json:"source"`
	Kind            middleware.ProtocolKind `json:"kind"`
	Transport       string                  `json:"transport,omitempty"`
	Address         string                  `json:"address,omitempty"`
	CardURL         string                  `json:"card_url,omitempty"`
	Website         string                  `json:"website,omitempty"`
	Repository      string                  `json:"repository,omitempty"`
	License         string                  `json:"license,omitempty"`
	Authors         []string                `json:"authors,omitempty"`
	Distribution    []string                `json:"distribution,omitempty"`
	Tags            []string                `json:"tags,omitempty"`
}

type Provider interface {
	Search(ctx context.Context, query string) ([]Record, error)
	Get(ctx context.Context, ref string) (*Record, error)
}

type Options struct {
	Net         middleware.Network
	Storage     middleware.Storage
	Registry    *agentmgr.Registry
	RegistryURL string
	CatalogURL  string
}

func NewProvider(source Source, opts Options) (Provider, error) {
	switch source {
	case SourceLocal:
		if opts.Registry == nil || opts.Storage == nil {
			return nil, fmt.Errorf("local discovery requires registry and storage")
		}
		return &localProvider{registry: opts.Registry, storage: opts.Storage}, nil
	case SourceACPRegistry:
		if opts.Net == nil {
			return nil, fmt.Errorf("ACP registry discovery requires network")
		}
		return &acpRegistryProvider{client: agentmgr.NewCachingRegistryClient(opts.Net, opts.RegistryURL, opts.Storage)}, nil
	case SourceA2ACard:
		if opts.Net == nil {
			return nil, fmt.Errorf("A2A card discovery requires network")
		}
		return &a2aCardProvider{net: opts.Net}, nil
	case SourceA2ACatalog:
		if opts.Net == nil {
			return nil, fmt.Errorf("A2A catalog discovery requires network")
		}
		if strings.TrimSpace(opts.CatalogURL) == "" {
			return nil, fmt.Errorf("A2A catalog discovery requires catalog URL")
		}
		return &a2aCatalogProvider{net: opts.Net, catalogURL: opts.CatalogURL}, nil
	default:
		return nil, fmt.Errorf("unsupported discovery source %q", source)
	}
}

type localProvider struct {
	registry *agentmgr.Registry
	storage  middleware.Storage
}

func (p *localProvider) Search(_ context.Context, query string) ([]Record, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	records := make([]Record, 0, len(p.registry.IDs()))
	for _, id := range p.registry.IDs() {
		record, err := p.Get(context.Background(), id)
		if err != nil {
			return nil, err
		}
		if query != "" && !matches(*record, query) {
			continue
		}
		records = append(records, *record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records, nil
}

func (p *localProvider) Get(_ context.Context, ref string) (*Record, error) {
	cfg, err := p.registry.Get(ref)
	if err != nil {
		return nil, err
	}
	meta, _ := agentcfg.LoadMeta(p.storage, ref)
	endpoint := agentcfg.NormalizeEndpoint(agentcfg.Config{
		Command:         cfg.Command,
		Args:            cfg.Args,
		Env:             cfg.Env,
		Kind:            cfg.Kind,
		Transport:       cfg.Transport,
		Address:         cfg.Address,
		CardURL:         cfg.CardURL,
		ProtocolVersion: cfg.ProtocolVersion,
		HealthcheckPath: cfg.HealthcheckPath,
		EnvIsolation:    cfg.EnvIsolation,
		Active:          cfg.Active,
	})
	address := endpoint.Address
	if endpoint.Kind == middleware.ProtocolKindACP && endpoint.Transport == "stdio" {
		address = endpoint.Command
	}
	return &Record{
		ID:              ref,
		Name:            meta.Name,
		Description:     meta.Description,
		Source:          SourceLocal,
		Kind:            endpoint.Kind,
		Transport:       endpoint.Transport,
		Address:         address,
		CardURL:         endpoint.CardURL,
		ProtocolVersion: endpoint.ProtocolVersion,
	}, nil
}

type acpRegistryProvider struct {
	client *agentmgr.RegistryClient
}

func (p *acpRegistryProvider) Search(ctx context.Context, query string) ([]Record, error) {
	index, err := p.client.FetchIndexCached(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	records := make([]Record, 0, len(index.Agents))
	for _, agent := range index.Agents {
		record := recordFromManifest(agent)
		if query != "" && !matches(record, query) {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (p *acpRegistryProvider) Get(ctx context.Context, ref string) (*Record, error) {
	manifest, err := p.client.FetchManifestCached(ctx, ref)
	if err != nil {
		return nil, err
	}
	record := recordFromManifest(*manifest)
	return &record, nil
}

type a2aCardProvider struct {
	net middleware.Network
}

func (p *a2aCardProvider) Search(context.Context, string) ([]Record, error) {
	return nil, ErrSearchUnsupported
}

func (p *a2aCardProvider) Get(ctx context.Context, ref string) (*Record, error) {
	cardURL, err := ResolveAgentCardURL(ref)
	if err != nil {
		return nil, err
	}
	var card a2asdk.AgentCard
	if err := p.net.FetchJSON(ctx, cardURL, &card); err != nil {
		return nil, err
	}
	record := recordFromAgentCard(cardURL, &card)
	return &record, nil
}

type a2aCatalogProvider struct {
	net        middleware.Network
	catalogURL string
}

type a2aCatalogDocument struct {
	Version string            `json:"version,omitempty"`
	Agents  []a2aCatalogEntry `json:"agents,omitempty"`
	Entries []a2aCatalogEntry `json:"entries,omitempty"`
}

type a2aCatalogEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Transport   string   `json:"transport,omitempty"`
	Address     string   `json:"address,omitempty"`
	CardURL     string   `json:"card_url,omitempty"`
	Website     string   `json:"website,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	License     string   `json:"license,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Protocol    string   `json:"protocol,omitempty"`
}

func (p *a2aCatalogProvider) Search(ctx context.Context, query string) ([]Record, error) {
	doc, err := p.load(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	records := make([]Record, 0, len(doc.entries()))
	for _, entry := range doc.entries() {
		record := recordFromCatalogEntry(entry)
		if query != "" && !matches(record, query) {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (p *a2aCatalogProvider) Get(ctx context.Context, ref string) (*Record, error) {
	doc, err := p.load(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range doc.entries() {
		if entry.ID == ref {
			record := recordFromCatalogEntry(entry)
			return &record, nil
		}
	}
	return nil, fmt.Errorf("agent %q not found in A2A catalog", ref)
}

func (p *a2aCatalogProvider) load(ctx context.Context) (*a2aCatalogDocument, error) {
	var doc a2aCatalogDocument
	if err := p.net.FetchJSON(ctx, p.catalogURL, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (d *a2aCatalogDocument) entries() []a2aCatalogEntry {
	if len(d.Agents) > 0 {
		return d.Agents
	}
	return d.Entries
}

func recordFromManifest(manifest agentmgr.AgentManifest) Record {
	return Record{
		ID:           manifest.ID,
		Name:         manifest.Name,
		Description:  manifest.Description,
		Version:      manifest.Version,
		Source:       SourceACPRegistry,
		Kind:         middleware.ProtocolKindACP,
		Distribution: manifest.DistTypes(),
		Website:      manifest.Website,
		Repository:   manifest.Repository,
		Authors:      append([]string{}, manifest.Authors...),
		License:      manifest.License,
	}
}

func recordFromCatalogEntry(entry a2aCatalogEntry) Record {
	kind := middleware.ProtocolKindA2A
	if strings.EqualFold(strings.TrimSpace(entry.Kind), string(middleware.ProtocolKindACP)) || strings.EqualFold(strings.TrimSpace(entry.Protocol), string(middleware.ProtocolKindACP)) {
		kind = middleware.ProtocolKindACP
	}
	transport := strings.TrimSpace(entry.Transport)
	if transport == "" && kind == middleware.ProtocolKindA2A {
		transport = "JSONRPC"
	}
	return Record{
		ID:              entry.ID,
		Name:            entry.Name,
		Description:     entry.Description,
		Version:         entry.Version,
		ProtocolVersion: entry.Protocol,
		Source:          SourceA2ACatalog,
		Kind:            kind,
		Transport:       transport,
		Address:         entry.Address,
		CardURL:         entry.CardURL,
		Website:         entry.Website,
		Repository:      entry.Repository,
		Authors:         append([]string{}, entry.Authors...),
		License:         entry.License,
		Tags:            append([]string{}, entry.Tags...),
	}
}

func recordFromAgentCard(cardURL string, card *a2asdk.AgentCard) Record {
	iface := preferredInterface(card)
	tags := make([]string, 0, len(card.Skills))
	for _, skill := range card.Skills {
		tags = append(tags, skill.Tags...)
	}
	website := card.DocumentationURL
	if card.Provider != nil && website == "" {
		website = card.Provider.URL
	}
	return Record{
		ID:              slugify(card.Name),
		Name:            card.Name,
		Description:     card.Description,
		Version:         card.Version,
		ProtocolVersion: iface.ProtocolVersion,
		Source:          SourceA2ACard,
		Kind:            middleware.ProtocolKindA2A,
		Transport:       iface.ProtocolBindingString(),
		Address:         iface.URL,
		CardURL:         cardURL,
		Website:         website,
		Tags:            dedupe(tags),
	}
}

func preferredInterface(card *a2asdk.AgentCard) *agentInterfaceView {
	view := &agentInterfaceView{}
	if card == nil || len(card.SupportedInterfaces) == 0 {
		return view
	}
	for _, iface := range card.SupportedInterfaces {
		if iface == nil {
			continue
		}
		candidate := &agentInterfaceView{
			URL:             iface.URL,
			ProtocolBinding: string(iface.ProtocolBinding),
			ProtocolVersion: string(iface.ProtocolVersion),
		}
		if iface.ProtocolBinding == a2asdk.TransportProtocolJSONRPC {
			return candidate
		}
		if view.URL == "" {
			view = candidate
		}
	}
	return view
}

type agentInterfaceView struct {
	URL             string
	ProtocolBinding string
	ProtocolVersion string
}

func (v *agentInterfaceView) ProtocolBindingString() string {
	return v.ProtocolBinding
}

func ResolveAgentCardURL(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty A2A card reference")
	}
	parsed, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("invalid A2A card reference: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("A2A card reference must be an absolute URL")
	}
	if strings.HasSuffix(parsed.Path, ".json") {
		return parsed.String(), nil
	}
	parsed.Path = path.Clean("/.well-known/agent-card.json")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func matches(record Record, query string) bool {
	if strings.Contains(strings.ToLower(record.ID), query) {
		return true
	}
	if strings.Contains(strings.ToLower(record.Name), query) {
		return true
	}
	if strings.Contains(strings.ToLower(record.Description), query) {
		return true
	}
	for _, tag := range record.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func slugify(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "a2a-agent"
	}
	return name
}

func dedupe(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
