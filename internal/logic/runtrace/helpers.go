package runtrace

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func normalizeExecutionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ExecutionModeAsync:
		return ExecutionModeAsync
	case ExecutionModeStream:
		return ExecutionModeStream
	default:
		return ExecutionModeSync
	}
}

func DigestString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func redactionFor(contentMode string) string {
	switch contentMode {
	case ContentModeInline:
		return "inline_allowed"
	case ContentModeRedacted:
		return "redacted"
	default:
		return "content_ref_only"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
