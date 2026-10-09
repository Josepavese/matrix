package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runclient"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/runapi"
)

type nativeSubmitRouter struct {
	middleware.SessionRouter
	received chan middleware.ConversationRequest
	release  chan struct{}
	once     sync.Once
}

func (r *nativeSubmitRouter) finish() { r.once.Do(func() { close(r.release) }) }

func (r *nativeSubmitRouter) RouteConversation(ctx context.Context, req middleware.ConversationRequest) (string, error) {
	r.received <- req
	select {
	case <-r.release:
		return "native-cli-finished", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (*nativeSubmitRouter) HandleSessionActionTyped(_ context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	return middleware.SessionActionResult{Action: req.Action}, nil
}

func nativeSubmitCLI(t *testing.T, bin, home string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "MATRIX_HOME="+home)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("CLI %v: %v", args, err)
	}
	return output
}

// The compiled CLI talks to the real handler, with provider work deliberately
// blocked until acceptance/replay have been verified. No model account is used.
func TestSmokePALRunSubmitCLI(t *testing.T) {
	bin := os.Getenv("MATRIX_PAL_BINARY")
	if bin == "" {
		t.Skip("set MATRIX_PAL_BINARY to the native build")
	}
	home, root := t.TempDir(), filepath.Join(t.TempDir(), "project with spaces")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	storage := memstore.New()
	if err := workspace.SaveMeta(storage, workspace.Meta{ID: "project", RootPath: root}); err != nil {
		t.Fatal(err)
	}
	router := &nativeSubmitRouter{received: make(chan middleware.ConversationRequest, 2), release: make(chan struct{})}
	t.Cleanup(router.finish)
	runtime := runapi.NewServer(router).WithTraceStorage(storage).WithAPIKey("native-cli-key")
	server := httptest.NewServer(http.HandlerFunc(runtime.HandleRuns))
	t.Cleanup(server.Close)
	if got := strings.TrimSpace(string(nativeSubmitCLI(t, bin, home, "home"))); got != home {
		t.Fatalf("home must print its path on stdout: %q", got)
	}
	nativeSubmitCLI(t, bin, home, "config", "set", "matrix_http_addr", strings.TrimPrefix(server.URL, "http://"))
	nativeSubmitCLI(t, bin, home, "config", "set", "matrix_api_key", "native-cli-key")
	args := []string{"run", "submit", "--agent", "specialist", "--model", "qualified/model", "--workspace", "project", "--channel", "native.cli", "--prompt", "review", "--idempotency-key", "native-review", "--json"}
	var first, replay runclient.Result
	if err := json.Unmarshal(nativeSubmitCLI(t, bin, home, args...), &first); err != nil {
		t.Fatal("stdout is not one JSON acceptance", err)
	}
	if first.RunID == "" || first.Status != runtrace.StatusRunning || first.Replayed {
		t.Fatalf("acceptance = %+v", first)
	}
	select {
	case req := <-router.received:
		if req.ChannelID != "native.cli" || req.WorkspaceID != "project" || req.WorkspacePath != root || req.AgentID != "specialist" || req.ModelID != "qualified/model" {
			t.Fatalf("CLI flags did not reach dispatch: %+v", req)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("accepted CLI task never dispatched")
	}
	if err := json.Unmarshal(nativeSubmitCLI(t, bin, home, args...), &replay); err != nil || !replay.Replayed || replay.RunID != first.RunID {
		t.Fatalf("CLI replay = %+v, %v", replay, err)
	}
	router.finish()
	store := runtrace.NewStore(storage)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, found, err := store.LoadRun(first.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if found && run.Status == runtrace.StatusCompleted && run.Output == "native-cli-finished" {
			t.Log("compiled CLI: async acceptance, explicit workspace/channel, JSON stdout, idempotent replay and eventual completion")
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("provider release did not complete the accepted run")
}
