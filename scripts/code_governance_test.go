package main

import "testing"

func TestEvaluateWarningBudgetAllowsBaseline(t *testing.T) {
	cfg := config{}
	cfg.WarningBudget.MaxTotalWarnings = 2
	cfg.WarningBudget.MaxBranchPoints = 12

	failures := evaluateWarningBudget(cfg, []string{"one", "two"}, []funcReport{
		{Qualified: "ok", Path: "ok.go", BranchPoints: 12},
	})

	if len(failures) != 0 {
		t.Fatalf("unexpected failures: %#v", failures)
	}
}

func TestEvaluateWarningBudgetFailsWhenDebtGrows(t *testing.T) {
	cfg := config{}
	cfg.WarningBudget.MaxTotalWarnings = 1
	cfg.WarningBudget.MaxBranchPoints = 10

	failures := evaluateWarningBudget(cfg, []string{"one", "two"}, []funcReport{
		{Qualified: "tooComplex", Path: "bad.go", BranchPoints: 11},
	})

	if len(failures) != 2 {
		t.Fatalf("expected two failures, got %#v", failures)
	}
}
