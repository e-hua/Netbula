package configs

type WorkerConfig struct {
	WorkerName string  
	ManagerAddress string
	TlsToken string
}

func NewWorkerConfig(workerName string, managerAddress string, tlsToken string) *WorkerConfig {
	return &WorkerConfig{
		WorkerName: workerName,
		ManagerAddress: managerAddress,
		TlsToken: tlsToken,
	} 
}
