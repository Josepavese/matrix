package session

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/matrixhome"
	"github.com/Josepavese/matrix/internal/logic/system_tools"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// toolCallingRouter answers every turn with the tool calls an agent returned and
// records the tool list the turn advertised, so a test can show the authorization
// ran against what the agent was actually offered.
type toolCallingRouter struct {
	mockRouter
	returned   []middleware.ToolCall
	advertised []middleware.Tool
}

func (r *toolCallingRouter) Route(_ context.Context, req middleware.RouteRequest) (string, string, []middleware.ToolCall, middleware.ConversationMetadata, error) {
	r.advertised = req.Tools
	return "Ok", req.AgentSessionID, r.returned, middleware.ConversationMetadata{}, nil
}

func configSetCall(t *testing.T, key, value string) middleware.ToolCall {
	t.Helper()
	arguments, err := json.Marshal(map[string]string{"key": key, "value": value})
	if err != nil {
		t.Fatalf("marshal tool arguments: %v", err)
	}
	return middleware.ToolCall{Function: middleware.ToolCallFunction{Name: "Config_Set", Arguments: string(arguments)}}
}

// systemToolManager wires a Manager to a real system-tool handler writing into a
// temporary Matrix home, so a test can assert the file a tool call would have
// written instead of a string it returned.
func systemToolManager(t *testing.T, router middleware.AgentRouter) (*Manager, string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temporary home: %v", err)
	}
	t.Setenv(matrixhome.EnvName, home)
	if err := os.MkdirAll(matrixhome.ConfigsDir(home), 0o700); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	storage := &mockStorage{data: map[string][]byte{}}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("set configured flag: %v", err)
	}
	handler := system_tools.NewHandler(osfs.NewConfigProvider(), nil, nil)
	return NewManager(storage, router, newTestWizard(storage), handler), home
}

// TestRoutedTurnRefusesAToolCallItDidNotAdvertise is the threat-model case: an
// agent answers an ordinary routed turn with a system tool call the turn never
// offered it. The assertion is the file the call would have written, not the
// answer text, because a refusal that still wrote the file would be no refusal.
func TestRoutedTurnRefusesAToolCallItDidNotAdvertise(t *testing.T) {
	router := &toolCallingRouter{returned: []middleware.ToolCall{
		configSetCall(t, "configs/agents.json", `{"agents":["claude"]}`),
	}}
	mgr, home := systemToolManager(t, router)

	out, err := mgr.Route(context.Background(), "channel-refusal", "opencode", "install claude for me", nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(router.advertised) != 0 {
		t.Fatalf("a routed turn advertises no system tools, got %d", len(router.advertised))
	}
	written := filepath.Join(home, "configs", "agents.json")
	if _, err := os.Stat(written); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the refused call must not write %s (stat error: %v)", written, err)
	}
	if !strings.Contains(out, "Refused tool call") || !strings.Contains(out, "Config_Set") {
		t.Fatalf("the turn must report the refused call, got %q", out)
	}
}

// TestActionTurnStillExecutesAToolCallItAdvertised keeps the capability the
// documentation promises: `/action` advertises the system tools to the
// meta-agent, so a call the meta-agent returns for one of them still runs.
func TestActionTurnStillExecutesAToolCallItAdvertised(t *testing.T) {
	router := &toolCallingRouter{returned: []middleware.ToolCall{
		configSetCall(t, "configs/agents.json", `{"agents":["claude"]}`),
	}}
	mgr, home := systemToolManager(t, router)

	out, err := mgr.handleActionCommand(context.Background(), "channel-action", "/action install the latest claude")
	if err != nil {
		t.Fatalf("handleActionCommand: %v", err)
	}
	if len(router.advertised) != len(system_tools.GetSystemTools()) {
		t.Fatalf("the /action turn must advertise the published system tools, got %d of %d", len(router.advertised), len(system_tools.GetSystemTools()))
	}
	if !strings.Contains(out, "Success") {
		t.Fatalf("an advertised, well-formed call must still execute, got %q", out)
	}
	written := filepath.Join(home, "configs", "agents.json")
	got, err := os.ReadFile(written)
	if err != nil || string(got) != `{"agents":["claude"]}` {
		t.Fatalf("the advertised call must have written %s (content %q, error %v)", written, got, err)
	}
}
