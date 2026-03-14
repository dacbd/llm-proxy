package run

import (
	"log/slog"
	"os"

	"github.com/dacbd/llm-proxy/internal/config"
	"github.com/dacbd/llm-proxy/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var ServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Run HTTP server",
	Long:  `Run an HTTP server using Go's standard library`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadServerConfig()
		if err != nil {
			slog.Error("Failed to load config", "error", err)
			os.Exit(1)
		}
		srv := server.NewServer(cfg)

		if err := srv.Start(); err != nil {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	},
}

func init() {
	ServerCmd.Flags().Int("port", 11435, "Port to listen on")
	ServerCmd.Flags().String("ollama-url", "http://localhost:11434", "Upstream Ollama URL")
	ServerCmd.Flags().String("wandb-api-key", "", "W&B API key for Weave tracing (env: WANDB_API_KEY)")
	ServerCmd.Flags().String("wandb-project", "", "W&B project for Weave tracing, format: entity/project (env: WANDB_PROJECT)")

	viper.BindPFlag("port", ServerCmd.Flags().Lookup("port"))
	viper.BindPFlag("ollama-url", ServerCmd.Flags().Lookup("ollama-url"))
	viper.BindPFlag("wandb-api-key", ServerCmd.Flags().Lookup("wandb-api-key"))
	viper.BindPFlag("wandb-project", ServerCmd.Flags().Lookup("wandb-project"))
}
