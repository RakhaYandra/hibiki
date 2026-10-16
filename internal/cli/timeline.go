package cli

import "github.com/spf13/cobra"

func newTimelineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Show system events in chronological order",
		Long: "Show the systemd journal as one chronological timeline.\n\n" +
			"Every event carries its source and the journal fields it came from, so a\n" +
			"finding can be verified. Order is evidence of sequence, never of cause.",
		Args: cobra.NoArgs,
	}
	flags := addFilterFlags(cmd)
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		_, err := runQuery(c, flags)
		return err
	}
	return cmd
}
