package control

import (
	"log"

	"github.com/e-hua/netbula/internal/configs"
)

const (
	ControlConfigDirPath  = "."
	ControlConfigFileName = "control_config.json"
)

func setupConfig(newManagerAddress string, newToken string) *configs.ControlConfig {
	config, err := configs.GetConfigFromFile[configs.ControlConfig](ControlConfigDirPath, ControlConfigFileName)
	hasExistingConfig := (err == nil)

	if newManagerAddress == "" && newToken == "" {
		if !hasExistingConfig {
			log.Fatalln("Critical: No CLI arguments and no config file found.")
		}
		return config
	}

	if !hasExistingConfig {
		config = configs.NewControlConfig(newManagerAddress, newToken)
	} else {
		if newManagerAddress != "" {
			config.ManagerServerAddress = newManagerAddress
		}
		if newToken != "" {
			config.ControlToken = newToken
		}
	}

	configs.StoreConfigToFile(ControlConfigDirPath, ControlConfigFileName, config)
	return config
}

func Run(managerAddress string, token string) *configs.ControlConfig {
	cfg := setupConfig(managerAddress, token)

	return cfg
}
