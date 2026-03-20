package configs

type ControlConfig struct {
	ManagerServerAddress string
	// The actual Token in the manager
	ControlToken string
}

func NewControlConfig(managerAddress string, token string) *ControlConfig {
	return &ControlConfig{
		ManagerServerAddress: managerAddress,
		ControlToken:         token,
	}
}
