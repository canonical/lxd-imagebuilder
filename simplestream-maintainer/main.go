package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "0.0.1"

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "simplestream-maintainer",
		Short:   "Simplestream server maintainer",
		Version: version,
	}

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	cmd.AddGroup(
		&cobra.Group{ID: "main", Title: "Commands:"},
		&cobra.Group{ID: "other", Title: "Other Commands:"},
	)

	cmd.SetCompletionCommandGroupID("other")
	cmd.SetHelpCommandGroupID("other")

	// Commands.
	cmd.AddCommand(NewBuildCmd())
	cmd.AddCommand(NewDiscardCmd())

	return cmd
}

func main() {
	err := NewRootCmd().Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
