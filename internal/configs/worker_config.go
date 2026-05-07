package configs

import "github.com/google/uuid"

type WorkerConfig struct {
	WorkerName             string
	Uuid                   uuid.UUID
	ManagerAddress         string
	ManagerCertFingerprint string
}

func NewWorkerConfig(uuid uuid.UUID, workerName string, managerAddress string, managerCertFingerprint string) *WorkerConfig {
	return &WorkerConfig{
		Uuid:                   uuid,
		WorkerName:             workerName,
		ManagerAddress:         managerAddress,
		ManagerCertFingerprint: managerCertFingerprint,
	}
}
