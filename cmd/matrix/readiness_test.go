package main

import "testing"

func TestReadinessExitCode(t *testing.T) {
	cases := []struct {
		name   string
		status any
		strict bool
		want   int
	}{
		{name: "not ready", status: "not_ready", strict: false, want: 0},
		{name: "not ready strict", status: "not_ready", strict: true, want: 2},
		{name: "ready with warnings", status: "ready_with_warnings", strict: false, want: 0},
		{name: "ready with warnings strict", status: "ready_with_warnings", strict: true, want: 2},
		{name: "ready strict", status: "ready", strict: true, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readinessExitCode(tc.status, tc.strict); got != tc.want {
				t.Fatalf("readinessExitCode(%v, %v) = %d, want %d", tc.status, tc.strict, got, tc.want)
			}
		})
	}
}
