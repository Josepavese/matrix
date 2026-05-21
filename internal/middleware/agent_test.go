package middleware

import (
	"encoding/json"
	"testing"
)

func TestAuthEnvVarDefaultsSecretToTrue(t *testing.T) {
	var variable AuthEnvVar
	if err := json.Unmarshal([]byte(`{"name":"API_KEY"}`), &variable); err != nil {
		t.Fatalf("unmarshal env var: %v", err)
	}
	if !variable.Secret {
		t.Fatalf("secret should default to true")
	}

	if err := json.Unmarshal([]byte(`{"name":"PUBLIC_TOKEN","secret":false}`), &variable); err != nil {
		t.Fatalf("unmarshal explicit non-secret env var: %v", err)
	}
	if variable.Secret {
		t.Fatalf("explicit secret=false should be preserved")
	}
}
