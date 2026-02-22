/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package control

import (
	"fmt"
	"os"
	"text/tabwriter"

	ctl "github.com/e-hua/netbula/internal/app/control"
	"github.com/e-hua/netbula/internal/networks/types"
	"github.com/e-hua/netbula/internal/node"
	"github.com/spf13/cobra"
)

func printNodesInTable(nodes []*node.Node) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 5, ' ', tabwriter.TabIndent)
	fmt.Fprintln(w, "NAME\tWORKER ID\tMEMORY (GB)\tDISK (GB)\tCORES\tTASKS\t")

	for _, currNode := range(nodes) {
		fmt.Fprintf(
			w, 
			"%s\t%s\t%.2f\t%.2f\t%d\t%d\t\n", 
			currNode.Name,
			currNode.WorkerUuid.String(),
			float64(currNode.Memory) / float64(types.GigabyteInBytes),
			float64(currNode.Disk) / float64(types.GigabyteInBytes), 
			currNode.Cores,
			currNode.TaskCount,
		)
	}
	w.Flush()
}

// runCmd represents the run command
var NodesCmd = &cobra.Command{
	Use:   "nodes",
	Short: "Netbula Control node command",
	Long: `
	List and show all the nodes in the manager 
	`,
	Run: func(cmd *cobra.Command, args []string) {
		// Read the configs from the config file 
		storedConfigs := ctl.Run("", "")

		nodes := ctl.GetNodes(
			storedConfigs.ManagerServerAddress, 
			storedConfigs.ControlToken, 
		)

		printNodesInTable(nodes)
	},
}

func init() {
}
