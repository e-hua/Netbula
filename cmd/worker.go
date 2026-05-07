/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/spf13/cobra"
)

// workerCmd represents the worker command
var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Worker command to run a Netbula worker node",
	Long: `Netbula worker commmand 
	The worker instance connects to the manager's open port 
	and runs container tasks sent from the manager

	Load from the config files and DB storage if not enough flags are provided. 
	`,
	Run: func(cmd *cobra.Command, args []string) {
		managerAddr, _ := cmd.Flags().GetString("manager")
		fingerprint, _ := cmd.Flags().GetString("fingerprint")
		name, _ := cmd.Flags().GetString("name")

		worker.Run(managerAddr, fingerprint, name)
	},
}

func init() {
	rootCmd.AddCommand(workerCmd)

	workerCmd.Flags().StringP(
		"manager",
		"M",
		"",
		"The TCP address of manager worker's connecting to (Example: <manager_ip>:1111)",
	)
	workerCmd.Flags().StringP(
		"fingerprint",
		"f",
		"",
		"Hexadecimal encoded hash of the manager's TLS certificate (Example: 0123456789abcde)",
	)
	workerCmd.Flags().StringP(
		"name",
		"n",
		"",
		"Name of the worker (Example: Ubuntu Server 1)",
	)
}
