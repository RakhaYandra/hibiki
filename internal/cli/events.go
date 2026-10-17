package cli

import "github.com/spf13/cobra"

func newEventsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "List system events with filters",
		Long: "List system events with the same filters as timeline.\n\n" +
			"--category is the official flag; --type is an accepted alias and both\n" +
			"must agree when they are given together.",
		Args: noArgs,
	}
	flags := addFilterFlags(cmd)
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		_, err := runQuery(c, flags)
		return err
	}
	return cmd
}
