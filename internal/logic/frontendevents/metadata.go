// Package frontendevents normalizes provider runtime signals into Matrix
// frontend/audit event fields without depending on one channel or protocol.
package frontendevents

import (
	"fmt"
	"strings"
)

func StringValue(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	switch value := values[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func SourceUpdateType(metadata map[string]interface{}, fallback string) string {
	return FirstNonEmpty(StringValue(metadata, "source_update_type"), fallback)
}

func ProtocolMeta(metadata map[string]interface{}) map[string]interface{} {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(metadata))
	for k, v := range metadata {
		out[k] = v
	}
	return out
}

func Merge(base, extra map[string]interface{}) map[string]interface{} {
	if base == nil {
		base = map[string]interface{}{}
	}
	for k, v := range extra {
		if v != "" && v != nil {
			base[k] = v
		}
	}
	return base
}

func truncateSummary(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 120 {
		return value
	}
	return value[:117] + "..."
}
