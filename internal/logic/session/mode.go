package session

import (
	"strings"

	"github.com/Josepavese/matrix/internal/logic/workspace"
)

const (
	modeImplementation = "implementation"
	modeReview         = "review"
	modeExplain        = "explain"
	modeTriage         = "triage"
)

func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case modeReview:
		return modeReview
	case modeExplain:
		return modeExplain
	case modeTriage:
		return modeTriage
	case "", modeImplementation:
		return modeImplementation
	default:
		return modeImplementation
	}
}

func defaultModeForWorkspace(meta workspace.Meta) string {
	return normalizeMode(meta.DefaultMode)
}
