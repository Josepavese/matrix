package orchestration

import (
	"encoding/json"
	"testing"
)

// TestProfileV1IsUniqueAndSelfDescribing treats the profile as what it is: a
// machine-readable contract. Duplicate identifiers or an empty description would
// make it unusable by the supervisory agents that consume it.
func TestProfileV1IsUniqueAndSelfDescribing(t *testing.T) {
	profile := ProfileV1()
	if profile.Name == "" || profile.Category == "" || profile.Role == "" {
		t.Fatalf("the profile must identify itself: %+v", profile)
	}
	if len(profile.Capabilities) == 0 || len(profile.Surfaces) == 0 {
		t.Fatal("an empty profile would advertise nothing")
	}

	capabilities := map[string]bool{}
	for _, capability := range profile.Capabilities {
		if capability.ID == "" {
			t.Fatal("a capability without an identifier cannot be referenced")
		}
		if capabilities[capability.ID] {
			t.Fatalf("capability %q is declared twice", capability.ID)
		}
		capabilities[capability.ID] = true
		if capability.Category == "" {
			t.Fatalf("capability %q has no category", capability.ID)
		}
		if capability.Description == "" {
			t.Fatalf("capability %q has no description", capability.ID)
		}
		if len(capability.Surfaces) == 0 {
			t.Fatalf("capability %q names no surface, so it is not reachable", capability.ID)
		}
	}

	surfaces := map[string]bool{}
	for _, surface := range profile.Surfaces {
		if surface.ID == "" {
			t.Fatal("a surface without an identifier cannot be addressed")
		}
		if surfaces[surface.ID] {
			t.Fatalf("surface %q is declared twice", surface.ID)
		}
		surfaces[surface.ID] = true
		if surface.Description == "" {
			t.Fatalf("surface %q has no description", surface.ID)
		}
		if len(surface.Actions) == 0 {
			t.Fatalf("surface %q lists no action", surface.ID)
		}
	}
}

// TestProfileV1IsStableJSON guards the wire shape: consumers decode these exact
// keys, so the profile must stay serialisable and keep its documented fields.
func TestProfileV1IsStableJSON(t *testing.T) {
	encoded, err := json.Marshal(ProfileV1())
	if err != nil {
		t.Fatalf("the profile must be serialisable: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("the profile must round trip: %v", err)
	}
	for _, key := range []string{"name", "category", "role", "capabilities", "surfaces"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("the profile lost the documented key %q: %s", key, encoded)
		}
	}
	// Determinism matters for diffing published profiles.
	again, err := json.Marshal(ProfileV1())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(again) != string(encoded) {
		t.Fatal("the profile must serialise identically on every call")
	}
}
