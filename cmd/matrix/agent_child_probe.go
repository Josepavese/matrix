package main

import (
	"fmt"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
)

// childIdentityWait bounds how long the probe waits for the kernel to publish
// the child it just started. It is short on purpose: the answer is available as
// soon as the process exists, and a doctor that lingers is a doctor nobody runs.
const childIdentityWait = 500 * time.Millisecond

// childReport is the doctor's evidence about the child process an endpoint
// actually starts.
type childReport struct {
	Status     string   `json:"status"`
	PID        int      `json:"pid,omitempty"`
	Cwd        string   `json:"cwd,omitempty"`
	Argv       []string `json:"argv,omitempty"`
	Source     string   `json:"source,omitempty"`
	ProcessCwd string   `json:"process_cwd_declared,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// probeChild starts the endpoint's real child, records what the kernel says
// about it, and stops it.
//
// The command is started exactly as the runtime starts it — same argv, same
// environment, same declared process cwd — so what it reports is evidence about
// the launch path rather than a second opinion about the configuration. Reading
// /proc is what makes a launcher visible: an endpoint that reaches its provider
// through `env -C`, or one whose process cwd never changed, is reported as it
// is instead of as it was configured.
func probeChild(endpoint middleware.ProtocolEndpoint, processCwd string) (childReport, []string) {
	if endpoint.Kind != middleware.ProtocolKindACP || endpoint.Transport != "stdio" || endpoint.Command == "" {
		return childReport{}, nil
	}

	handle, err := execprovider.NewProvider().Start(middleware.CommandSpec{
		Runner:       endpoint.Command,
		Args:         endpoint.Args,
		Env:          endpoint.Env,
		EnvIsolation: endpoint.EnvIsolation,
		Dir:          processCwd,
	})
	if err != nil {
		return childReport{
			Status: "not_started",
			Error:  err.Error(),
		}, []string{"child identity not read: the agent command did not start"}
	}
	defer func() {
		_ = handle.Kill()
		_ = handle.Wait()
	}()

	pid := handle.GetPID()
	deadline := time.Now().Add(childIdentityWait)
	var lastErr error
	for {
		identity, err := readChildIdentity(pid)
		// Between fork and exec the kernel publishes the process before its
		// command line, so an empty argv means "not exec'd yet", not "the
		// provider was handed nothing". Reporting it as evidence would measure
		// the reader's timing instead of the launch.
		if err == nil && len(identity.Argv) == 0 {
			err = fmt.Errorf("child command line is empty: the process has not been exec'd yet")
		}
		if err == nil {
			return childReport{
				Status:     "observed",
				PID:        identity.PID,
				Cwd:        identity.Cwd,
				Argv:       identity.Argv,
				Source:     fmt.Sprintf("/proc/%d/{cwd,cmdline}", identity.PID),
				ProcessCwd: processCwd,
			}, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return childReport{
		Status:     "unreadable",
		PID:        pid,
		ProcessCwd: processCwd,
		Error:      lastErr.Error(),
	}, []string{"child identity unreadable: " + lastErr.Error()}
}

// childProcessCwd resolves the process cwd a doctor run should use: the one the
// endpoint declares, refused rather than substituted when it is unusable.
func childProcessCwd(endpoint middleware.ProtocolEndpoint) (string, error) {
	return agentlaunch.ResolveProcessCwd(endpoint)
}
