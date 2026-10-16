package cli

import (
	"errors"
	"fmt"

	"github.com/RakhaYandra/hibiki/internal/diagnostics"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/tui"
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
		newSourcesCmd(),
		newDoctorCmd(),
		newTuiCmd(),
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
		return commandCode(err)
	}
	return exit.CodeOK
}

// The one place an error becomes an exit code. The TUI owns two codes the rest
// of the command tree does not use: 2 when the terminal cannot run it, and 130
// when a person interrupted it.
func commandCode(err error) int {
	switch {
	case err == nil:
		return exit.CodeOK
	case errors.Is(err, tui.ErrInterrupted):
		return interruptedCode
	case errors.Is(err, diagnostics.ErrNotATerminal):
		return exit.CodeUsage
	}
	return exit.FromError(err)
}
