/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package control

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	ctl "github.com/e-hua/netbula/internal/app/control"
	"github.com/spf13/cobra"
)

func fileExists(fileName string) bool {
	_, err := os.Stat(fileName)
	return !errors.Is(err, fs.ErrNotExist) 
}

// runCmd represents the run command
var RunCmd = &cobra.Command{
	Use:   "run",
	Short: "Netbula Control run command",
	Long: `
	The run command that starts a new task
	`,
	Run: func(cmd *cobra.Command, args []string) {
		fileName, _ := cmd.Flags().GetString("filename")
		fullFileName, err := filepath.Abs(fileName)
		if (err != nil) {
			log.Fatal(err)
		}
		if (!fileExists(fullFileName)) {
			log.Fatalf("File %s does not exist.", fileName)
		}

		data, err := os.ReadFile(fileName)
		if (err != nil) {
			log.Fatalf("Error reading file: %v\n", err)
		}

		// Read the configs from the config file 
		storedConfigs := ctl.Run("", "")

		ctl.StartTask(
			storedConfigs.ManagerServerAddress, 
			storedConfigs.ControlToken, 
			data,
		)
	},
}

func init() {
	RunCmd.PersistentFlags().StringP(
		"filename", 
		"f", 
		"task.json", 
		"Task specification file",
	)
}
