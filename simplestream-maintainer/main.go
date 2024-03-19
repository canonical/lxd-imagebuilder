package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var version = "0.0.1"

func NewRootCmd() *cobra.Command {
	var flagLogLevel string
	var flagLogFormat string

	cmd := &cobra.Command{
		Use:     "simplestream-maintainer",
		Short:   "Simplestream server maintainer",
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			err := setDefaultLogger(flagLogLevel, flagLogFormat)
			if err != nil {
				// Error out, so we don't use the default logger.
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	cmd.AddGroup(
		&cobra.Group{ID: "main", Title: "Commands:"},
		&cobra.Group{ID: "other", Title: "Other Commands:"},
	)

	cmd.SetCompletionCommandGroupID("other")
	cmd.SetHelpCommandGroupID("other")

	// Global flags.
	cmd.PersistentFlags().StringVar(&flagLogLevel, "loglevel", "info", "Log level")
	cmd.PersistentFlags().StringVar(&flagLogFormat, "logformat", "text", "Log format")

	// Commands.
	cmd.AddCommand(NewBuildCmd())
	cmd.AddCommand(NewDiscardCmd())

	return cmd
}

func setDefaultLogger(level string, format string) error {
	opts := slog.HandlerOptions{}

	switch level {
	case "debug":
		opts.Level = slog.LevelDebug
	case "info":
		opts.Level = slog.LevelInfo
	case "warn":
		opts.Level = slog.LevelWarn
	case "error":
		opts.Level = slog.LevelError
	default:
		return fmt.Errorf("Invalid log level %q. Valid log levels are: [debug, info, warn, error]", level)
	}

	var handler slog.Handler

	switch format {
	case "text":
		handler = slog.NewTextHandler(os.Stderr, &opts)
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, &opts)
	default:
		return fmt.Errorf("Invalid log format %q. Valid log formats are: [text, json]", format)
	}

	slog.SetDefault(slog.New(handler))
	return nil
}

func main() {
	err := NewRootCmd().Execute()
	if err != nil {
		slog.Error(fmt.Sprintf("%v", err))
		os.Exit(1)
	}
}
