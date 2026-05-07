/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package control

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/docker/go-units"
	ctl "github.com/e-hua/netbula/internal/app/control"
	"github.com/e-hua/netbula/internal/task"

	"github.com/spf13/cobra"
)

func printTasksInTable(tasks []*task.Task) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 5, ' ', tabwriter.TabIndent)
	fmt.Fprintln(w, "ID\tNAME\tCREATED\tSTATE\tCONTAINER NAME\tIMAGE\t")
	for _, task := range tasks {
		var startTime string
		if task.StartTime.IsZero() {
			startTime = fmt.Sprintf(
				"%s ago",
				units.HumanDuration(time.Now().UTC().Sub(time.Now().UTC())),
			)
		} else {
			startTime = fmt.Sprintf(
				"%s ago",
				units.HumanDuration(time.Now().UTC().Sub(task.StartTime)),
			)
		}

		taskState := task.State.String()
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t\n",
			task.ID,
			task.Name,
			startTime,
			taskState,
			task.Name,
			task.Image,
		)
	}
	w.Flush()
}

// tasksCmd represents the tasks command
var TasksCmd = &cobra.Command{
	Use:   "tasks",
	Short: "Nebula Control tasks command",
	Long: `
	List and show all the tasks in the manager 
	`,
	Run: func(cmd *cobra.Command, args []string) {
		// Read the configs from the config file
		storedConfigs := ctl.Run("", "", "")
		tasks := ctl.GetTasks(
			storedConfigs.ManagerServerAddress,
			storedConfigs.ControlToken,
			storedConfigs.ManagerCertFingerprint,
		)
		printTasksInTable(tasks)
	},
}

func init() {
}
