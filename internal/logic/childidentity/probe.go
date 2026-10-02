package childidentity

import (
	"fmt"
	"os"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
)

// probeWait bounds how long the probe waits for the kernel to publish
// the child it just started. It is short on purpose: the answer is available as
// soon as the process exists, and a doctor that lingers is a doctor nobody runs.
const probeWait = 500 * time.Millisecond

// Report is the doctor's evidence about the child process an endpoint
// actually starts.
type Report struct {
	Status     string   `json:"status"`
	PID        int      `json:"pid,omitempty"`
	Cwd        string   `json:"cwd,omitempty"`
	Argv       []string `json:"argv,omitempty"`
	Source     string   `json:"source,omitempty"`
	ProcessCwd string   `json:"process_cwd_declared,omitempty"`
	StartTicks uint64   `json:"start_ticks,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// Probe starts the endpoint's real child, records what the kernel says
// about it, and stops it.
//
// The command is started exactly as the runtime starts it — same argv, same
// environment, same declared process cwd — so what it reports is evidence about
// the launch path rather than a second opinion about the configuration. Reading
// /proc is what makes a launcher visible: an endpoint that reaches its provider
// through `env -C`, or one whose process cwd never changed, is reported as it
// is instead of as it was configured.
func Probe(endpoint middleware.ProtocolEndpoint, processCwd string) (Report, []string) {
	if endpoint.Kind != middleware.ProtocolKindACP || endpoint.Transport != "stdio" || endpoint.Command == "" {
		return Report{}, nil
	}

	handle, err := execprovider.NewProvider().Start(middleware.CommandSpec{
		Runner:       endpoint.Command,
		Args:         endpoint.Args,
		Env:          endpoint.Env,
		EnvIsolation: endpoint.EnvIsolation,
		Dir:          processCwd,
	})
	if err != nil {
		return Report{
			Status: "not_started",
			Error:  err.Error(),
		}, []string{"child identity not read: the agent command did not start"}
	}
	defer func() {
		_ = handle.Kill()
		_ = handle.Wait()
	}()

	return observeChild(handle.GetPID(), &reuseGuard{parent: os.Getpid()}, processCwd)
}

// observeChild reads what the kernel says about the child until the identity is
// complete or the wait expires.
//
// The pid is a number the kernel recycles, so an observation is only evidence
// while it is the child this probe started: parent and start time have to say so.
// A refused identity is terminal — waiting cannot bring the child back — and it
// is reported as a refusal rather than as the identity of whichever process
// inherited the number.
func observeChild(pid int, guard *reuseGuard, processCwd string) (Report, []string) {
	deadline := time.Now().Add(probeWait)
	var lastErr error
	for {
		identity, err := readIdentity(pid)
		if err == nil {
			if err = guard.accept(identity); err != nil {
				return refusalReport(pid, processCwd, err)
			}
		}
		// Between fork and exec the kernel publishes the process before its
		// command line, so an empty argv means "not exec'd yet", not "the
		// provider was handed nothing". Reporting it as evidence would measure
		// the reader's timing instead of the launch.
		if err == nil && len(identity.Argv) == 0 {
			err = fmt.Errorf("child command line is empty: the process has not been exec'd yet")
		}
		if err == nil {
			return observedReport(identity, processCwd), nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return Report{
		Status:     "unreadable",
		PID:        pid,
		ProcessCwd: processCwd,
		Error:      lastErr.Error(),
	}, []string{"child identity unreadable: " + lastErr.Error()}
}

// refusalReport is the terminal answer for a pid that is no longer the child this
// probe started.
func refusalReport(pid int, processCwd string, err error) (Report, []string) {
	return Report{
		Status:     "unreadable",
		PID:        pid,
		ProcessCwd: processCwd,
		Error:      err.Error(),
	}, []string{"child identity refused: " + err.Error()}
}

// observedReport is the evidence a doctor reports: where the child's process cwd
// actually is, the argv the kernel recorded, and the start time that binds both
// to this pid.
func observedReport(identity Identity, processCwd string) Report {
	return Report{
		Status:     "observed",
		PID:        identity.PID,
		Cwd:        identity.Cwd,
		Argv:       identity.Argv,
		Source:     fmt.Sprintf("/proc/%d/{cwd,cmdline,stat}", identity.PID),
		ProcessCwd: processCwd,
		StartTicks: identity.StartTicks,
	}
}

// DeclaredProcessCwd resolves the process cwd a doctor run should use: the one the
// endpoint declares, refused rather than substituted when it is unusable.
func DeclaredProcessCwd(endpoint middleware.ProtocolEndpoint) (string, error) {
	return agentlaunch.ResolveProcessCwd(endpoint)
}
