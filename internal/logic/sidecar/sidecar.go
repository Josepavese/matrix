package sidecar

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

var mediaTokenRE = regexp.MustCompile(`[^a-z0-9.+-]+`)

func NormalizeCapsules(capsules []middleware.SidecarCapsule) []middleware.SidecarCapsule {
	if len(capsules) == 0 {
		return nil
	}
	out := make([]middleware.SidecarCapsule, 0, len(capsules))
	for _, capsule := range capsules {
		capsule.Provider = strings.ToLower(strings.TrimSpace(capsule.Provider))
		capsule.ID = strings.TrimSpace(capsule.ID)
		capsule.Schema = strings.TrimSpace(capsule.Schema)
		capsule.Version = strings.TrimSpace(capsule.Version)
		capsule.Visibility = normalizeVisibility(capsule.Visibility)
		capsule.Format = normalizeFormat(capsule.Format, capsule.Content)
		capsule.Content = strings.TrimSpace(capsule.Content)
		out = append(out, capsule)
	}
	return out
}

func ValidateCapsules(capsules []middleware.SidecarCapsule) error {
	for idx, capsule := range NormalizeCapsules(capsules) {
		if capsule.Provider == "" {
			return fmt.Errorf("sidecar_capsules[%d].provider is required", idx)
		}
		if capsule.ID == "" {
			return fmt.Errorf("sidecar_capsules[%d].id is required", idx)
		}
		if capsule.Visibility == middleware.SidecarVisibilityLLMVisible && capsule.Content == "" {
			return fmt.Errorf("sidecar_capsules[%d].content is required for llm_visible capsules", idx)
		}
	}
	return nil
}

func ProjectPrompt(message string, capsules []middleware.SidecarCapsule) string {
	blocks := ModelVisibleBlocks(capsules)
	if len(blocks) == 0 {
		return message
	}
	return strings.TrimRight(message, "\n") + "\n\n" + strings.Join(blocks, "\n\n")
}

func ModelVisibleBlocks(capsules []middleware.SidecarCapsule) []string {
	normalized := NormalizeCapsules(capsules)
	blocks := make([]string, 0, len(normalized))
	for _, capsule := range normalized {
		if capsule.Visibility == middleware.SidecarVisibilityLLMVisible && capsule.Content != "" {
			blocks = append(blocks, capsule.Content)
		}
	}
	return blocks
}

func CapsuleIDs(capsules []middleware.SidecarCapsule) []string {
	normalized := NormalizeCapsules(capsules)
	ids := make([]string, 0, len(normalized))
	for _, capsule := range normalized {
		if capsule.ID != "" {
			ids = append(ids, capsule.ID)
		}
	}
	return ids
}

func MediaType(capsule middleware.SidecarCapsule) string {
	provider := strings.ToLower(strings.TrimSpace(capsule.Provider))
	if provider == "" {
		provider = "matrix"
	}
	provider = strings.Trim(mediaTokenRE.ReplaceAllString(provider, "-"), "-")
	if provider == "" {
		provider = "matrix"
	}
	return "application/vnd." + provider + ".sidecar+json"
}

func normalizeVisibility(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return middleware.SidecarVisibilityLLMVisible
	}
	return value
}

func normalizeFormat(raw string, content string) string {
	format := strings.ToLower(strings.TrimSpace(raw))
	if format != "" {
		return format
	}
	if strings.Contains(strings.ToLower(content), "<noema") {
		return middleware.SidecarFormatNoemaXML
	}
	return middleware.SidecarFormatText
}
