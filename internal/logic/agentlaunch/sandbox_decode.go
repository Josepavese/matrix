package agentlaunch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Duplicate keys can silently replace a stronger request with a weaker one.
// Check object keys before typed decoding; scalar/type/unknown-field checks stay
// in the strict decoder. The declaration has no array-valued fields.
func uniqueSandboxObject(raw []byte, depth int) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil
	}
	if depth > 8 {
		return fmt.Errorf("sandbox declaration nesting exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("invalid sandbox object")
	}
	seen := map[string]bool{}
	for decoder.More() {
		if err := uniqueSandboxField(decoder, seen, depth); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("invalid sandbox object")
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		return fmt.Errorf("invalid sandbox trailing data")
	}
	return nil
}

func uniqueSandboxField(decoder *json.Decoder, seen map[string]bool, depth int) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("invalid sandbox key")
	}
	key, ok := token.(string)
	if !ok || seen[key] {
		return fmt.Errorf("duplicate sandbox key")
	}
	seen[key] = true
	var value json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("invalid sandbox value")
	}
	return uniqueSandboxObject(value, depth+1)
}
