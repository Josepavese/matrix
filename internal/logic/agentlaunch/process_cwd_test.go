package agentlaunch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestResolveProcessCwdRefusesRatherThanSubstituting pins the guardrail that
// makes a declared process cwd safe: a value Matrix cannot use stops the launch
// and says what to fix. It is never repaired, never resolved against another
// directory, and never replaced by a default, because starting the child
// somewhere else is the failure the declaration exists to prevent.
func TestResolveProcessCwdRefusesRatherThanSubstituting(t *testing.T) {
	dir := t.TempDir()
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "absent")

	for _, test := range []struct {
		why      string
		declared string
		want     string
		wantErr  string
	}{
		{why: "no declaration chooses nothing", declared: "", want: ""},
		{why: "an absolute existing directory is used", declared: dir, want: resolvedDir},
		{why: "a relative path is refused", declared: "relative/checkout", wantErr: "must be an absolute path"},
		{why: "an absent directory is refused", declared: missing, wantErr: "is not usable"},
		{why: "a file is refused", declared: file, wantErr: "is not a directory"},
	} {
		t.Run(test.why, func(t *testing.T) {
			endpoint := middleware.ProtocolEndpoint{Env: []string{ProcessCwdEnv + "=" + test.declared}}
			got, err := ResolveProcessCwd(endpoint)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("expected refusal containing %q, got %q (err %v)", test.wantErr, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected refusal: %v", err)
			}
			if got != test.want {
				t.Fatalf("process cwd = %q, want %q", got, test.want)
			}
		})
	}
}

// TestDeclaredProcessCwdReadsOnlyTheExactDeclaration keeps the declaration from
// being read out of a variable that merely ends in the same words: a provider is
// not speaking because a name resembles its key.
func TestDeclaredProcessCwdReadsOnlyTheExactDeclaration(t *testing.T) {
	for _, test := range []struct {
		why  string
		env  []string
		want string
	}{
		{why: "exact key", env: []string{ProcessCwdEnv + "=/srv/checkout"}, want: "/srv/checkout"},
		{why: "padded value", env: []string{ProcessCwdEnv + "=  /srv/checkout  "}, want: "/srv/checkout"},
		{why: "lookalike suffix", env: []string{"NOT_" + ProcessCwdEnv + "=/srv/other"}, want: ""},
		{why: "lookalike prefix", env: []string{ProcessCwdEnv + "_OLD=/srv/other"}, want: ""},
		{why: "undeclared", env: []string{"PATH=/usr/bin"}, want: ""},
	} {
		t.Run(test.why, func(t *testing.T) {
			if got := DeclaredProcessCwd(middleware.ProtocolEndpoint{Env: test.env}); got != test.want {
				t.Fatalf("declared process cwd = %q, want %q", got, test.want)
			}
		})
	}
}
