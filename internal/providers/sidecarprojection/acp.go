package sidecarprojection

import (
	"github.com/jose/matrix-v2/internal/logic/sidecar"
	"github.com/jose/matrix-v2/internal/middleware"
)

func ACPMeta(capsules []middleware.SidecarCapsule) map[string]interface{} {
	normalized := sidecar.NormalizeCapsules(capsules)
	if len(normalized) == 0 {
		return nil
	}
	out := map[string]interface{}{
		"matrix.dev/sidecar": map[string]interface{}{
			"capsule_ids": sidecar.CapsuleIDs(normalized),
			"count":       len(normalized),
		},
	}
	byProvider := map[string][]map[string]interface{}{}
	for _, capsule := range normalized {
		record := map[string]interface{}{
			"id":         capsule.ID,
			"schema":     capsule.Schema,
			"version":    capsule.Version,
			"visibility": capsule.Visibility,
			"format":     capsule.Format,
			"carrier":    capsule.Format,
		}
		if capsule.Metadata != nil {
			record["metadata"] = capsule.Metadata
		}
		byProvider[capsule.Provider] = append(byProvider[capsule.Provider], record)
	}
	for provider, records := range byProvider {
		out[provider+".dev/sidecar"] = map[string]interface{}{
			"capsules":        records,
			"capsule_ids":     capsuleIDsFromRecords(records),
			"primary_carrier": "model_visible_text",
		}
	}
	return out
}

func capsuleIDsFromRecords(records []map[string]interface{}) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if id, ok := record["id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
