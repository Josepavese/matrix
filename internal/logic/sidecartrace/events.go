package sidecartrace

import (
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/sidecar"
	"github.com/Josepavese/matrix/internal/middleware"
)

func Events(run runtrace.Run, capsules []middleware.SidecarCapsule) []runtrace.Event {
	normalized := sidecar.NormalizeCapsules(capsules)
	events := make([]runtrace.Event, 0, len(normalized))
	for _, capsule := range normalized {
		event := runtrace.Event{
			RunID:             run.ID,
			Kind:              "sidecar.capsule.delivered",
			Actor:             "matrix",
			Status:            runtrace.StatusCompleted,
			Protocol:          run.Protocol,
			ProtocolMethod:    "matrix.sidecar.project",
			SidecarProvider:   capsule.Provider,
			SidecarID:         capsule.ID,
			SidecarSchema:     capsule.Schema,
			SidecarVersion:    capsule.Version,
			SidecarCarrier:    Carrier(capsule),
			SidecarVisibility: capsule.Visibility,
			Summary:           fmt.Sprintf("Delivered %s sidecar capsule %s", capsule.Provider, capsule.ID),
			ContentRef:        "matrix://runs/" + run.ID + "/sidecars/" + capsule.ID,
			ContentDigest:     runtrace.DigestString(capsule.Content),
			Metadata:          metadata(capsule),
			ProtocolMeta:      protocolMeta(run.Protocol, capsule),
		}
		if run.TracePolicy.ContentMode == runtrace.ContentModeInline {
			event.Message = capsule.Content
		}
		events = append(events, event)
	}
	return events
}

func Carrier(capsule middleware.SidecarCapsule) string {
	if capsule.Visibility != middleware.SidecarVisibilityLLMVisible {
		return "trace"
	}
	if capsule.Format != "" {
		return capsule.Format
	}
	return "text"
}

func metadata(capsule middleware.SidecarCapsule) map[string]interface{} {
	return map[string]interface{}{
		"frontend_visible": false,
		"audit_visible":    true,
		"trace_visible":    true,
		"provider":         capsule.Provider,
		"capsule_id":       capsule.ID,
		"schema":           capsule.Schema,
		"version":          capsule.Version,
		"format":           capsule.Format,
		"visibility":       capsule.Visibility,
	}
}

func protocolMeta(protocol string, capsule middleware.SidecarCapsule) map[string]interface{} {
	base := map[string]interface{}{
		"provider":   capsule.Provider,
		"capsule_id": capsule.ID,
		"schema":     capsule.Schema,
		"version":    capsule.Version,
		"format":     capsule.Format,
		"visibility": capsule.Visibility,
		"carrier":    Carrier(capsule),
		"metadata":   capsule.Metadata,
	}
	switch protocol {
	case string(middleware.ProtocolKindA2A):
		base["a2a"] = map[string]interface{}{
			"extension":    middleware.SidecarA2AExtensionURI,
			"media_type":   sidecar.MediaType(capsule),
			"metadata_key": "matrix.sidecar",
		}
	case string(middleware.ProtocolKindACP):
		base["acp"] = map[string]interface{}{
			"_meta_key":       capsule.Provider + ".dev/sidecar",
			"primary_carrier": Carrier(capsule),
		}
	}
	return base
}
