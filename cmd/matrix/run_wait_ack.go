//go:build linux || darwin

package main

import (
	"encoding/json"
	"fmt"
	"strings"
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
	var elicitations []elicitationSummary
	for {
		page, err := client.wakeups(cmd.Context(), wakeupQuery{After: cursor, RunID: runID})
		if err != nil {
			return err
		}
		if page.NextCursor > cursor {
			cursor = page.NextCursor
		}
		for _, wakeup := range page.Notifications {
			// A run blocked on a question a person has to answer is not
			// terminal, and saying nothing about it left the caller waiting
			// with no idea what was holding the run up. The elicitation
			// lifecycle already travels in this stream, so the summary is
			// reported from what arrived rather than asked for separately.
			if summary, isElicitation := elicitationSummaryOf(wakeup); isElicitation {
				elicitations = append(elicitations, summary)
				if !runWaitJSON {
					printElicitation(cmd, summary)
				}
				continue
			}
			outcome, terminal := wakeupOutcome(wakeup.Kind)
			if !terminal {
				continue
			}
			printWakeupOutcome(cmd, wakeupReport{
				RunID: wakeup.RunID, Outcome: outcome, Kind: wakeup.Kind,
				Sequence: wakeup.Sequence, Cursor: cursor, FailureCode: wakeup.FailureCode,
				Elicitations: elicitations,
			})
			return nil
		}
		if time.Now().After(deadline) {
			printWakeupTimeout(cmd, wakeupReport{
				RunID: runID, Outcome: "timeout", Cursor: cursor, Elicitations: elicitations,
			})
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
	Kind        string `json:"kind,omitempty"`
	Sequence    uint64 `json:"sequence,omitempty"`
	Cursor      uint64 `json:"cursor"`
	FailureCode string `json:"failure_code,omitempty"`
	// Elicitations are the questions the run is blocked on, reported with the
	// identity Matrix recorded for them and the state the stream announced.
	Elicitations []elicitationSummary `json:"elicitations,omitempty"`
}

// elicitationSummary is the short form of one elicitation: which one, whether it
// is open or answered, since when, and the question it is asking.
//
// The question IS carried, and it is user content — the text the person being
// asked has to answer. It is bounded where the notification is written, with the
// producer's own marker: Truncated is a boolean rather than a suffix to
// interpret, and the text travels with its spaces collapsed so one elicitation
// stays one line. A trace policy can redact it, and only the opening event
// carries it. The caller must therefore treat the field as possibly absent: an
// empty Question means "not carried here", and it never means "there is no
// question".
type elicitationSummary struct {
	ID        string `json:"id,omitempty"`
	State     string `json:"state"`
	SessionID string `json:"session_id,omitempty"`
	Since     string `json:"since,omitempty"`
	Question  string `json:"question,omitempty"`
	Truncated bool   `json:"question_truncated,omitempty"`
}

// elicitationSummaryOf recognises the elicitation lifecycle by prefix rather
// than by comparing the kind against a name: the state is whatever the producer
// appended, so a new state reaches this summary without a code change.
func elicitationSummaryOf(wakeup notificationWakeup) (elicitationSummary, bool) {
	state, isElicitation := strings.CutPrefix(strings.TrimSpace(wakeup.Kind), "elicitation.")
	if !isElicitation || strings.TrimSpace(state) == "" {
		return elicitationSummary{}, false
	}
	return elicitationSummary{
		ID: wakeup.ElicitationID, State: state,
		SessionID: wakeup.SessionID, Since: wakeup.Timestamp.UTC().Format(time.RFC3339),
		Question: wakeup.Question, Truncated: wakeup.QuestionTruncated,
	}, true
}

func printElicitation(cmd *cobra.Command, summary elicitationSummary) {
	cmd.Printf("elicitation id=%s state=%s session=%s since=%s%s\n",
		summary.ID, summary.State, summary.SessionID, summary.Since, questionSuffix(summary))
}

// questionSuffix renders the question when the notification carried one, quoted
// because it is a sentence a person is being asked rather than a token, and
// marked when the record says it was cut at the bound. When nothing was carried
// the suffix is empty: the summary keeps reporting the elicitation without
// inventing text for it.
func questionSuffix(summary elicitationSummary) string {
	if summary.Question == "" {
		return ""
	}
	suffix := fmt.Sprintf(" question=%q", summary.Question)
	if summary.Truncated {
		suffix += " truncated=true"
	}
	return suffix
}

func printWakeupOutcome(cmd *cobra.Command, report wakeupReport) {
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

func printWakeupTimeout(cmd *cobra.Command, report wakeupReport) {
	if runWaitJSON {
		encoded, err := json.Marshal(report)
		if err == nil {
			cmd.Println(string(encoded))
		}
		return
	}
	for _, summary := range report.Elicitations {
		printElicitation(cmd, summary)
	}
	cmd.Printf("run_id=%s outcome=timeout cursor=%d\n", report.RunID, report.Cursor)
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
