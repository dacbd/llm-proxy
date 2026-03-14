package cmd

import (
	"github.com/dacbd/llm-proxy/cmd/run"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run various services",
	Long:  `Run various services like HTTP servers`,
}

func init() {
	runCmd.AddCommand(run.ServerCmd)
}
