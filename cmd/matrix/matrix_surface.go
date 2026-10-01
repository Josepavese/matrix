package main

import (
	"fmt"
	"os"

	"github.com/Josepavese/matrix/internal/logic/cmdutil"
	"github.com/Josepavese/matrix/internal/logic/matrixhome"
)

// This file carries no build tag on purpose. The two helpers below read the
// process configuration, not the local socket, and `matrix run submit` and
// `matrix runtime endpoints` call them on every platform. They used to live
// beside the socket client, which is linux||darwin only, so Windows lost two
// commands to a build tag: a cross-build caught what no local test could.
//
// The rule this file states: a helper sits in the file of the feature that owns
// it, and a build tag follows the platform dependency, not the neighbourhood.

// matrixSurfaceConfig reads the address and the credential of the Matrix HTTP
// surface. The local notification socket is served by that same server, so both
// the socket clients and the TCP client read the same pair here instead of each
// deciding which configuration key belongs to which surface.
func matrixSurfaceConfig() (string, string, error) {
	manager, cleanup, err := cmdutil.OpenReadOnlyConfigManager(DefaultVaultPath)
	if err != nil {
		return "", "", fmt.Errorf("read runtime configuration: %w", err)
	}
	defer cleanup()
	return manager.GetWithDefault("matrix_http_addr", ""), manager.GetWithDefault("matrix_api_key", ""), nil
}

// resolveActiveHome is the Matrix home whose socket this client talks to.
func resolveActiveHome() (string, error) {
	if activeMatrixHome != "" {
		return activeMatrixHome, nil
	}
	if home := os.Getenv(matrixhome.EnvName); home != "" {
		return home, nil
	}
	return "", fmt.Errorf("%s is not set: the local notification socket cannot be located", matrixhome.EnvName)
}
