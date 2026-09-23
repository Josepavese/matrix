package main

import (
	"encoding/json"
	"os"
	"strings"
)

// ----------------------------------------------------------------------------
// ACP v2 terminal login program
//
// A terminal authentication method does not log in over the wire: the client
// launches the configured agent program with the method's args and env and waits
// for it to exit. This is that second mode of the same binary. It writes what it
// was launched with to the credential file, appends its own line to the shared
// observation file so a test can order it against the ACP traffic, and exits
// zero. The running peer reads the credential once, at startup, which is why the
// client has to reconnect for the login to take effect.
// ----------------------------------------------------------------------------

// peerLogRecord is one line of the observation file. Kind separates the phases
// ("initialize_seen", "terminal_login", ...) and Method is the wire method name,
// so a test can assert both what the peer was asked and in which order.
type peerLogRecord struct {
	Kind    string                 `json:"kind"`
	Method  string                 `json:"method,omitempty"`
	PID     int                    `json:"pid"`
	Args    []string               `json:"args,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// terminalLoginRecord is the credential file the login program leaves behind:
// the token it read from the environment it was launched with, and the argument
// vector it was launched as.
type terminalLoginRecord struct {
	Token string   `json:"token"`
	Args  []string `json:"args"`
	PID   int      `json:"pid"`
	Cwd   string   `json:"cwd,omitempty"`
}

// terminalLoginRequested reports whether this process was started as the login
// program the terminal method names instead of as the ACP peer.
func terminalLoginRequested(args []string) bool {
	return hasArg(args, terminalLoginFlag)
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

// runTerminalLogin is the program the advertised terminal method names. It
// writes what it was launched with to the credential file and exits zero, which
// is how the client's own run of it becomes observable.
func runTerminalLogin() int {
	record := terminalLoginRecord{
		Token: os.Getenv(envBaseToken),
		Args:  append([]string{}, os.Args...),
		PID:   os.Getpid(),
	}
	if cwd, err := os.Getwd(); err == nil {
		record.Cwd = cwd
	}
	path := strings.TrimSpace(os.Getenv(envCredentialPath))
	if path == "" {
		return 2
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return 3
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return 4
	}
	appendPeerLog(strings.TrimSpace(os.Getenv(envPeerLogPath)), peerLogRecord{
		Kind: "terminal_login",
		PID:  record.PID,
		Args: record.Args,
		Details: map[string]interface{}{
			"token": record.Token,
		},
	})
	return 0
}

func appendPeerLog(path string, record peerLogRecord) {
	if path == "" {
		return
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = file.Write(append(encoded, '\n'))
}
