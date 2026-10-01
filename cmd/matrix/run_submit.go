package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runclient"
	"github.com/spf13/cobra"
)

var (
	runSubmitAgent          string
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

The run is accepted, not awaited: the command reports the run_id and the state
it was accepted in. Wait for its outcome with ` + "`matrix run wait <run_id>`" + `,
which consumes the durable notification cursor so an outcome delivered once is
not delivered twice.

Passing --idempotency-key makes a repeated submission of the same key return the
run it already created instead of starting a second one.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		if err := runRunSubmit(cmd); err != nil {
			exitf("Error: %v", err)
		}
	},
}

// submitPromptText resolves the prompt from the flag or the file, refusing an
// empty prompt: a run with no input is accepted by the wire contract and then
// does nothing, which reads as a broken agent rather than a malformed command.
func submitPromptText(prompt, promptFile string) (string, error) {
	inline := strings.TrimSpace(prompt)
	file := strings.TrimSpace(promptFile)
	switch {
	case inline != "" && file != "":
		return "", fmt.Errorf("pass either --prompt or --prompt-file, not both")
	case file != "":
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read prompt file: %w", err)
		}
		if strings.TrimSpace(string(raw)) == "" {
			return "", fmt.Errorf("prompt file %s is empty", file)
		}
		return string(raw), nil
	case inline != "":
		return inline, nil
	default:
		return "", fmt.Errorf("a prompt is required: pass --prompt or --prompt-file")
	}
}

func submitAddress(configValue string) string {
	if configured := strings.TrimSpace(configValue); configured != "" {
		return configured
	}
	return DefaultMatrixHTTPAddr
}

func runRunSubmit(cmd *cobra.Command) error {
	prompt, err := submitPromptText(runSubmitPrompt, runSubmitPromptFile)
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
	fmt.Fprintf(cmd.OutOrStdout(), "wait with: matrix run wait %s\n", result.RunID)
	return nil
}

func init() {
	runSubmitCmd.Flags().StringVar(&runSubmitAgent, "agent", "", "Agent to run with (defaults to the runtime's configured agent)")
	runSubmitCmd.Flags().StringVar(&runSubmitPrompt, "prompt", "", "Prompt text to submit")
	runSubmitCmd.Flags().StringVar(&runSubmitPromptFile, "prompt-file", "", "Read the prompt from a file")
	runSubmitCmd.Flags().StringVar(&runSubmitIdempotencyKey, "idempotency-key", "", "Key that makes a repeated submission return the run it already created")
	runSubmitCmd.Flags().BoolVar(&runSubmitJSON, "json", false, "print the machine-readable submission result")
	runSubmitCmd.Flags().DurationVar(&runSubmitTimeout, "timeout", runclient.DefaultTimeout, "How long to wait for the runtime to accept the run")
	runCmd.AddCommand(runSubmitCmd)
}
