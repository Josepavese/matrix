//go:build windows

package workspacegrant

import "fmt"

func requireOwnedDirectory(_ string) error {
	return fmt.Errorf("workspace grants require filesystem ownership verification, unavailable on Windows")
}
