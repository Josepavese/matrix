package sidecarprojection

import (
	"testing"

	"github.com/jose/matrix-v2/internal/middleware"
)

func TestACPMetaProjectsProviderCorrelation(t *testing.T) {
	meta := ACPMeta([]middleware.SidecarCapsule{{
		Provider:   "noema",
		ID:         "caps-acp",
		Schema:     "sidecar.intent.v0",
		Version:    "0.1",
		Visibility: middleware.SidecarVisibilityLLMVisible,
		Format:     middleware.SidecarFormatNoemaXML,
		Content:    "<noema/>",
	}})

	matrix, ok := meta["matrix.dev/sidecar"].(map[string]interface{})
	if !ok || matrix["count"] != 1 {
		t.Fatalf("expected Matrix sidecar correlation meta, got %#v", meta)
	}
	noema, ok := meta["noema.dev/sidecar"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected provider sidecar meta, got %#v", meta)
	}
	if noema["primary_carrier"] != "model_visible_text" {
		t.Fatalf("expected model-visible primary carrier, got %#v", noema)
	}
}
