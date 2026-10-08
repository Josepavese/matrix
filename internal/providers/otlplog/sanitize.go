package otlplog

import (
	"encoding/json"
	"regexp"
	"strconv"
	"time"

	"github.com/Josepavese/matrix/internal/logic/providerdiag"
)

type value struct {
	String string `json:"stringValue,omitempty"`
	Int    string `json:"intValue,omitempty"`
}
type attribute struct {
	Key   string `json:"key"`
	Value value  `json:"value"`
}
type record struct {
	Time       string      `json:"timeUnixNano"`
	Severity   int         `json:"severityNumber"`
	Body       value       `json:"body"`
	Attributes []attribute `json:"attributes,omitempty"`
}

var safeLabel = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,128}$`)

func sanitize(raw []byte) (record, bool) {
	if len(raw) > 64<<10 {
		return record{}, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return record{}, false
	}
	event := label(fields["event"])
	if event == "" {
		return record{}, false
	}
	r := record{Time: strconv.FormatInt(time.Now().UnixNano(), 10), Severity: 9, Body: value{String: event}}
	for _, key := range []string{"component", "event", "agent", "protocol_kind", "failure_code", "status", "run_id", "logical_session"} {
		if s := label(fields[key]); s != "" {
			r.Attributes = append(r.Attributes, attribute{Key: key, Value: value{String: s}})
		}
	}
	r.Attributes = append(r.Attributes, numericAttributes(fields)...)
	var level string
	_ = json.Unmarshal(fields["level"], &level)
	switch level {
	case "DEBUG":
		r.Severity = 5
	case "WARN":
		r.Severity = 13
	case "ERROR":
		r.Severity = 17
	}
	return r, true
}

func numericAttributes(fields map[string]json.RawMessage) []attribute {
	var attrs []attribute
	for _, key := range []string{"duration_ms", "count", "tool_calls", "disk_free_bytes", "active", "queue_depth"} {
		if n, err := strconv.ParseInt(string(fields[key]), 10, 64); err == nil && n >= 0 {
			attrs = append(attrs, attribute{Key: key, Value: value{Int: strconv.FormatInt(n, 10)}})
		}
	}
	return attrs
}

func label(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil || !safeLabel.MatchString(s) || providerdiag.Redact(s) != s {
		return ""
	}
	return s
}
