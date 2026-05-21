package agents

import (
	"errors"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
)

func TestClassifyProviderFailureDetectsModelUnavailable(t *testing.T) {
	err := classifyProviderFailure("codex", middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   "/usr/local/bin/codex-acp",
	}, "session/prompt", errors.New("stream disconnected before completion: The model `gpt-5.5` does not exist or you do not have access to it"))

	var failure *providerfailure.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("expected ProviderFailure, got %T %[1]v", err)
	}
	if failure.Code != providerfailure.ModelUnavailable {
		t.Fatalf("expected model unavailable code, got %+v", failure)
	}
	if failure.RequestedModel != "gpt-5.5" {
		t.Fatalf("expected requested model extraction, got %+v", failure)
	}
	if failure.Diagnostics["adapter"] != "codex-acp" {
		t.Fatalf("expected adapter diagnostic, got %+v", failure.Diagnostics)
	}
	if failure.Diagnostics["provider_error"] == "" || failure.Diagnostics["failure_reason"] == "" {
		t.Fatalf("expected provider error diagnostics, got %+v", failure.Diagnostics)
	}
}

func TestClassifyProviderFailureClassifiesClientContextCancellation(t *testing.T) {
	err := classifyProviderFailure("opencode", middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   "/home/jose/.local/bin/opencode",
	}, "session/new", errors.New("ACP new session failed: client context cancelled"))

	var failure *providerfailure.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("expected ProviderFailure, got %T %[1]v", err)
	}
	if failure.Diagnostics["failure_reason"] != "provider_client_context_cancelled" {
		t.Fatalf("expected precise failure reason, got %+v", failure.Diagnostics)
	}
	if failure.Diagnostics["provider_error"] == "" {
		t.Fatalf("expected underlying provider error, got %+v", failure.Diagnostics)
	}
}

func TestAnnotateProviderFailureAgentKeepsExistingAgent(t *testing.T) {
	err := &providerfailure.Failure{Code: providerfailure.PreflightFailed, AgentID: "codex"}

	got := annotateProviderFailureAgent(err, "opencode")

	var failure *providerfailure.Failure
	if !errors.As(got, &failure) {
		t.Fatalf("expected ProviderFailure, got %T %[1]v", got)
	}
	if failure.AgentID != "codex" {
		t.Fatalf("expected existing agent to be retained, got %+v", failure)
	}
}
