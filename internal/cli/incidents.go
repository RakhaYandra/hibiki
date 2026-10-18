package cli

import (
	"time"

	"github.com/RakhaYandra/hibiki/internal/correlation"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/output"
	"github.com/spf13/cobra"
)

func newIncidentsCmd() *cobra.Command {
	var window time.Duration
	cmd := &cobra.Command{
		Use:   "incidents",
		Short: "Group events into incidents with named rules, confidence and evidence",
		Long: "Group events that match a named correlation rule. Every incident\n" +
			"carries the rule that fired, its confidence, the window it inspected and\n" +
			"the events behind it.\n\n" +
			"A rule reports what was observed together in one window. It never states\n" +
			"that one event produced another.",
		Args: noArgs,
	}
	flags := addFilterFlags(cmd)
	cmd.Flags().DurationVar(&window, "window", 0, "override every rule's correlation window (0 uses per-rule defaults)")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		return runIncidents(c, flags, window)
	}
	return cmd
}

func runIncidents(cmd *cobra.Command, f *filterFlags, window time.Duration) error {
	if window < 0 {
		return exit.Usage("--window must not be negative, got %s", window)
	}
	res, now, err := fetchQuery(cmd, f)
	if err != nil {
		return err
	}
	incs := correlation.Run(res.Events, correlation.Options{Window: window})
	if f.Format == "json" {
		return output.RenderIncidentsJSON(cmd.OutOrStdout(), res, incs)
	}
	return output.RenderIncidentsText(cmd.OutOrStdout(), res, incs, output.TextOptions{Color: isTTY(cmd.OutOrStdout()), Now: now})
}
