package deliverycontract

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestTheContractRefusesMoreArtifactsThanItAccepts pins the ceiling on the number
// of declared requirements. The limit on the request body bounds the bytes of a
// declaration, not how many stats and hashes the terminal is asked to do: a small
// body holds tens of thousands of minimal entries, so this is the one that bounds
// the work.
func TestTheContractRefusesMoreArtifactsThanItAccepts(t *testing.T) {
	// The number is a policy, so it is pinned as data: a limit that moves silently
	// is a limit nobody agreed to.
	if maxContractArtifacts != 32 {
		t.Fatalf("the artifact limit is %d, want %d", maxContractArtifacts, 32)
	}

	over := declaredArtifacts(maxContractArtifacts + 1)
	err := Contract{Artifacts: over}.Validate()
	if err == nil {
		t.Fatalf("%d declared artifacts were accepted, and the limit is %d", len(over), maxContractArtifacts)
	}
	for _, want := range []string{strconv.Itoa(len(over)), strconv.Itoa(maxContractArtifacts)} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal must name %s, got %q", want, err.Error())
		}
	}

	at := declaredArtifacts(maxContractArtifacts)
	if err := (Contract{Artifacts: at}).Validate(); err != nil {
		t.Fatalf("a contract exactly at the limit was refused: %v", err)
	}
}

// TestTheLimitBoundsTheWorkTheTerminalDoes is the consequence, and the tooth:
// with the rule removed the terminal walks every declared artifact instead of
// refusing the declaration, so the test reports what it did rather than that
// something is wrong.
func TestTheLimitBoundsTheWorkTheTerminalDoes(t *testing.T) {
	workspace := t.TempDir()
	over := declaredArtifacts(maxContractArtifacts + 1)
	for i, artifact := range over {
		path := filepath.Join(workspace, artifact.Path)
		if err := os.WriteFile(path, []byte(fmt.Sprintf("delivered %d", i)), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	verdict := Evaluate(context.Background(), workspace, Contract{Artifacts: over})
	if verdict.Status != StatusUnverifiable {
		t.Fatalf("the terminal evaluated %d declared artifacts (status %s) instead of refusing the declaration: the limit is %d",
			len(verdict.Checks), verdict.Status, maxContractArtifacts)
	}
	if len(verdict.Checks) != 0 {
		t.Fatalf("the terminal did %d checks for a declaration it should have refused", len(verdict.Checks))
	}
}

// declaredArtifacts builds a declaration of exactly count artifacts, each inside
// the workspace it will be evaluated against.
func declaredArtifacts(count int) []Artifact {
	artifacts := make([]Artifact, 0, count)
	for i := 0; i < count; i++ {
		artifacts = append(artifacts, Artifact{Path: fmt.Sprintf("artifact-%02d.txt", i)})
	}
	return artifacts
}
