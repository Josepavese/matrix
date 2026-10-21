package runapi

import (
	"bytes"
	"encoding/json"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRichRunHTTPInteroperability(t *testing.T) {
	router := &richInputRouter{imageSupported: true}
	mux := http.NewServeMux()
	server := NewServer(router).WithAPIKey("synthetic-test-key")
	server.RegisterRoutes(mux)
	peer := httptest.NewServer(mux)
	defer peer.Close()
	body := imageInputBody(t, color.RGBA{G: 255, A: 255})
	for _, key := range []string{"wrong-key", "synthetic-test-key"} {
		request, err := http.NewRequest(http.MethodPost, peer.URL+RunPathV1, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Matrix-Key", key)
		response, err := peer.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if key == "wrong-key" {
			if response.StatusCode != http.StatusUnauthorized || router.lastConversation.ChannelID != "" {
				t.Fatal("unauthenticated dispatch", response.StatusCode)
			}
			continue
		}
		if response.StatusCode != http.StatusCreated || len(router.lastConversation.ContentBlocks) != 1 {
			t.Fatal(response.StatusCode, string(raw))
		}
		if bytes.Contains(raw, []byte(router.lastConversation.ContentBlocks[0].Data)) {
			t.Fatal("image leaked in HTTP result")
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded["status"] != "completed" {
			t.Fatal(decoded)
		}
	}
}
