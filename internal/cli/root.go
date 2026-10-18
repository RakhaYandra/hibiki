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
	root.Args = noArgs
	// The root command is runnable only so ValidateArgs runs: without it cobra
	// stops at ErrHelp for a non-runnable root and an unknown subcommand would
	// exit 0 behind a help screen.
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return exit.Usage("%s", err)
	})
	root.AddCommand(
		newTimelineCmd(),
		newEventsCmd(),
		newInspectCmd(),
		newIncidentsCmd(),
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

// noArgs and exactArgs mirror cobra's own validators, messages included, so a
// bad argument leaves as exit 2 (PRD 9.7) instead of the untyped error code 1.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return exit.Usage("unknown command %q for %q", args[0], cmd.CommandPath())
	}
	return nil
}

func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return exit.Usage("accepts %d arg(s), received %d", n, len(args))
		}
		return nil
	}
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
