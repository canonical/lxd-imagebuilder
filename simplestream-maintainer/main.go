package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
)

var version = "0.0.1"

type globalOptions struct {
	flagTimeout   uint
	flagLogLevel  string
	flagLogFormat string

	ctx    context.Context
	cancel context.CancelFunc
}

// NewRootCmd initializes a CLI tool.
func NewRootCmd() *cobra.Command {
	o := globalOptions{}

	cmd := &cobra.Command{
		Use:              "simplestream-maintainer",
		Short:            "Simplestream server maintainer",
		Version:          version,
		SilenceUsage:     true,
		SilenceErrors:    true,
		PersistentPreRun: o.PreRun,
	}

	cmd.AddGroup(
		&cobra.Group{ID: "main", Title: "Commands:"},
		&cobra.Group{ID: "other", Title: "Other Commands:"},
	)

	cmd.SetCompletionCommandGroupID("other")
	cmd.SetHelpCommandGroupID("other")

	// Global flags.
	cmd.PersistentFlags().UintVar(&o.flagTimeout, "timeout", 0, "Timeout in seconds")
	cmd.PersistentFlags().StringVar(&o.flagLogLevel, "loglevel", "info", "Log level")
	cmd.PersistentFlags().StringVar(&o.flagLogFormat, "logformat", "text", "Log format")

	// Commands.
	buildOpts := buildOptions{global: &o}
	cmd.AddCommand(buildOpts.NewCommand())

	pruneOpts := pruneOptions{global: &o}
	cmd.AddCommand(pruneOpts.NewCommand())

	return cmd
}

func (o *globalOptions) PreRun(cmd *cobra.Command, args []string) {
	// Configure global context.
	if o.flagTimeout == 0 {
		o.ctx, o.cancel = context.WithCancel(context.Background())
	} else {
		o.ctx, o.cancel = context.WithTimeout(context.Background(), time.Duration(o.flagTimeout)*time.Second)
	}

	// Set signals that cancel the context.
	o.ctx, o.cancel = signal.NotifyContext(o.ctx, os.Interrupt)

	// Configure default logger.
	err := setDefaultLogger(o.flagLogLevel, o.flagLogFormat)
	if err != nil {
		// Error out, so we don't use the default logger.
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
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
