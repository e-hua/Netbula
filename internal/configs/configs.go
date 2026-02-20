package configs

import (
	"encoding/json"
	"log"
	"os"
	"path"
)

const (
	standardPermission = 0755 // (drwxr-xr-x)
	readWritePermission = 0644 // (rw-r--r--)
)

func StoreConfigToFile[T any](dirPath string, fileName string, config *T) error {
	// Encode the config struct in JSON 
	encodedBytes, err :=  json.MarshalIndent(config, "", "  ")
	if (err != nil) {
		log.Printf("Error unmarshalling config file: %v", err)
		return err
	}

	// Make directories recursively 
	err = os.MkdirAll(dirPath, standardPermission)
	if (err != nil) {
		log.Printf("Error creating directory recursively on the path to config file: %v", err)
		return err 
	}

	// Create the file if not exist, then overwrite the file
	err = os.WriteFile(path.Join(dirPath, fileName), encodedBytes, readWritePermission)
	if (err != nil) {
		log.Printf("Error creating file and writing to it: %v", err)
		return err 
	}

	return nil
}

func GetConfigFromFile[T any](dirPath string, fileName string) (*T, error) {
	filePath := path.Join(dirPath, fileName)

	dataRead, err := os.ReadFile(filePath)
	if (err != nil) {
		log.Printf("Error reading file: %v", err)
		return nil, err
	}

	var workerConfig T
	err = json.Unmarshal(dataRead, &workerConfig)
	if (err != nil) {
		log.Printf("Error decoding file: %v", err)
		return nil, err
	}

	return &workerConfig, nil
}