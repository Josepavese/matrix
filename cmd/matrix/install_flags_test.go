package main

import (
	"strings"
	"testing"
)

// TestInstallAllowUnverifiedIsAnExplicitOffByDefaultOptIn pins the knob the
// fail-closed digest gate needs: the operator has to ask for an unverified
// install by name, the default stays refusal, and the flag says what it accepts.
func TestInstallAllowUnverifiedIsAnExplicitOffByDefaultOptIn(t *testing.T) {
	flag := installCmd.Flags().Lookup("allow-unverified")
	if flag == nil {
		t.Fatal("matrix install must expose --allow-unverified for a digest-less binary distribution")
	}
	if flag.DefValue != "false" {
		t.Fatalf("--allow-unverified must default to false, got %q", flag.DefValue)
	}
	for _, want := range []string{"sha256", "refused by default", "evidence"} {
		if !strings.Contains(flag.Usage, want) {
			t.Fatalf("the flag usage must mention %q, got %q", want, flag.Usage)
		}
	}
}

// TestInstallAllowUnverifiedParsesIntoTheCommandVariable keeps the flag wired to
// the value the command hands the installer: parsing the flag is what the Run
// path does, so a renamed or unregistered variable shows up here.
func TestInstallAllowUnverifiedParsesIntoTheCommandVariable(t *testing.T) {
	saved := installAllowUnverified
	t.Cleanup(func() {
		installAllowUnverified = saved
		if err := installCmd.Flags().Set("allow-unverified", "false"); err != nil {
			t.Fatalf("resetting the flag failed: %v", err)
		}
	})

	if installAllowUnverified {
		t.Fatal("the opt-in must start off")
	}
	if err := installCmd.Flags().Parse([]string{"--allow-unverified"}); err != nil {
		t.Fatalf("parsing --allow-unverified failed: %v", err)
	}
	if !installAllowUnverified {
		t.Fatal("--allow-unverified must set the opt-in the command passes to the installer")
	}
}

func TestConfiguredAgentRegistryURL(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		accepted bool
	}{
		{"", true},
		{"https://mirror.example.test/acp/registry.json", true},
		{"http://mirror.example.test/acp/registry.json", false},
		{"https://user:pass@mirror.example.test/index", false},
		{"/local/index.json", false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			t.Setenv("MATRIX_AGENT_REGISTRY_URL", tc.raw)
			_, err := configuredAgentRegistryURL()
			if (err == nil) != tc.accepted {
				t.Fatalf("URL %q accepted=%v, error=%v", tc.raw, tc.accepted, err)
			}
		})
	}
}
