package control

import (
	"log"

	"github.com/e-hua/netbula/internal/configs"
)

const (
	ControlConfigDirPath  = "."
	ControlConfigFileName = "control_config.json"
)

func setupConfig(newManagerAddress string, newToken string, newCertFingerprint string) *configs.ControlConfig {
	config, err := configs.GetConfigFromFile[configs.ControlConfig](ControlConfigDirPath, ControlConfigFileName)
	hasExistingConfig := (err == nil)

	// All flags empty, no configs to update
	if newManagerAddress == "" && newToken == "" && newCertFingerprint == "" {
		if !hasExistingConfig {
			log.Fatalln("Critical: No CLI arguments and no config file found.")
		}
		return config
	}

	if !hasExistingConfig {
		config = configs.NewControlConfig(newManagerAddress, newToken, newCertFingerprint)
	} else {
		if newManagerAddress != "" {
			config.ManagerServerAddress = newManagerAddress
		}
		if newToken != "" {
			config.ControlToken = newToken
		}
		if newCertFingerprint != "" {
			config.ManagerCertFingerprint = newCertFingerprint
		}
	}

	configs.StoreConfigToFile(ControlConfigDirPath, ControlConfigFileName, config)
	return config
}

func Run(managerAddress string, token string, certFingerprint string) *configs.ControlConfig {
	cfg := setupConfig(managerAddress, token, certFingerprint)

	return cfg
}
