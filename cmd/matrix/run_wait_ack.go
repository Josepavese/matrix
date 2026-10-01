//go:build linux || darwin

package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// This file is the CLI surface for the delivery primitives: flags, printing and
// exit codes. The conversation with the daemon's socket lives in
// run_notifications_client.go, so a change in the wire contract and a change in
// what an operator types do not edit the same function.

var (
	runWaitAfter   uint64
	runWaitTimeout time.Duration
	runWaitJSON    bool

	runAckRunID          string
	runAckSequence       uint64
	runAckIdempotencyKey string
	runAckJSON           bool
)

var runWaitCmd = &cobra.Command{
	Use:   "wait <run_id>",
	Short: "Wait for a run's terminal outcome over the local notification socket",
	Long: `Wait for one run to reach a terminal outcome.

The command consumes the durable notification cursor: pass --after to resume
from the cursor a previous invocation printed, and it prints the new cursor on
every exit path, so a supervisor restart neither replays deliveries nor loses
them. What it reports is the run's outcome, never the turn: the wakeup record
carries no content and this command adds none.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runWait(cmd, args[0]); err != nil {
			exitf("Error: %v", err)
		}
	},
}

var runAckCmd = &cobra.Command{
	Use:   "ack",
	Short: "Acknowledge one delivered run wakeup exactly once",
	Long: `Acknowledge one delivered wakeup.

The idempotency key is required and is not generated: the same key with the same
claim is a replay and reports success, while the same key with a different claim
is refused. A random key per invocation would make every retry a second claim,
which is the opposite of what an acknowledgement is for.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		if err := runAck(cmd); err != nil {
			exitf("Error: %v", err)
		}
	},
}

func runWait(cmd *cobra.Command, runID string) error {
	home, err := resolveActiveHome()
	if err != nil {
		return err
	}
	return runWaitAt(cmd, notificationSocketPath(home), runID)
}

// runWaitAt waits on one socket path. The path is a parameter so the wait
// contract can be exercised against the daemon's own handler without a running
// daemon.
func runWaitAt(cmd *cobra.Command, socketPath, runID string) error {
	client := newLocalNotificationClient(socketPath, localSurfaceAPIKey())

	deadline := time.Now().Add(runWaitTimeout)
	cursor := runWaitAfter
	for {
		page, err := client.wakeups(cmd.Context(), wakeupQuery{After: cursor, RunID: runID})
		if err != nil {
			return err
		}
		if page.NextCursor > cursor {
			cursor = page.NextCursor
		}
		for _, wakeup := range page.Notifications {
			outcome, terminal := wakeupOutcome(wakeup.Kind)
			if !terminal {
				continue
			}
			printWakeupOutcome(cmd, wakeup, outcome, cursor)
			return nil
		}
		if time.Now().After(deadline) {
			printWakeupTimeout(cmd, runID, cursor)
			return fmt.Errorf("run %s reached no terminal outcome within %s (resume with --after %d)", runID, runWaitTimeout, cursor)
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-time.After(notificationPollInterval):
		}
	}
}

type wakeupReport struct {
	RunID       string `json:"run_id"`
	Outcome     string `json:"outcome"`
	Kind        string `json:"kind"`
	Sequence    uint64 `json:"sequence"`
	Cursor      uint64 `json:"cursor"`
	FailureCode string `json:"failure_code,omitempty"`
}

func printWakeupOutcome(cmd *cobra.Command, wakeup notificationWakeup, outcome string, cursor uint64) {
	report := wakeupReport{
		RunID:       wakeup.RunID,
		Outcome:     outcome,
		Kind:        wakeup.Kind,
		Sequence:    wakeup.Sequence,
		Cursor:      cursor,
		FailureCode: wakeup.FailureCode,
	}
	if runWaitJSON {
		encoded, err := json.Marshal(report)
		if err != nil {
			exitf("Error: %v", err)
		}
		cmd.Println(string(encoded))
		return
	}
	if report.FailureCode != "" {
		cmd.Printf("run_id=%s outcome=%s kind=%s sequence=%d cursor=%d failure_code=%s\n",
			report.RunID, report.Outcome, report.Kind, report.Sequence, report.Cursor, report.FailureCode)
		return
	}
	cmd.Printf("run_id=%s outcome=%s kind=%s sequence=%d cursor=%d\n",
		report.RunID, report.Outcome, report.Kind, report.Sequence, report.Cursor)
}

func printWakeupTimeout(cmd *cobra.Command, runID string, cursor uint64) {
	if runWaitJSON {
		encoded, err := json.Marshal(map[string]any{"run_id": runID, "outcome": "timeout", "cursor": cursor})
		if err == nil {
			cmd.Println(string(encoded))
		}
		return
	}
	cmd.Printf("run_id=%s outcome=timeout cursor=%d\n", runID, cursor)
}

func runAck(cmd *cobra.Command) error {
	home, err := resolveActiveHome()
	if err != nil {
		return err
	}
	client := newLocalNotificationClient(notificationSocketPath(home), localSurfaceAPIKey())
	replayed, err := client.ack(cmd.Context(), ackRequest{
		RunID:          runAckRunID,
		Sequence:       runAckSequence,
		IdempotencyKey: runAckIdempotencyKey,
	})
	if err != nil {
		return err
	}
	if runAckJSON {
		encoded, encodeErr := json.Marshal(map[string]any{
			"status": "acked", "run_id": runAckRunID, "sequence": runAckSequence, "replayed": replayed,
		})
		if encodeErr != nil {
			return encodeErr
		}
		cmd.Println(string(encoded))
		return nil
	}
	cmd.Printf("status=acked run_id=%s sequence=%d replayed=%t\n", runAckRunID, runAckSequence, replayed)
	return nil
}

func init() {
	runWaitCmd.Flags().Uint64Var(&runWaitAfter, "after", 0, "notification cursor to resume from")
	runWaitCmd.Flags().DurationVar(&runWaitTimeout, "timeout", 60*time.Second, "how long to wait for a terminal outcome")
	runWaitCmd.Flags().BoolVar(&runWaitJSON, "json", false, "print one machine-readable line")

	runAckCmd.Flags().StringVar(&runAckRunID, "run-id", "", "run the wakeup belongs to")
	runAckCmd.Flags().Uint64Var(&runAckSequence, "sequence", 0, "notification sequence being acknowledged")
	runAckCmd.Flags().StringVar(&runAckIdempotencyKey, "idempotency-key", "", "stable key that makes the acknowledgement exactly-once")
	runAckCmd.Flags().BoolVar(&runAckJSON, "json", false, "print one machine-readable line")

	for _, flag := range []string{"run-id", "sequence", "idempotency-key"} {
		if err := runAckCmd.MarkFlagRequired(flag); err != nil {
			panic(err)
		}
	}

	runCmd.AddCommand(runWaitCmd, runAckCmd)
}
