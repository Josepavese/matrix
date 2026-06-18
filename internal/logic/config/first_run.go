package config

import (
	"errors"
	"os"

	"github.com/Josepavese/matrix/internal/middleware"
)

const defaultTelegramSeedConfig = `{"enabled":false,"admins":[]}`

type firstRunConfigReader struct {
	delegate middleware.ConfigReader
}

func NewFirstRunConfigReader(delegate middleware.ConfigReader) middleware.ConfigReader {
	return firstRunConfigReader{delegate: delegate}
}

func (r firstRunConfigReader) ReadConfig(path string) ([]byte, error) {
	data, err := r.delegate.ReadConfig(path)
	if err != nil && path == "configs/telegram.json" && errors.Is(err, os.ErrNotExist) {
		return []byte(defaultTelegramSeedConfig), nil
	}
	return data, err
}
