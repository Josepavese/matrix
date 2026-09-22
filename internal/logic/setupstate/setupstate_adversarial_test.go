package setupstate

import "testing"

// TestConfiguredRejectsAmbiguousSetup keeps a half-written or hostile value from
// counting as "configured": this flag decides whether onboarding is offered, so
// the failure direction must be "not configured".
func TestConfiguredRejectsAmbiguousSetup(t *testing.T) {
	// Note: a case-insensitive "true" with surrounding space is accepted on
	// purpose, so it is asserted in the accepted list below instead.
	for _, raw := range []string{"", "   ", "null", `"yes"`, `"1"`, "not json", "{", `{"configured":true}`} {
		if Configured([]byte(raw)) {
			t.Fatalf("value %q must not count as configured", raw)
		}
	}
	for _, raw := range []string{"true", `"true"`, `"True"`, `" true "`, `"TRUE "`} {
		if !Configured([]byte(raw)) {
			t.Fatalf("value %q must count as configured", raw)
		}
	}
	if Configured([]byte("false")) {
		t.Fatal("an explicit false must stay false")
	}
}
