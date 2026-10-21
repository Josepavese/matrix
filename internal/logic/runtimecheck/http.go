package runtimecheck

import (
	"context"
	"fmt"

	"github.com/Josepavese/matrix/internal/middleware"
)

// FetchRuntimeReport uses the configured Matrix credential when present. A
// backend without governed headers must fail rather than discard the key.
func FetchRuntimeReport(ctx context.Context, net middleware.Network, url, apiKey string) (map[string]any, error) {
	var report map[string]any
	var err error
	if apiKey == "" {
		err = net.FetchJSON(ctx, url, &report)
	} else {
		fetcher, ok := net.(middleware.JSONHeaderFetcher)
		if !ok {
			return nil, fmt.Errorf("runtime network backend does not support authenticated requests")
		}
		err = fetcher.FetchJSONWithHeaders(ctx, url, map[string]string{middleware.MatrixAPIKeyHeader: apiKey}, &report)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch runtime report: %w", err)
	}
	return report, nil
}
