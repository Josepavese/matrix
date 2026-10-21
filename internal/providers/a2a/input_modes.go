package a2a

import (
	"context"
	"fmt"
	"mime"
	"strings"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// Matrix advertises MIME families (image/*, audio/*). The SDK 2.7 card
// enforcer matches only exact strings, so this adapter owns family matching
// against the same card instead of rejecting content the card explicitly allows.
type inputModeGuard struct {
	a2asrv.PassthroughCallInterceptor
	modes []string
}

func (g *inputModeGuard) Before(ctx context.Context, _ *a2asrv.CallContext, req *a2asrv.Request) (context.Context, any, error) {
	if req == nil {
		return ctx, nil, nil
	}
	message, ok := req.Payload.(*a2asdk.SendMessageRequest)
	if !ok || message == nil || message.Message == nil {
		return ctx, nil, nil
	}
	for _, part := range message.Message.Parts {
		mediaType, _, err := mime.ParseMediaType(partMediaType(part))
		if err != nil {
			return ctx, nil, fmt.Errorf("invalid input media type: %w", a2asdk.ErrInvalidParams)
		}
		if !matchesInputMode(g.modes, mediaType) {
			return ctx, nil, fmt.Errorf("unsupported input media type %q: %w", mediaType, a2asdk.ErrUnsupportedContentType)
		}
	}
	return ctx, nil, nil
}
func partMediaType(part *a2asdk.Part) string {
	if part == nil {
		return ""
	}
	if part.MediaType != "" {
		return part.MediaType
	}
	switch part.Content.(type) {
	case a2asdk.Text:
		return "text/plain"
	case a2asdk.Data:
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
func matchesInputMode(modes []string, value string) bool {
	for _, mode := range modes {
		if mode == value || mode == "*/*" {
			return true
		}
		if strings.HasSuffix(mode, "/*") && strings.HasPrefix(value, strings.TrimSuffix(mode, "*")) {
			return true
		}
	}
	return false
}
