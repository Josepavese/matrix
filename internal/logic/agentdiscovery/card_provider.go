package agentdiscovery

import (
	"context"
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/middleware"
	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
)

type a2aCardProvider struct {
	net     middleware.Network
	headers map[string]string
}

type headerJSONFetcher interface {
	FetchJSONWithHeaders(ctx context.Context, url string, headers map[string]string, target interface{}) error
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
	if len(p.headers) > 0 {
		fetcher, ok := p.net.(headerJSONFetcher)
		if !ok {
			return nil, fmt.Errorf("A2A card discovery backend does not support governed request headers")
		}
		if err := fetcher.FetchJSONWithHeaders(ctx, cardURL, p.headers, &card); err != nil {
			return nil, err
		}
	} else if err := p.net.FetchJSON(ctx, cardURL, &card); err != nil {
		return nil, err
	}
	record := recordFromAgentCard(cardURL, &card)
	return &record, nil
}

func copyHeaders(headers map[string]string) map[string]string {
	return agentcfg.CloneHeaders(headers)
}
