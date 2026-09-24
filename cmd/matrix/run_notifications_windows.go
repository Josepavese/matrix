//go:build windows

package main

import (
	"context"
	"github.com/Josepavese/matrix/internal/providers/matrixapi"
)

func startLocalNotificationServer(_ context.Context, _ string, _ *matrixapi.Server) error { return nil }
