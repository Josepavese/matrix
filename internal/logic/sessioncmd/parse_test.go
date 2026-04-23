package sessioncmd

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOK  bool
		command string
		args    string
	}{
		{name: "empty", input: "  ", wantOK: false},
		{name: "not command", input: "hello /session list", wantOK: false},
		{name: "command only", input: " /STATUS ", wantOK: true, command: "/status"},
		{name: "command with args", input: "/session delete abc", wantOK: true, command: "/session", args: "delete abc"},
		{name: "prefixed token stays unknown command", input: "/session-list", wantOK: true, command: "/session-list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Parse(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("Parse ok=%v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.Command != tt.command || got.Args != tt.args {
				t.Fatalf("Parse()=%+v, want command=%q args=%q", got, tt.command, tt.args)
			}
		})
	}
}
