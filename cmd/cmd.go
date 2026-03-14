package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cmd = &cobra.Command{
	Use:   "llm-proxy",
	Short: "",
	Long:  ``,
	PersistentPreRun: func(c *cobra.Command, args []string) {
		level := new(slog.LevelVar)
		if err := level.UnmarshalText([]byte(viper.GetString("log-level"))); err != nil {
			fmt.Fprintf(os.Stderr, "invalid log level %q, defaulting to info\n", viper.GetString("log-level"))
			level.Set(slog.LevelInfo)
		}

		var handler slog.Handler
		opts := &slog.HandlerOptions{Level: level}
		switch viper.GetString("log-format") {
		case "json":
			handler = slog.NewJSONHandler(os.Stdout, opts)
		case "text":
			handler = slog.NewTextHandler(os.Stdout, opts)
		default:
			fmt.Fprintf(os.Stderr, "invalid log format %q, expected text or json\n", viper.GetString("log-format"))
			fallthrough
		case "":
			if isInteractive() {
				handler = slog.NewTextHandler(os.Stdout, opts)
			} else {
				handler = slog.NewJSONHandler(os.Stdout, opts)
			}
		}
		slog.SetDefault(slog.New(handler))
	},
}

func Execute() {
	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	cmd.PersistentFlags().String("log-level", "info", "Log level: debug, info, warn, error")
	cmd.PersistentFlags().String("log-format", "", "Log format: text, json (default: text if interactive, json otherwise)")
	viper.BindPFlag("log-level", cmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("log-format", cmd.PersistentFlags().Lookup("log-format"))

	cmd.AddCommand(runCmd)
}

func isInteractive() bool {
	fileInfo, _ := os.Stdout.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
