package agentcfg

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

// TestMetaRoundTripsArtifactVerification locks the persisted shape of the
// integrity evidence: it is the machine-readable answer consumers read from the
// vault, so its keys must not drift silently.
func TestMetaRoundTripsArtifactVerification(t *testing.T) {
	store := memstore.New()
	verifiedAt := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	want := Meta{
		ID: "opencode", Name: "OpenCode", Version: "1.18.32", DistTypes: []string{"binary"},
		ArtifactVerification: &ArtifactVerification{
			Verified:   true,
			Status:     ArtifactVerified,
			Platform:   "linux-x86_64",
			Artifact:   "https://example.invalid/opencode-linux-x64.tar.gz",
			Expected:   "3046e0404fdc60fb80307e7a47824ba07477364178a4d09baa8548496dd6d43b",
			Actual:     "3046e0404fdc60fb80307e7a47824ba07477364178a4d09baa8548496dd6d43b",
			VerifiedAt: verifiedAt,
		},
	}
	if err := SaveMeta(store, "opencode", want); err != nil {
		t.Fatalf("SaveMeta failed: %v", err)
	}

	got, err := LoadMeta(store, "opencode")
	if err != nil {
		t.Fatalf("LoadMeta failed: %v", err)
	}
	if got.ArtifactVerification == nil {
		t.Fatal("the verification evidence must survive the vault round trip")
	}
	if *got.ArtifactVerification != *want.ArtifactVerification {
		t.Fatalf("verification = %+v, want %+v", got.ArtifactVerification, want.ArtifactVerification)
	}

	raw, err := store.Get(MetaKey("opencode"))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("the stored meta must be JSON: %v", err)
	}
	evidence, ok := decoded["artifact_verification"].(map[string]any)
	if !ok {
		t.Fatalf("the stored meta must expose artifact_verification, got %v", decoded)
	}
	for _, key := range []string{"verified", "status", "platform", "artifact", "expected_sha256", "actual_sha256", "verified_at"} {
		if _, ok := evidence[key]; !ok {
			t.Fatalf("artifact_verification must expose %q, got %v", key, evidence)
		}
	}
	if evidence["status"] != string(ArtifactVerified) {
		t.Fatalf("status = %v, want %q", evidence["status"], ArtifactVerified)
	}
}

// TestMetaOmitsArtifactVerificationWhenAbsent keeps the field out of the vault
// for agents that no installer touched: an absent record is not the same claim
// as a recorded "not verified".
func TestMetaOmitsArtifactVerificationWhenAbsent(t *testing.T) {
	store := memstore.New()
	if err := SaveMeta(store, "seeded", Meta{ID: "seeded", Name: "Seeded"}); err != nil {
		t.Fatalf("SaveMeta failed: %v", err)
	}

	raw, err := store.Get(MetaKey("seeded"))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("the stored meta must be JSON: %v", err)
	}
	if _, ok := decoded["artifact_verification"]; ok {
		t.Fatalf("meta without evidence must not carry the field, got %v", decoded)
	}
}
