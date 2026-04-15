package runtimecheck

import (
	"context"
	"fmt"

	"github.com/jose/matrix-v2/internal/middleware"
)

// FetchRuntimeReport fetches a runtime report from a remote agent endpoint.
func FetchRuntimeReport(ctx context.Context, net middleware.Network, url string) (map[string]any, error) {
	var report map[string]any
	if err := net.FetchJSON(ctx, url, &report); err != nil {
		return nil, fmt.Errorf("failed to fetch runtime report: %w", err)
	}
	return report, nil
}
