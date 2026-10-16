package cli

import (
	"fmt"

	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "hibiki",
		Short:         "Reconstruct a Linux system event timeline from the systemd journal",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newTimelineCmd(),
		newEventsCmd(),
		newInspectCmd(),
		newVersionCmd(),
	)
	return root
}

func Execute() int {
	return run(NewRootCmd())
}

func run(root *cobra.Command) int {
	if err := root.Execute(); err != nil {
		fmt.Fprintln(root.ErrOrStderr(), "hibiki:", err)
		return exit.FromError(err)
	}
	return exit.CodeOK
}
