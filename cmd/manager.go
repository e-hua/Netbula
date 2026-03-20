/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/e-hua/netbula/internal/app/manager"
	"github.com/spf13/cobra"
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

		manager.Run([2]int{workerPort, apiPort})
	},
}

func init() {
	rootCmd.AddCommand(managerCmd)

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
