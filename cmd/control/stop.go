/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package control

import (
	ctl "github.com/e-hua/netbula/internal/app/control"
	"github.com/spf13/cobra"
)

// stopCmd represents the stop command
var StopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Netbula Control stop command",
	Long: `
	The stop command that stops and removes a running task

	Need to append the UUID of the task to stop.
	For example: 
	netbula control stop 21b23589-5d2d-4731-b5c9-a97e9832d021
	`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Read the configs from the config file
		storedConfigs := ctl.Run("", "")

		ctl.StopTask(
			storedConfigs.ManagerServerAddress,
			storedConfigs.ControlToken,
			args[0],
		)
	},
}

func init() {
}
