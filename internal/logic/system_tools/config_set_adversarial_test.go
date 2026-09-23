package system_tools

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/matrixhome"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

func configSetHandler(t *testing.T, home string) *Handler {
	t.Helper()
	t.Setenv(matrixhome.EnvName, home)
	return NewHandler(osfs.NewConfigProvider(), nil, nil)
}

func configSetCall(t *testing.T, key, value string) middleware.ToolCall {
	t.Helper()
	arguments, err := json.Marshal(map[string]string{"key": key, "value": value})
	if err != nil {
		t.Fatalf("marshal tool arguments: %v", err)
	}
	return middleware.ToolCall{Function: middleware.ToolCallFunction{Name: "Config_Set", Arguments: string(arguments)}}
}

// TestConfigSetRefusesAPathOutsideTheConfigDirectory drives the refusal through
// the tool call an agent actually sends: Config_Set takes its key from the model,
// so the write has to be confined end to end, not only in the provider.
func TestConfigSetRefusesAPathOutsideTheConfigDirectory(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temporary home: %v", err)
	}
	if err := os.MkdirAll(matrixhome.ConfigsDir(home), 0o700); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	handler := configSetHandler(t, home)

	outside := filepath.Join(t.TempDir(), "escape.json")
	result := handler.ExecuteTool(configSetCall(t, outside, "pwned"))
	if !strings.Contains(result, "Error") {
		t.Fatalf("an escaping key must be answered with an error, got %q", result)
	}
	if !strings.Contains(result, outside) || !strings.Contains(result, matrixhome.ConfigsDir(home)) {
		t.Fatalf("the answer must name the refused path %q and the allowed root %q, got %q", outside, matrixhome.ConfigsDir(home), result)
	}
	if _, err := os.Stat(outside); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the refused tool call must not create %s (stat error: %v)", outside, err)
	}

	// The same tool still writes a configuration file inside the directory.
	result = handler.ExecuteTool(configSetCall(t, "configs/agents.json", `{"agents":["claude"]}`))
	if !strings.HasPrefix(result, "Success") {
		t.Fatalf("a key inside the config directory must succeed, got %q", result)
	}
	if got, err := os.ReadFile(filepath.Join(home, "configs", "agents.json")); err != nil || string(got) != `{"agents":["claude"]}` {
		t.Fatalf("the config file was not written as asked (content %q, error %v)", got, err)
	}
}
