package configs

type ControlConfig struct {
	ManagerServerAddress string
	// The actual Token in the manager
	ControlToken           string
	ManagerCertFingerprint string
}

func NewControlConfig(managerAddress string, token string, managerCertFingerprint string) *ControlConfig {
	return &ControlConfig{
		ManagerServerAddress:   managerAddress,
		ControlToken:           token,
		ManagerCertFingerprint: managerCertFingerprint,
	}
}
