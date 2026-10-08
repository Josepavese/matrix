package otlplog

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

type scopeLogs struct {
	Scope   map[string]string `json:"scope"`
	Records []record          `json:"logRecords"`
}
type resourceLogs struct {
	Resource map[string][]attribute `json:"resource"`
	Scopes   []scopeLogs            `json:"scopeLogs"`
}
type payload struct {
	Resources []resourceLogs `json:"resourceLogs"`
}

func (e *Exporter) send(batch []record) {
	data := payload{Resources: []resourceLogs{{Resource: map[string][]attribute{"attributes": {{Key: "service.name", Value: value{String: "matrix"}}}}, Scopes: []scopeLogs{{Scope: map[string]string{"name": "matrix.operational"}, Records: batch}}}}}
	body, err := json.Marshal(data)
	if err != nil {
		e.fail("collector_encode_failed", len(batch))
		return
	}
	req, err := http.NewRequestWithContext(e.ctx, http.MethodPost, e.options.Endpoint, bytes.NewReader(body))
	if err != nil {
		e.fail("collector_request_failed", len(batch))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if e.authorization != "" {
		req.Header.Set("Authorization", e.authorization)
	}
	response, err := e.client.Do(req)
	if err != nil {
		e.fail("collector_transport_failed", len(batch))
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		e.fail("collector_http_"+strconv.Itoa(response.StatusCode), len(batch))
		return
	}
	rejected, ok := readRejections(response.Body, len(batch))
	if !ok {
		e.fail("collector_ack_invalid", len(batch))
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stats.Acknowledged += uint64(len(batch) - rejected)
	e.stats.Dropped += uint64(rejected)
	if rejected > 0 {
		e.stats.FailedBatches++
		e.stats.LastFailure = "collector_partial_rejection"
	} else {
		e.stats.LastFailure = ""
	}
}

func readRejections(body io.Reader, count int) (int, bool) {
	data, err := io.ReadAll(io.LimitReader(body, 4097))
	if err != nil || len(data) > 4096 {
		return 0, false
	}
	var result struct {
		Partial struct {
			Rejected json.RawMessage `json:"rejectedLogRecords"`
		} `json:"partialSuccess"`
	}
	if json.Unmarshal(data, &result) != nil {
		return 0, false
	}
	if len(result.Partial.Rejected) == 0 {
		return 0, true
	}
	var encoded string
	if json.Unmarshal(result.Partial.Rejected, &encoded) != nil {
		encoded = string(result.Partial.Rejected)
	}
	rejected, err := strconv.ParseUint(encoded, 10, 64)
	if err != nil || rejected > uint64(count) {
		return 0, false
	}
	return int(rejected), true
}

func (e *Exporter) fail(code string, count int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stats.FailedBatches++
	e.stats.Dropped += uint64(count)
	e.stats.LastFailure = code
}
