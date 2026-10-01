package runclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/providers/runapi"
)

// Package runclient speaks the Matrix run surface as a local client: it submits
// a run and reads the acceptance — the route, the body contract, the surface
// credential, and the refusal that must never read as an accepted run — and it
// publishes the idempotency contract every keyed call on that surface is sent
// and answered under.
//
// Submitting a run is the first half of the delivery primitive: `run wait`
// consumes the outcome of a run, and something has to start it. This command
// speaks the surface that already exists — POST /v1/runs on the Matrix HTTP API,
// the same route and the same key a client would use by hand — so it adds a
// command, not a contract.
//
// The run is accepted, not awaited: the answer carries the run_id and the URLs
// the rest of the surface is built from, and the caller waits with
// `matrix run wait <run_id>`, which consumes the durable notification cursor.

const DefaultTimeout = 30 * time.Second

// Request is the body Matrix accepts. `input` is sent as the compact
// string form the payload contract defines; the agent is named only when the
// caller chose one, so the runtime's configured default still decides.
type Request struct {
	AgentID string `json:"agent_id,omitempty"`
	Input   string `json:"input"`
}

// Input is everything one submission needs, so the request is built and
// exercised without a running daemon.
type Input struct {
	Address        string
	APIKey         string
	AgentID        string
	Prompt         string
	IdempotencyKey string
	Timeout        time.Duration
}

// Result is what the caller needs to continue: which run it is, what
// state it was accepted in, whether the answer was a replay of an earlier
// submission of the same key, and where its trace lives.
type Result struct {
	RunID    string `json:"run_id"`
	Status   string `json:"status"`
	Replayed bool   `json:"replayed"`
	TraceURL string `json:"trace_url,omitempty"`
}

// BaseURL turns a configured bind address into the URL a local client
// must dial. A daemon bound to every interface listens on loopback too, so the
// client dials the loopback of the same family instead of the wildcard address.
func BaseURL(address string) (string, error) {
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

// TimeoutOf bounds how long a submission waits for the runtime to accept
// the run, so a caller cannot leave the command hanging on a silent daemon.
func TimeoutOf(configured time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	return DefaultTimeout
}

// buildRequest turns the input into the request Matrix publishes: the
// route, the body contract, the surface credential and the idempotency key.
// Nothing here talks to the network, so the request a caller would send can be
// read and asserted without a daemon.
func buildRequest(ctx context.Context, input Input) (*http.Request, error) {
	baseURL, err := BaseURL(input.Address)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(Request{AgentID: input.AgentID, Input: input.Prompt})
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
		request.Header.Set(IdempotencyKeyHeader, key)
	}
	return request, nil
}

// readResponse turns the answer into the accepted run, and refuses
// anything that is not an acceptance. A caller that cannot tell an accepted run
// from a refused one has no way to know whether waiting is meaningful.
func readResponse(response *http.Response) (Result, error) {
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return Result{}, fmt.Errorf("submit refused: %w", err)
	}
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("submit refused: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	var decoded struct {
		RunID    string `json:"run_id"`
		Status   string `json:"status"`
		TraceURL string `json:"trace_url"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return Result{}, fmt.Errorf("submit answered a body that is not the run response: %w", err)
	}
	if strings.TrimSpace(decoded.RunID) == "" {
		return Result{}, fmt.Errorf("submit answered no run_id, so there is nothing to wait on")
	}
	return Result{
		RunID:    decoded.RunID,
		Status:   decoded.Status,
		Replayed: strings.EqualFold(response.Header.Get(IdempotencyReplayedHeader), "true"),
		TraceURL: decoded.TraceURL,
	}, nil
}

// Submit posts one run and reports the accepted identity.
func Submit(ctx context.Context, input Input) (Result, error) {
	timeout := TimeoutOf(input.Timeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := buildRequest(ctx, input)
	if err != nil {
		return Result{}, err
	}
	response, err := (&http.Client{Timeout: timeout}).Do(request)
	if err != nil {
		return Result{}, fmt.Errorf("submit refused: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	return readResponse(response)
}
