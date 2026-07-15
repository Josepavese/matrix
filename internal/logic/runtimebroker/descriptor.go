package runtimebroker

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

const Version = 1

type Descriptor struct {
	Version     int       `json:"version"`
	PID         int       `json:"pid"`
	JSONRPCAddr string    `json:"jsonrpc_addr"`
	Token       string    `json:"token"`
	LogFile     string    `json:"log_file"`
	StartedAt   time.Time `json:"started_at"`
}

type StorageArgs struct {
	Key    string `json:"key,omitempty"`
	Prefix string `json:"prefix,omitempty"`
	Value  []byte `json:"value,omitempty"`
	Token  string `json:"token"`
}

type StorageReply struct {
	Value []byte   `json:"value,omitempty"`
	Keys  []string `json:"keys,omitempty"`
}

func Path(home string) string {
	return filepath.Join(home, "data", "runtime-broker.json")
}

func New(addr, logFile string) (Descriptor, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return Descriptor{}, fmt.Errorf("generate runtime broker token: %w", err)
	}
	if logFile != "" && !filepath.IsAbs(logFile) {
		absolute, err := filepath.Abs(logFile)
		if err != nil {
			return Descriptor{}, fmt.Errorf("resolve runtime log path: %w", err)
		}
		logFile = absolute
	}
	return Descriptor{
		Version: Version, PID: os.Getpid(), JSONRPCAddr: addr,
		Token: hex.EncodeToString(secret), LogFile: logFile, StartedAt: time.Now().UTC(),
	}, nil
}

func Write(fs middleware.FS, path string, descriptor Descriptor) error {
	data, err := json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		return fmt.Errorf("encode runtime broker descriptor: %w", err)
	}
	temporary := path + ".tmp"
	file, err := fs.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create runtime broker descriptor: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = fs.Remove(temporary)
		return fmt.Errorf("write runtime broker descriptor: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = fs.Remove(temporary)
		return fmt.Errorf("close runtime broker descriptor: %w", err)
	}
	if err := fs.Rename(temporary, path); err != nil {
		_ = fs.Remove(temporary)
		return fmt.Errorf("activate runtime broker descriptor: %w", err)
	}
	return nil
}

func Read(fs middleware.FS, path string) (Descriptor, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return Descriptor{}, err
	}
	var descriptor Descriptor
	if err := json.Unmarshal(data, &descriptor); err != nil {
		return Descriptor{}, fmt.Errorf("decode runtime broker descriptor: %w", err)
	}
	if descriptor.Version != Version || descriptor.JSONRPCAddr == "" || descriptor.Token == "" {
		return Descriptor{}, fmt.Errorf("invalid runtime broker descriptor")
	}
	return descriptor, nil
}

func Remove(fs middleware.FS, path string) error {
	err := fs.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
