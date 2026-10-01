package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/providers/runapi"
	"github.com/spf13/cobra"
)

// Submitting a run is the first half of the delivery primitive: `run wait`
// consumes the outcome of a run, and something has to start it. This command
// speaks the surface that already exists — POST /v1/runs on the Matrix HTTP API,
// the same route and the same key a client would use by hand — so it adds a
// command, not a contract.
//
// The run is accepted, not awaited: the answer carries the run_id and the URLs
// the rest of the surface is built from, and the caller waits with
// `matrix run wait <run_id>`, which consumes the durable notification cursor.

const defaultRunSubmitTimeout = 30 * time.Second

// runSubmitRequest is the body Matrix accepts. `input` is sent as the compact
// string form the payload contract defines; the agent is named only when the
// caller chose one, so the runtime's configured default still decides.
type runSubmitRequest struct {
	AgentID string `json:"agent_id,omitempty"`
	Input   string `json:"input"`
}

// runSubmitInput is everything one submission needs, so the request is built and
// exercised without a running daemon.
type runSubmitInput struct {
	Address        string
	APIKey         string
	AgentID        string
	Prompt         string
	IdempotencyKey string
	Timeout        time.Duration
}

// runSubmitResult is what the caller needs to continue: which run it is, what
// state it was accepted in, whether the answer was a replay of an earlier
// submission of the same key, and where its trace lives.
type runSubmitResult struct {
	RunID    string `json:"run_id"`
	Status   string `json:"status"`
	Replayed bool   `json:"replayed"`
	TraceURL string `json:"trace_url,omitempty"`
}

// matrixHTTPBaseURL turns a configured bind address into the URL a local client
// must dial. A daemon bound to every interface listens on loopback too, so the
// client dials the loopback of the same family instead of the wildcard address.
func matrixHTTPBaseURL(address string) (string, error) {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return "", fmt.Errorf("no Matrix HTTP address is configured")
	}
	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		return "", fmt.Errorf("matrix HTTP address %q is not host:port: %w", trimmed, err)
	}
	switch trimmedHost := strings.Trim(host, "[]"); trimmedHost {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port), nil
}

// submitTimeoutOf bounds how long a submission waits for the runtime to accept
// the run, so a caller cannot leave the command hanging on a silent daemon.
func submitTimeoutOf(configured time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	return defaultRunSubmitTimeout
}

// buildSubmitRequest turns the input into the request Matrix publishes: the
// route, the body contract, the surface credential and the idempotency key.
// Nothing here talks to the network, so the request a caller would send can be
// read and asserted without a daemon.
func buildSubmitRequest(ctx context.Context, input runSubmitInput) (*http.Request, error) {
	baseURL, err := matrixHTTPBaseURL(input.Address)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(runSubmitRequest{AgentID: input.AgentID, Input: input.Prompt})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+runapi.RunPathV1, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(input.APIKey) != "" {
		request.Header.Set("X-Matrix-Key", input.APIKey)
	}
	if key := strings.TrimSpace(input.IdempotencyKey); key != "" {
		request.Header.Set(ackIdempotencyKeyHeader, key)
	}
	return request, nil
}

// readSubmitResponse turns the answer into the accepted run, and refuses
// anything that is not an acceptance. A caller that cannot tell an accepted run
// from a refused one has no way to know whether waiting is meaningful.
func readSubmitResponse(response *http.Response) (runSubmitResult, error) {
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return runSubmitResult{}, fmt.Errorf("submit refused: %w", err)
	}
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return runSubmitResult{}, fmt.Errorf("submit refused: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	var decoded struct {
		RunID    string `json:"run_id"`
		Status   string `json:"status"`
		TraceURL string `json:"trace_url"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return runSubmitResult{}, fmt.Errorf("submit answered a body that is not the run response: %w", err)
	}
	if strings.TrimSpace(decoded.RunID) == "" {
		return runSubmitResult{}, fmt.Errorf("submit answered no run_id, so there is nothing to wait on")
	}
	return runSubmitResult{
		RunID:    decoded.RunID,
		Status:   decoded.Status,
		Replayed: strings.EqualFold(response.Header.Get(ackReplayedHeader), "true"),
		TraceURL: decoded.TraceURL,
	}, nil
}

// submitRun posts one run and reports the accepted identity.
func submitRun(ctx context.Context, input runSubmitInput) (runSubmitResult, error) {
	timeout := submitTimeoutOf(input.Timeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := buildSubmitRequest(ctx, input)
	if err != nil {
		return runSubmitResult{}, err
	}
	response, err := (&http.Client{Timeout: timeout}).Do(request)
	if err != nil {
		return runSubmitResult{}, fmt.Errorf("submit refused: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	return readSubmitResponse(response)
}

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
	result, err := submitRun(cmd.Context(), runSubmitInput{
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
	runSubmitCmd.Flags().DurationVar(&runSubmitTimeout, "timeout", defaultRunSubmitTimeout, "How long to wait for the runtime to accept the run")
	runCmd.AddCommand(runSubmitCmd)
}
