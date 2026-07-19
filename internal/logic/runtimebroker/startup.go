package runtimebroker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

type Startup struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

func StartupPath(home string) string {
	return filepath.Join(home, "data", "runtime-broker.starting.json")
}

func ClaimStartup(fs middleware.FS, path string, startedAt time.Time, maxAge time.Duration) error {
	file, err := fs.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		fresh, statErr := StartupFresh(fs, path, startedAt, maxAge)
		if statErr != nil {
			return statErr
		}
		if fresh {
			return fmt.Errorf("runtime broker startup is already in progress")
		}
		if err := RemoveStartup(fs, path); err != nil {
			return fmt.Errorf("remove stale runtime broker startup marker: %w", err)
		}
		file, err = fs.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	}
	if err != nil {
		return fmt.Errorf("claim runtime broker startup: %w", err)
	}
	data, err := json.Marshal(Startup{PID: os.Getpid(), StartedAt: startedAt.UTC()})
	if err != nil {
		_ = file.Close()
		_ = fs.Remove(path)
		return fmt.Errorf("encode runtime broker startup marker: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = fs.Remove(path)
		return fmt.Errorf("write runtime broker startup marker: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = fs.Remove(path)
		return fmt.Errorf("close runtime broker startup marker: %w", err)
	}
	return nil
}

func StartupFresh(fs middleware.FS, path string, now time.Time, maxAge time.Duration) (bool, error) {
	info, err := fs.Stat(path)
	if err != nil {
		return false, err
	}
	age := now.Sub(info.ModTime())
	return age >= 0 && age <= maxAge, nil
}

func RemoveStartup(fs middleware.FS, path string) error {
	return Remove(fs, path)
}
