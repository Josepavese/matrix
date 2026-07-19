package agentcatalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentdiscovery"
	"github.com/Josepavese/matrix/internal/middleware"
)

// A2ARegistration describes a direct or agent-card-discovered endpoint.
type A2ARegistration struct {
	ID              string
	Address         string
	Transport       string
	ProtocolVersion string
	CardURL         string
	Tenant          string
	Headers         map[string]string
}

// ResolveAndRegisterA2A resolves an optional card and persists the current endpoint contract.
func ResolveAndRegisterA2A(ctx context.Context, storage middleware.Storage, net middleware.Network, request A2ARegistration) (Entry, error) {
	entry := Entry{
		ID: request.ID, Name: request.ID, Source: agentdiscovery.SourceA2ACard,
		Kind: middleware.ProtocolKindA2A, Transport: request.Transport,
		Address: request.Address, CardURL: request.CardURL, Tenant: request.Tenant,
		Headers: agentcfg.CloneHeaders(request.Headers), ProtocolVersion: request.ProtocolVersion,
	}
	if strings.TrimSpace(entry.Address) == "" {
		provider, err := agentdiscovery.NewProvider(agentdiscovery.SourceA2ACard, agentdiscovery.Options{Net: net, Headers: entry.Headers})
		if err != nil {
			return Entry{}, err
		}
		record, err := provider.Get(ctx, entry.CardURL)
		if err != nil {
			return Entry{}, err
		}
		applyDiscoveredA2A(&entry, *record)
	}
	if strings.TrimSpace(entry.Address) == "" {
		return Entry{}, fmt.Errorf("unable to resolve an A2A endpoint address")
	}
	if err := RegisterRemote(storage, entry); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func applyDiscoveredA2A(entry *Entry, record agentdiscovery.Record) {
	entry.Address = record.Address
	entry.CardURL = record.CardURL
	if entry.Transport == "" {
		entry.Transport = record.Transport
	}
	if entry.ProtocolVersion == "" {
		entry.ProtocolVersion = record.ProtocolVersion
	}
	if entry.Tenant == "" {
		entry.Tenant = record.Tenant
	}
}
