package cli

import (
	"context"
	"time"

	"github.com/RakhaYandra/hibiki/internal/tui"
	"github.com/spf13/cobra"
)

const (
	detailWindow    = 2 * time.Minute
	interruptedCode = 130
)

func newTuiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Explore the timeline interactively",
		Long: "Explore the systemd journal as one interactive timeline: move through\n" +
			"events, filter them, and inspect a single event with the journal fields\n" +
			"it came from.\n\n" +
			"Reading is all it does: no command writes to the journal.",
		Args: cobra.NoArgs,
	}
	flags := addFilterFlags(cmd)
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		filter, err := flags.toFilter(nowFunc())
		if err != nil {
			return err
		}
		ctx := c.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		return tui.Run(ctx, tui.Deps{
			Sources:      newSources(),
			Now:          nowFunc,
			Save:         saveSnapshot,
			PageSize:     flags.Limit,
			DetailWindow: detailWindow,
		}, tui.Options{Filter: filter, Limit: flags.Limit}, c.InOrStdin(), c.OutOrStdout())
	}
	return cmd
}
