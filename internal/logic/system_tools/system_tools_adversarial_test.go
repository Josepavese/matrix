package system_tools

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestGetSystemToolsPublishesAUniqueWellFormedContract pins the machine-readable
// tool surface: an agent discovers these by name and schema, so a duplicate name
// or an empty schema is a broken contract, not a style issue.
func TestGetSystemToolsPublishesAUniqueWellFormedContract(t *testing.T) {
	tools := GetSystemTools()
	if len(tools) == 0 {
		t.Fatal("the system must publish at least one tool")
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		if tool.Name == "" {
			t.Fatal("a tool without a name cannot be called")
		}
		if seen[tool.Name] {
			t.Fatalf("tool %q is published twice; a caller could not choose between them", tool.Name)
		}
		seen[tool.Name] = true
		if tool.Description == "" {
			t.Fatalf("tool %q has no description; an agent cannot decide when to use it", tool.Name)
		}
		if len(tool.InputSchema) == 0 {
			t.Fatalf("tool %q has no input schema", tool.Name)
		}
		if schemaType, ok := tool.InputSchema["type"].(string); !ok || schemaType != "object" {
			t.Fatalf("tool %q must declare an object input schema, got %v", tool.Name, tool.InputSchema["type"])
		}
	}
	// The published names are the visible contract: pin them so a rename is a
	// deliberate, reviewed change rather than an accident.
	for _, expected := range []string{"APM_Install", "APM_Uninstall", "Config_Set"} {
		if !seen[expected] {
			t.Fatalf("tool %q disappeared from the published contract: %v", expected, keys(seen))
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

// TestExecuteToolRejectsAnUnknownTool keeps an unknown name from silently
// succeeding: an agent that mistypes a tool must learn that nothing happened.
func TestExecuteToolRejectsAnUnknownTool(t *testing.T) {
	handler := NewHandler(nil, nil, nil)
	result := handler.ExecuteTool(middleware.ToolCall{Function: middleware.ToolCallFunction{Name: "NoSuchTool", Arguments: "{}"}})
	if result == "" {
		t.Fatal("an unknown tool must produce an explicit answer")
	}
	if !strings.Contains(strings.ToLower(result), "unknown") && !strings.Contains(strings.ToLower(result), "not") {
		t.Fatalf("the answer must say the tool is not known, got %q", result)
	}
	// A known tool with missing required arguments must also refuse, not panic.
	// APM_Install needs an agent name; the handler must answer, not crash.
	if got := handler.ExecuteTool(middleware.ToolCall{Function: middleware.ToolCallFunction{Name: "APM_Install", Arguments: "{}"}}); got == "" {
		t.Fatal("a known tool with missing arguments must answer rather than stay silent")
	}
}
