//go:build linux || darwin

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/cmdutil"
	"github.com/Josepavese/matrix/internal/logic/matrixhome"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

// The local notification socket is the delivery channel for run wakeups. It is
// a socket under the Matrix home, so a client finds it without guessing a port,
// and it carries the same API key requirement as the daemon's TCP surfaces.
const runNotificationsSocketRelPath = "data/run-notifications.sock"

const (
	runNotificationsPath    = "/v1/run-notifications"
	runNotificationsAckPath = "/v1/run-notifications/ack"

	// ackIdempotencyKeyHeader is what makes an acknowledgement exactly-once
	// Matrix-side. It is required and never generated here: a fresh key per
	// invocation would turn every retry into a second claim.
	ackIdempotencyKeyHeader = "Idempotency-Key"
	// ackReplayedHeader marks a response that returns a recorded outcome
	// instead of recording a new one. The body is identical either way.
	ackReplayedHeader = "Idempotency-Replayed"

	notificationPollInterval = 500 * time.Millisecond
)

// notificationWakeup is the content-minimal record the socket publishes: which
// run woke up and why, never what the agent said. Declaring the fields here,
// rather than passing the response through, is what keeps a turn's content from
// reaching this client by accident.
type notificationWakeup struct {
	Sequence    uint64    `json:"sequence"`
	Kind        string    `json:"kind"`
	RunID       string    `json:"run_id,omitempty"`
	AgentID     string    `json:"agent_id,omitempty"`
	FailureCode string    `json:"failure_code,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

type notificationPage struct {
	Notifications []notificationWakeup `json:"notifications"`
	NextCursor    uint64               `json:"next_cursor"`
}

// wakeupQuery is one read of the cursor: everything after a sequence, narrowed
// to one run when the caller is waiting for a specific outcome.
type wakeupQuery struct {
	After uint64
	RunID string
}

// ackRequest is one acknowledgement. IdempotencyKey is part of the request, not
// an optional extra: an acknowledgement without a stable key cannot be recorded
// exactly once, so there is no valid request that omits it.
type ackRequest struct {
	RunID          string
	Sequence       uint64
	IdempotencyKey string
}

// localNotificationClient speaks to the daemon's notification socket and to
// nothing else.
type localNotificationClient struct {
	http   *http.Client
	apiKey string
}

func notificationSocketPath(home string) string {
	return filepath.Join(home, runNotificationsSocketRelPath)
}

func newLocalNotificationClient(socketPath, apiKey string) localNotificationClient {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return localNotificationClient{
		apiKey: apiKey,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

// request builds a request against the socket. The host in the URL is a
// placeholder: the transport dials the socket path regardless.
func (c localNotificationClient) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, "http://run-notifications.local"+path, body)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		request.Header.Set("X-Matrix-Key", c.apiKey)
	}
	return request, nil
}

// wakeups reads the wakeups recorded after a cursor. The cursor is the client's
// own resume point: it is printed on every exit path so a restarted supervisor
// continues where the previous one stopped instead of replaying.
func (c localNotificationClient) wakeups(ctx context.Context, query wakeupQuery) (notificationPage, error) {
	params := url.Values{}
	params.Set("after", strconv.FormatUint(query.After, 10))
	if query.RunID != "" {
		params.Set("run_id", query.RunID)
	}
	request, err := c.request(ctx, http.MethodGet, runNotificationsPath+"?"+params.Encode(), nil)
	if err != nil {
		return notificationPage{}, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return notificationPage{}, fmt.Errorf("notification socket unreachable: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return notificationPage{}, err
	}
	if response.StatusCode != http.StatusOK {
		return notificationPage{}, fmt.Errorf("notification socket refused the read: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	var page notificationPage
	if err := json.Unmarshal(payload, &page); err != nil {
		return notificationPage{}, fmt.Errorf("invalid notification payload: %w", err)
	}
	return page, nil
}

// ack records one delivered wakeup. A replay reports success and says so; a
// conflict is a failure, because the daemon refused to overwrite a claim that
// was already made with the same key.
func (c localNotificationClient) ack(ctx context.Context, request ackRequest) (bool, error) {
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return false, fmt.Errorf("%s is required: an acknowledgement without a stable key cannot be recorded exactly once", ackIdempotencyKeyHeader)
	}
	body, err := json.Marshal(map[string]any{"run_id": request.RunID, "sequence": request.Sequence})
	if err != nil {
		return false, err
	}
	httpRequest, err := c.request(ctx, http.MethodPost, runNotificationsAckPath, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set(ackIdempotencyKeyHeader, request.IdempotencyKey)

	response, err := c.http.Do(httpRequest)
	if err != nil {
		return false, fmt.Errorf("notification socket unreachable: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return false, err
	}
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("acknowledgement refused: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	return response.Header.Get(ackReplayedHeader) == "true", nil
}

// wakeupOutcome reports the run status a wakeup announces, and whether the
// wakeup is terminal at all.
//
// The kind is the run status under the runtime's own prefix, and the statuses
// compared here are the run vocabulary's exported values. Which kinds exist is
// not restated: a wakeup announcing a status this client does not recognise is
// not treated as terminal, it is reported as observed.
func wakeupOutcome(kind string) (string, bool) {
	status, found := strings.CutPrefix(strings.TrimSpace(kind), "run.")
	if !found {
		return "", false
	}
	switch status {
	case runtrace.StatusCompleted, runtrace.StatusFailed, runtrace.StatusCancelled, runtrace.StatusUnknown:
		return status, true
	default:
		return status, false
	}
}

// localSurfaceAPIKey reads the key the local notification socket enforces.
//
// The socket is served by the Matrix HTTP API server, which authenticates with
// matrix_api_key; daemon_api_key belongs to the JSON-RPC broker on its own port
// and is a different value. Reading the wrong one produced a 401 on every local
// command while the request itself was correct. An install without a key
// answers local clients without authentication.
func localSurfaceAPIKey() string {
	manager, cleanup, err := cmdutil.OpenReadOnlyConfigManager(DefaultVaultPath)
	if err != nil {
		return ""
	}
	defer cleanup()
	return manager.GetWithDefault("matrix_api_key", "")
}

// resolveActiveHome is the Matrix home whose socket this client talks to.
func resolveActiveHome() (string, error) {
	if activeMatrixHome != "" {
		return activeMatrixHome, nil
	}
	if home := os.Getenv(matrixhome.EnvName); home != "" {
		return home, nil
	}
	return "", fmt.Errorf("%s is not set: the local notification socket cannot be located", matrixhome.EnvName)
}
