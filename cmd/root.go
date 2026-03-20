/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "netbula",
	Short: "Yet another container orchestrator",
	Long: `
Netbula is a distributed container orchestration system designed for simplicity. 
It provides a robust platform for scheduling, deploying, 
and managing dockerized tasks across a cluster of worker nodes.

Key Components:
  netbula manager: Responsible for task scheduling, 
           state persistence, and monitoring worker health.
  netbula worker:  Connects to the manager, manages 
           local Docker containers, and reports task status.
  netbula control: The user interface. A CLI tool for developers to interact 
           with the manager API to run, stop, and monitor tasks and worker nodes.
	`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.netbula.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
