package system_tools

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/matrixhome"
	"github.com/Josepavese/matrix/internal/middleware"
)

// systemToolHome prepares a temporary Matrix home with the config directory the
// only writable tool targets, so a test can assert on the file itself.
func systemToolHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temporary home: %v", err)
	}
	t.Setenv(matrixhome.EnvName, home)
	if err := os.MkdirAll(matrixhome.ConfigsDir(home), 0o700); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	return home
}

func rawToolCall(name, arguments string) middleware.ToolCall {
	return middleware.ToolCall{Function: middleware.ToolCallFunction{Name: name, Arguments: arguments}}
}

// TestExecuteToolRefusesAToolCallTheTurnDidNotAdvertise pins the turn scoping:
// Config_Set belongs to the published contract, but a turn that advertised only
// APM_Install never offered it, so it must not run with that turn's authority.
// The assertion is the file, because the check is worth nothing if the write
// still lands.
func TestExecuteToolRefusesAToolCallTheTurnDidNotAdvertise(t *testing.T) {
	home := systemToolHome(t)
	handler := configSetHandler(t, home)

	result := handler.ExecuteTool(
		[]middleware.Tool{{Name: "APM_Install"}},
		configSetCall(t, "configs/agents.json", `{"agents":["claude"]}`),
	)
	written := filepath.Join(home, "configs", "agents.json")
	if _, err := os.Stat(written); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a call the turn did not advertise must not write %s (stat error: %v)", written, err)
	}
	if !strings.Contains(result, "Refused") || !strings.Contains(result, "did not advertise") {
		t.Fatalf("a call the turn did not advertise must be refused as such, got %q", result)
	}
}

// TestExecuteToolExecutesACallTheTurnAdvertised is the other direction of the
// same check: the call the previous test refused still runs when the turn
// advertised it, so the check authorizes a call rather than blocking the tool.
func TestExecuteToolExecutesACallTheTurnAdvertised(t *testing.T) {
	home := systemToolHome(t)
	handler := configSetHandler(t, home)

	result := handler.ExecuteTool(GetSystemTools(), configSetCall(t, "configs/agents.json", `{"agents":["claude"]}`))
	if !strings.HasPrefix(result, "Success") {
		t.Fatalf("an advertised, well-formed call must execute, got %q", result)
	}
	written := filepath.Join(home, "configs", "agents.json")
	if got, err := os.ReadFile(written); err != nil || string(got) != `{"agents":["claude"]}` {
		t.Fatalf("the advertised call must have written %s (content %q, error %v)", written, got, err)
	}
}

// TestExecuteToolRefusesACallWithMalformedArguments keeps an unparseable payload
// from reaching a handler that would decide what the empty values mean.
func TestExecuteToolRefusesACallWithMalformedArguments(t *testing.T) {
	home := systemToolHome(t)
	handler := configSetHandler(t, home)

	result := handler.ExecuteTool(GetSystemTools(), rawToolCall("Config_Set", `{"key":`))
	written := filepath.Join(home, "configs", "agents.json")
	if _, err := os.Stat(written); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("an unparseable call must not write %s (stat error: %v)", written, err)
	}
	if !strings.Contains(result, "Refused") {
		t.Fatalf("an unparseable call must be refused, got %q", result)
	}
}

// TestExecuteToolRefusesACallMissingARequiredArgument drives the shape the
// advertised schema forbids: the key without the value. Dispatching it reaches
// the writer with an empty value and creates the configuration file anyway, so
// the assertion is that no file appears.
func TestExecuteToolRefusesACallMissingARequiredArgument(t *testing.T) {
	home := systemToolHome(t)
	handler := configSetHandler(t, home)

	result := handler.ExecuteTool(GetSystemTools(), rawToolCall("Config_Set", `{"key":"configs/agents.json"}`))
	written := filepath.Join(home, "configs", "agents.json")
	if _, err := os.Stat(written); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a call missing a required argument must not write %s (stat error: %v)", written, err)
	}
	if !strings.Contains(result, "Refused") || !strings.Contains(result, "value") {
		t.Fatalf("a call missing a required argument must name it in the refusal, got %q", result)
	}
}

// TestExecuteToolRefusesANameOutsideThePublishedContract pins the gate the switch
// alone cannot provide: the dispatch is decided by the advertised contract, so a
// name added to the switch later cannot become an entry point on its own. Every
// advertised name must still pass that gate, which is the other half of the same
// contract.
func TestExecuteToolRefusesANameOutsideThePublishedContract(t *testing.T) {
	handler := NewHandler(nil, nil, nil)

	result := handler.ExecuteTool(GetSystemTools(), rawToolCall("APM_Purge", "{}"))
	if !strings.Contains(result, "Refused") || !strings.Contains(result, "does not advertise") {
		t.Fatalf("a name outside the published contract must be refused by the contract, got %q", result)
	}
	for _, tool := range GetSystemTools() {
		got := handler.ExecuteTool(GetSystemTools(), rawToolCall(tool.Name, "{}"))
		if strings.Contains(got, "does not advertise") {
			t.Fatalf("advertised tool %q must pass the contract gate, got %q", tool.Name, got)
		}
	}
}
