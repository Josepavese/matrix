package setupstate

import (
	"encoding/json"
	"strings"
)

func Configured(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	var configured bool
	if json.Unmarshal(data, &configured) == nil {
		return configured
	}
	var configuredString string
	if json.Unmarshal(data, &configuredString) == nil {
		return strings.EqualFold(strings.TrimSpace(configuredString), "true")
	}
	return false
}
