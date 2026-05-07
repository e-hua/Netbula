/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/e-hua/netbula/internal/app/manager"
	"github.com/e-hua/netbula/internal/logger"
	"github.com/e-hua/netbula/internal/scheduler"
	"github.com/spf13/cobra"
)

const (
	ManagerConfigDirPath  = "."
	ManagerConfigFileName = "manager_config.json"

	ManagerDbPath                 = "manager.db"
	ManagerDbFileMode os.FileMode = 0600
)

// managerCmd represents the manager command
var managerCmd = &cobra.Command{
	Use:   "manager",
	Short: "Manager command to run a Netbula manager orchestrator",
	Long: `Netbula manager commmand 
	The manager controls the orchestration system and is responsible for:
	- Accepting tasks from users
	- Scheduling tasks onto worker nodes
	- Periodically polling workers to get task updates and worker machine loads 
	`,
	Run: func(cmd *cobra.Command, args []string) {
		workerPort, _ := cmd.Flags().GetInt("worker-port")
		apiPort, _ := cmd.Flags().GetInt("api-port")
		verbose, _ := cmd.Flags().GetBool("verbose")

		// Load configurations
		cfg, err := manager.SetupConfig(
			ManagerConfigDirPath,
			ManagerConfigFileName,
			workerPort,
			apiPort,
		)
		if err != nil {
			logger.TerminateApplication(
				"Failed to setup configurations for the manager application",
				err,
			)
		}

		// Open sockets for listening
		workerListener, err := manager.CreateTcpListener(cfg.WorkerConnectionPort)
		if err != nil {
			logger.TerminateApplication(
				"Failed to open socket for workers to connect to",
				err,
			)
		}
		ctlListener, err := manager.CreateTcpListener(cfg.ServerApiPort)
		if err != nil {
			logger.TerminateApplication(
				"Failed to open socket for the control application to connect to",
				err,
			)
		}

		// Building the application
		appConfigs := manager.AppConfigs{
			LogDest:                  os.Stderr,
			Scheduler:                &scheduler.Epvm{},
			AllowVerbose:             verbose,
			IsPersistent:             true,
			ManagerDbPath:            ManagerDbPath,
			ManagerDbFileMode:        ManagerDbFileMode,
			ManagerConfigs:           *cfg,
			WorkerConnectionListener: workerListener,
			ManagerServerApiListener: ctlListener,
		}
		app, err := manager.NewApp(appConfigs)
		if err != nil {
			logger.TerminateApplication(
				"Failed to create application",
				err,
			)
		}

		// Printing to stdout for normal users to see
		log.Printf("Manager API (Secure) for control program to connect to listening on %d\n", cfg.ServerApiPort)
		log.Printf("Manager API (Secure) for workers to connect to listening on %d\n", cfg.ServerApiPort)

		fmt.Printf("Connection token(auth-token): %v (Enter this when registering control program)\n", cfg.AuthToken)
		fmt.Printf("Tls certificate fingerprint(cert-fingerprint): %v (Enter this when registering workers or control programs)\n", cfg.CertFingerprint)

		app.Run(context.Background())
	},
}

func init() {
	rootCmd.AddCommand(managerCmd)
	managerCmd.Flags().BoolP(
		"verbose",
		"v",
		false,
		"Enable verbose mode, which uses JSON instead of whitespace-separated key-value pairs for logging",
	)

	// Yamux/TLS connection
	managerCmd.Flags().IntP(
		"worker-port",
		"w",
		0,
		"Port for the worker nodes to connect to (Example: 1111)",
	)

	// User connection
	managerCmd.Flags().IntP(
		"api-port",
		"a",
		0,
		"Port for the user client API (Example: 2222)",
	)
}
