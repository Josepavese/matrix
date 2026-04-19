package sidecarprojection

import (
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/jose/matrix-v2/internal/logic/sidecar"
	"github.com/jose/matrix-v2/internal/middleware"
)

func A2AMessageParts(turn middleware.ConversationTurn) []*a2a.Part {
	capsules := sidecar.NormalizeCapsules(turn.SidecarCapsules)
	parts := []*a2a.Part{a2a.NewTextPart(turn.Message)}
	for _, capsule := range capsules {
		dataPart := a2a.NewDataPart(map[string]any{"sidecar": map[string]any{
			"provider":   capsule.Provider,
			"id":         capsule.ID,
			"schema":     capsule.Schema,
			"version":    capsule.Version,
			"visibility": capsule.Visibility,
			"format":     capsule.Format,
			"metadata":   capsule.Metadata,
		}})
		dataPart.MediaType = sidecar.MediaType(capsule)
		dataPart.Metadata = map[string]any{
			"matrix.sidecar": true,
			"provider":       capsule.Provider,
			"capsule_id":     capsule.ID,
			"schema":         capsule.Schema,
			"visibility":     capsule.Visibility,
		}
		parts = append(parts, dataPart)
		if capsule.Visibility == middleware.SidecarVisibilityLLMVisible && capsule.Content != "" {
			parts = append(parts, a2a.NewTextPart(capsule.Content))
		}
	}
	return parts
}

func ApplyA2AMetadata(msg *a2a.Message, capsules []middleware.SidecarCapsule) {
	normalized := sidecar.NormalizeCapsules(capsules)
	if len(normalized) == 0 {
		return
	}
	msg.Extensions = append(msg.Extensions, middleware.SidecarA2AExtensionURI)
	msg.Metadata = map[string]any{"matrix.sidecar": map[string]any{
		"capsule_ids": sidecar.CapsuleIDs(normalized),
		"count":       len(normalized),
	}}
}

func A2ARequestMetadata(capsules []middleware.SidecarCapsule) map[string]any {
	normalized := sidecar.NormalizeCapsules(capsules)
	if len(normalized) == 0 {
		return nil
	}
	return map[string]any{"matrix.sidecar": map[string]any{
		"capsule_ids": sidecar.CapsuleIDs(normalized),
		"extension":   middleware.SidecarA2AExtensionURI,
	}}
}
