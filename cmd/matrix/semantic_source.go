package main

import (
	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/Josepavese/matrix/internal/logic/semanticfs"
	"github.com/Josepavese/matrix/internal/providers/oscapacity"
)

func semanticAgentViews(registry *agentmgr.Registry) func() ([]semanticfs.AgentView, error) {
	return func() ([]semanticfs.AgentView, error) {
		ids := registry.IDs()
		views := make([]semanticfs.AgentView, 0, len(ids))
		for _, id := range ids {
			cfg, err := registry.Get(id)
			if err != nil {
				return nil, err
			}
			endpoint := agentcfg.NormalizeEndpoint(cfg)
			active := cfg.IsActive()
			views = append(views, semanticfs.AgentView{ID: id, Active: &active, Kind: string(endpoint.Kind), Transport: endpoint.Transport, Source: "effective_registry_configuration"})
		}
		return views, nil
	}
}

func openSemanticFS(includeSummaries bool) (*semanticfs.FS, func(), error) {
	ctx, closeFn, err := NewReadOnlyAppContext(DefaultVaultPath)
	if err != nil {
		return nil, nil, err
	}
	registry, err := agentmgr.NewRegistry(ctx.ConfigRdr, ctx.Store)
	if err != nil {
		closeFn()
		return nil, nil, err
	}
	view := semanticfs.New(semanticfs.Source{Storage: ctx.Store, Agents: semanticAgentViews(registry), Capacity: oscapacity.New(), IncludeSummaries: includeSummaries})
	return view, closeFn, nil
}
