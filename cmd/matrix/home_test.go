package main

import (
	"path/filepath"
	"testing"
)

func TestResolveInvocationPathUsesCapturedCWD(t *testing.T) {
	old := invocationCWD
	t.Cleanup(func() { invocationCWD = old })

	base := t.TempDir()
	invocationCWD = base

	got, err := resolveInvocationPath(filepath.Join("bin", "agent"))
	if err != nil {
		t.Fatalf("resolveInvocationPath: %v", err)
	}
	want := filepath.Join(base, "bin", "agent")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveOptionalInvocationPathPreservesEmpty(t *testing.T) {
	got, err := resolveOptionalInvocationPath("")
	if err != nil {
		t.Fatalf("resolveOptionalInvocationPath: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty path, got %q", got)
	}
}
