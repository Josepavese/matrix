package runtimebroker

import (
	"os"
	"path/filepath"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

type Startup struct {
	State     string    `json:"state"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

func StartupPath(home string) string {
	return filepath.Join(home, "data", "runtime-broker.starting.json")
}

func WriteStartup(fs middleware.FS, path string, startedAt time.Time) error {
	const initializing = "initializing"
	return writeMetadata(fs, path, "startup marker", Startup{State: initializing, PID: os.Getpid(), StartedAt: startedAt.UTC()})
}
