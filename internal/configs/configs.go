package configs

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
)

const (
	standardPermission  = 0755 // (drwxr-xr-x)
	readWritePermission = 0644 // (rw-r--r--)
)

func StoreConfigToFile[T any](dirPath string, fileName string, config *T) error {
	// Encode the config struct in JSON
	encodedBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	// Make directories recursively
	err = os.MkdirAll(dirPath, standardPermission)
	if err != nil {
		return fmt.Errorf("failed to create directory recursively on the path to config file(%s): %w", dirPath, err)
	}

	// Create the file if not exist, then overwrite the file
	err = os.WriteFile(path.Join(dirPath, fileName), encodedBytes, readWritePermission)
	if err != nil {
		return fmt.Errorf("failed to create file %s and writing to it: %w", path.Join(dirPath, fileName), err)
	}

	return nil
}

func GetConfigFromFile[T any](dirPath string, fileName string) (*T, error) {
	filePath := path.Join(dirPath, fileName)

	dataRead, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read data from file [%s]: %w", filePath, err)
	}

	var workerConfig T
	err = json.Unmarshal(dataRead, &workerConfig)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal data read from file [filePath: %s] [data: %s]: %w",
			filePath,
			string(dataRead),
			err,
		)
	}

	return &workerConfig, nil
}
