package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/providers/runapi"
)

// This opt-in harness uses the public run API of a real, already provisioned
// local runtime. It sends only synthetic images, with independent visual oracles.
func TestSmokeRealRichRunVision(t *testing.T) {
	endpoint := os.Getenv("MATRIX_REAL_RICH_RUN_URL")
	if endpoint == "" {
		t.Skip("requires an explicitly selected real Matrix runtime")
	}
	key, model := os.Getenv("MATRIX_REAL_RICH_RUN_KEY"), os.Getenv("MATRIX_REAL_RICH_RUN_MODEL")
	if key == "" || model == "" {
		t.Fatal("real runtime credential and explicit model are required")
	}
	agent := os.Getenv("MATRIX_REAL_RICH_RUN_AGENT")
	if agent == "" {
		t.Fatal("real agent must be explicit")
	}
	client := &http.Client{Timeout: 180 * time.Second}
	workspace := t.TempDir()
	for _, tc := range []struct {
		color       color.RGBA
		name, shape string
	}{
		{color.RGBA{R: 255, A: 255}, "red", "square"}, {color.RGBA{B: 255, A: 255}, "blue", "circle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := visionFixture(t, tc.color, tc.shape)
			request := map[string]any{
				"channel_id": "matrix.vision." + filepath.Base(workspace) + "." + tc.name,
				"agent_id":   agent, "model_id": model, "workspace_path": workspace,
				"input":          map[string]any{"text": "Inspect the attached synthetic picture. Reply with two English words: the dominant foreground color followed by the geometric shape. Do not use tools, files, or web access.", "content": []any{map[string]string{"type": "image", "mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(data)}}},
				"session_policy": "new_ephemeral_delete_after_run", "cleanup_policy": "delete_remote_or_forget_local", "emergency_kill_seconds": 120,
			}
			raw, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			status, result := realVisionRequest(t, client, endpoint+runapi.RunPathV1, key, raw)
			if status != http.StatusCreated || result["status"] != "completed" {
				t.Fatalf("real run failed: http=%d result=%v", status, result)
			}
			output, _ := result["output"].(string)
			lower := strings.ToLower(output)
			if !strings.Contains(lower, tc.name) || !strings.Contains(lower, tc.shape) {
				t.Fatalf("visual oracle mismatch: %q", output)
			}
			cleanup, ok := result["cleanup"].(map[string]any)
			if !ok || cleanup["clean"] != true || cleanup["strong_cleanup"] != true {
				t.Fatalf("cleanup not strong: %v", cleanup)
			}
			runID, _ := result["run_id"].(string)
			_, trace := realVisionRequest(t, client, endpoint+runapi.RunResourcePrefixV1+runID+"/trace", key, nil)
			run, ok := trace["run"].(map[string]any)
			if !ok || run["model_verification"] != "provider_confirmed" || run["effective_model"] != model || run["model_fallback_used"] == true {
				t.Fatalf("selected model not confirmed: %v", run)
			}
			if bytes.Contains(mustVisionJSON(t, trace), []byte(base64.StdEncoding.EncodeToString(data))) {
				t.Fatal("image bytes leaked into trace")
			}
			t.Logf("REAL_VISION_OK run_id=%s agent=%s model=%s output=%q image_sha256=%x provider_confirmed=true fallback=false strong_cleanup=true", runID, agent, model, output, sha256.Sum256(data))
		})
	}
}

func visionFixture(t *testing.T, c color.RGBA, shape string) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 240, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 240; x++ {
			inside := x >= 45 && x < 195 && y >= 45 && y < 195
			if shape == "circle" {
				inside = (x-120)*(x-120)+(y-120)*(y-120) < 75*75
			}
			pixel := color.RGBA{R: 255, G: 255, B: 255, A: 255}
			if inside {
				pixel = c
			}
			im.SetRGBA(x, y, pixel)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func realVisionRequest(t *testing.T, client *http.Client, url, key string, body []byte) (int, map[string]any) {
	t.Helper()
	method := http.MethodPost
	if body == nil {
		method = http.MethodGet
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Matrix-Key", key)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var value map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&value); err != nil {
		t.Fatal(fmt.Errorf("decode real runtime response: %w", err))
	}
	return response.StatusCode, value
}

func mustVisionJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
