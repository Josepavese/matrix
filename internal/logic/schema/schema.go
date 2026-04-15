package schema

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

const (
	CurrentVersion   = 1
	VersionKey       = "system.schema.version"
	UpdatedAtKey     = "system.schema.updated_at"
	InitializedAtKey = "system.schema.initialized_at"
)

type Report struct {
	CurrentVersion int    `json:"current_version"`
	StoredVersion  int    `json:"stored_version"`
	Status         string `json:"status"`
	InitializedAt  string `json:"initialized_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

func EnsureCurrent(storage middleware.Storage) (Report, error) {
	report, err := LoadReport(storage)
	if err != nil {
		return Report{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if report.StoredVersion == 0 {
		if err := setInt(storage, VersionKey, CurrentVersion); err != nil {
			return Report{}, err
		}
		if err := setString(storage, InitializedAtKey, now); err != nil {
			return Report{}, err
		}
		if err := setString(storage, UpdatedAtKey, now); err != nil {
			return Report{}, err
		}
		return Report{
			CurrentVersion: CurrentVersion,
			StoredVersion:  CurrentVersion,
			Status:         "initialized",
			InitializedAt:  now,
			UpdatedAt:      now,
		}, nil
	}

	if report.StoredVersion != CurrentVersion {
		if err := setInt(storage, VersionKey, CurrentVersion); err != nil {
			return Report{}, err
		}
		if err := setString(storage, UpdatedAtKey, now); err != nil {
			return Report{}, err
		}
		return Report{
			CurrentVersion: CurrentVersion,
			StoredVersion:  CurrentVersion,
			Status:         "migrated",
			InitializedAt:  report.InitializedAt,
			UpdatedAt:      now,
		}, nil
	}

	report.Status = "current"
	return report, nil
}

func LoadReport(storage middleware.Storage) (Report, error) {
	if storage == nil {
		return Report{}, fmt.Errorf("storage not available")
	}
	version, err := getInt(storage, VersionKey)
	if err != nil {
		return Report{}, err
	}
	initializedAt, err := getString(storage, InitializedAtKey)
	if err != nil {
		return Report{}, err
	}
	updatedAt, err := getString(storage, UpdatedAtKey)
	if err != nil {
		return Report{}, err
	}
	status := "uninitialized"
	if version == CurrentVersion && version != 0 {
		status = "current"
	} else if version > 0 {
		status = "outdated"
	}
	return Report{
		CurrentVersion: CurrentVersion,
		StoredVersion:  version,
		Status:         status,
		InitializedAt:  initializedAt,
		UpdatedAt:      updatedAt,
	}, nil
}

func getInt(storage middleware.Storage, key string) (int, error) {
	data, err := storage.Get(key)
	if err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return 0, nil
	}
	var value int
	if err := json.Unmarshal(data, &value); err != nil {
		return 0, fmt.Errorf("decode %s: %w", key, err)
	}
	return value, nil
}

func getString(storage middleware.Storage, key string) (string, error) {
	data, err := storage.Get(key)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return "", fmt.Errorf("decode %s: %w", key, err)
	}
	return value, nil
}

func setInt(storage middleware.Storage, key string, value int) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", key, err)
	}
	return storage.Set(key, data)
}

func setString(storage middleware.Storage, key, value string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", key, err)
	}
	return storage.Set(key, data)
}
