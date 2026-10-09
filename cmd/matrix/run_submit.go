package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runclient"
	"github.com/spf13/cobra"
)

var (
	runSubmitAgent          string
	runSubmitModel          string
	runSubmitChannel        string
	runSubmitWorkspace      string
	runSubmitPrompt         string
	runSubmitPromptFile     string
	runSubmitIdempotencyKey string
	runSubmitJSON           bool
	runSubmitTimeout        time.Duration
)

var runSubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit a run and report the run_id to wait on",
	Long: `Submit a run to this Matrix runtime.

The run is submitted asynchronously: the command reports its run_id and accepted
state. On Linux/macOS, wait with ` + "`matrix run wait <run_id>`" + ` and persist
the returned cursor for reconnects. On Windows, consume the HTTP run events.

The default channel is cli.run.submit. Use --channel to separate callers and idempotency scopes, and
--workspace to name a registered project. No workspace is inferred from the
invoking shell's directory.

Passing --idempotency-key makes a repeated submission of the same key return the
run it already created instead of starting a second one.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		if err := runRunSubmit(cmd); err != nil {
			exitf("Error: %v", err)
		}
	},
}

func submitAddress(configValue string) string {
	if configured := strings.TrimSpace(configValue); configured != "" {
		return configured
	}
	return DefaultMatrixHTTPAddr
}

func runRunSubmit(cmd *cobra.Command) error {
	if strings.TrimSpace(runSubmitChannel) == "" {
		return fmt.Errorf("--channel must not be blank")
	}
	prompt, err := runclient.PromptText(runSubmitPrompt, runSubmitPromptFile)
	if err != nil {
		return err
	}
	address, apiKey, err := matrixSurfaceConfig()
	if err != nil {
		return err
	}
	result, err := runclient.Submit(cmd.Context(), runclient.Input{
		Address:        submitAddress(address),
		APIKey:         apiKey,
		AgentID:        strings.TrimSpace(runSubmitAgent),
		ModelID:        strings.TrimSpace(runSubmitModel),
		ChannelID:      strings.TrimSpace(runSubmitChannel),
		WorkspaceID:    strings.TrimSpace(runSubmitWorkspace),
		Prompt:         prompt,
		IdempotencyKey: runSubmitIdempotencyKey,
		Timeout:        runSubmitTimeout,
	})
	if err != nil {
		return err
	}
	if runSubmitJSON {
		encoded, encodeErr := json.MarshalIndent(result, "", "  ")
		if encodeErr != nil {
			return encodeErr
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "run_id=%s status=%s replayed=%t\n", result.RunID, result.Status, result.Replayed)
	fmt.Fprintf(cmd.OutOrStdout(), "events: /v1/runs/%s/events\n", result.RunID)
	fmt.Fprintf(cmd.OutOrStdout(), "Linux/macOS wait: matrix run wait %s\n", result.RunID)
	return nil
}

func init() {
	runSubmitCmd.Flags().StringVar(&runSubmitModel, "model", "", "Optional model selector; requires support from the selected agent")
	runSubmitCmd.Flags().StringVar(&runSubmitAgent, "agent", "", "Agent to run with (defaults to the runtime's configured agent)")
	runSubmitCmd.Flags().StringVar(&runSubmitChannel, "channel", runclient.DefaultChannelID, "Caller channel; also scopes session bindings and idempotency keys")
	runSubmitCmd.Flags().StringVar(&runSubmitWorkspace, "workspace", "", "Registered workspace ID; omitted leaves resolution to the runtime's channel/session binding")
	runSubmitCmd.Flags().StringVar(&runSubmitPrompt, "prompt", "", "Prompt text to submit")
	runSubmitCmd.Flags().StringVar(&runSubmitPromptFile, "prompt-file", "", "Read the prompt from a file")
	runSubmitCmd.Flags().StringVar(&runSubmitIdempotencyKey, "idempotency-key", "", "Key that makes a repeated submission return the run it already created")
	runSubmitCmd.Flags().BoolVar(&runSubmitJSON, "json", false, "print the machine-readable submission result")
	runSubmitCmd.Flags().DurationVar(&runSubmitTimeout, "timeout", runclient.DefaultTimeout, "How long to wait for the runtime to accept the run")
	runCmd.AddCommand(runSubmitCmd)
}
