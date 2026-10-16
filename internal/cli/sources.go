package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/spf13/cobra"
)

func newSourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sources",
		Short: "List event sources and what each one can answer",
		Long: "List every source Hibiki can read, whether it is reachable here, and how\n" +
			"far back it reaches. A source that cannot reconstruct history says so.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runSources(ctx, c.OutOrStdout())
		},
	}
}

func runSources(ctx context.Context, w io.Writer) error {
	out := bufio.NewWriter(w)
	for _, src := range newSources() {
		status := src.Available(ctx)
		state := "available"
		if !status.Available {
			state = "unavailable"
		}
		fmt.Fprintf(out, "%s\n", status.Name)
		fmt.Fprintf(out, "  %-9s %s\n", "status", state)
		fmt.Fprintf(out, "  %-9s %s\n", "detail", status.Detail)
		if !status.Available {
			continue
		}
		oldest, newest, err := coverage(ctx, src)
		switch {
		case err != nil:
			fmt.Fprintf(out, "  %-9s unknown (%v)\n", "coverage", err)
		case oldest == nil || newest == nil:
			fmt.Fprintf(out, "  %-9s no records\n", "coverage")
		default:
			fmt.Fprintf(out, "  %-9s %s .. %s\n", "coverage", coverageTime(*oldest), coverageTime(*newest))
		}
		fmt.Fprintf(out, "  %-9s %s\n", "notes", "read only; every journal field is preserved verbatim")
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "process")
	fmt.Fprintln(out, "  status    not available in v0.1.0")
	fmt.Fprintln(out, "  detail    process history cannot be reconstructed from the journal; a process is only observable while it runs (PRD 6.1)")
	return out.Flush()
}

// The oldest record needs a plain query and the newest needs journalctl's own
// tail, so the two ends come from two reads (and the iterator closes the
// process as soon as it has what it needs).
func coverage(ctx context.Context, src source.Source) (*event.Event, *event.Event, error) {
	oldest, err := firstEvent(ctx, src, source.Query{})
	if err != nil {
		return nil, nil, err
	}
	newest, err := firstEvent(ctx, src, source.Query{Limit: 1})
	if err != nil {
		return nil, nil, err
	}
	return oldest, newest, nil
}

func firstEvent(ctx context.Context, src source.Source, q source.Query) (*event.Event, error) {
	it, err := src.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer it.Close()
	ev, err := it.Next(ctx)
	if errors.Is(err, io.EOF) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ev, nil
}

func coverageTime(e event.Event) string {
	if e.TimestampUnixUS == 0 {
		return "unknown"
	}
	return e.Timestamp.UTC().Format(time.RFC3339Nano)
}
