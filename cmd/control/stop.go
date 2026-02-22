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
