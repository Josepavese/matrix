package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/Josepavese/matrix/internal/logic/config"
)

// ensureLocalAPIKeys closes the default unauthenticated loopback ingress. The
// explicit development opt-out is restricted to loopback by the bind preflight.
func ensureLocalAPIKeys(cfg *config.Manager) error {
	if os.Getenv("MATRIX_LOCAL_UNAUTHENTICATED") == "1" {
		return nil
	}
	for _, name := range []string{"matrix_api_key", "daemon_api_key"} {
		value, err := cfg.Get(name)
		if err != nil {
			return err
		}
		if value != "" {
			continue
		}
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return fmt.Errorf("generate %s: %w", name, err)
		}
		if err := cfg.Set(name, hex.EncodeToString(bytes)); err != nil {
			return err
		}
	}
	return nil
}

func registryURLFromEnvironment() string {
	url, _ := configuredAgentRegistryURL() // NewDaemonContext already validated it.
	return url
}
