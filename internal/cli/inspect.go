package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/RakhaYandra/hibiki/internal/correlation"
	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/inspect"
	"github.com/RakhaYandra/hibiki/internal/output"
	"github.com/RakhaYandra/hibiki/internal/session"
	"github.com/RakhaYandra/hibiki/internal/timeline"
	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	var (
		contextWindow time.Duration
		file          string
		format        string
	)
	cmd := &cobra.Command{
		Use:   "inspect <event-id>",
		Short: "Show one event with its surroundings and raw journal fields",
		Long: "Show one event in full: the normalized fields, every raw journal field\n" +
			"it came from, and what happened shortly before and after it.\n\n" +
			"The event comes from the last timeline or events query, or from a file\n" +
			"written with --save. Surrounding events come from the live journal.",
		Args: exactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runInspect(c, args[0], contextWindow, file, format)
		},
	}
	flags := cmd.Flags()
	flags.DurationVar(&contextWindow, "context", 2*time.Minute, "how far before and after the event to look")
	flags.StringVar(&file, "file", "", "read the event from this saved result instead of the last query")
	flags.StringVar(&format, "format", "text", "output format: text or json")
	return cmd
}

func runInspect(cmd *cobra.Command, eventID string, window time.Duration, file, format string) error {
	if format != "text" && format != "json" {
		return exit.Usage("--format must be text or json, got %q", format)
	}
	if window <= 0 {
		return exit.Usage("--context must be a positive duration, got %s", window)
	}
	target, _, err := loadEvent(file, eventID)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	found, err := inspect.ForEvent(ctx, newSources(), target, window)
	if err != nil {
		return err
	}
	found, incs := inspect.Correlate(found)

	if format == "json" {
		return renderInspectJSON(cmd.OutOrStdout(), found, incs)
	}
	return renderInspectText(cmd.OutOrStdout(), found, incs, window, nowFunc().Location())
}

func loadEvent(file, eventID string) (event.Event, timeline.Result, error) {
	if file == "" {
		return session.Load(session.DefaultDir(), eventID)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return event.Event{}, timeline.Result{}, fmt.Errorf("cannot read %s: %w", file, err)
	}
	var env output.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return event.Event{}, timeline.Result{}, exit.BadData("%s is not a Hibiki result: %v", file, err)
	}
	return session.FindIn(env, eventID)
}

type inspectView struct {
	SchemaVersion int                    `json:"schema_version"`
	Event         event.Event            `json:"event"`
	Before        []event.Event          `json:"before"`
	After         []event.Event          `json:"after"`
	Correlations  []correlation.Incident `json:"correlations"`
	Sources       []output.SourceStatus  `json:"sources"`
	Limitations   []string               `json:"limitations"`
}

func renderInspectJSON(w io.Writer, found inspect.Context, incs []correlation.Incident) error {
	view := inspectView{
		SchemaVersion: event.SchemaVersion,
		Event:         found.Event,
		Before:        found.Before,
		After:         found.After,
		Correlations:  incs,
		Sources:       output.SourceStatuses(found.Sources),
		Limitations:   found.Limitations,
	}
	if view.Before == nil {
		view.Before = []event.Event{}
	}
	if view.After == nil {
		view.After = []event.Event{}
	}
	if view.Correlations == nil {
		view.Correlations = []correlation.Incident{}
	}
	if view.Limitations == nil {
		view.Limitations = []string{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(view)
}

func renderInspectText(w io.Writer, found inspect.Context, incs []correlation.Incident, window time.Duration, loc *time.Location) error {
	out := bufio.NewWriter(w)
	e := found.Event

	fmt.Fprintf(out, "Event %s\n", e.ID)
	field := func(name, value string) {
		if value != "" {
			fmt.Fprintf(out, "  %-12s %s\n", name, value)
		}
	}
	field("time", timeLabel(e))
	field("time_basis", string(e.TimeBasis))
	if e.TimestampMonotonicUS != nil {
		field("monotonic_us", fmt.Sprintf("%d", *e.TimestampMonotonicUS))
	}
	field("category", string(e.Category))
	field("severity", string(e.Severity))
	field("source_type", e.Source.Type)
	field("host", e.Source.Host)
	if e.Source.BootID != nil {
		field("boot_id", *e.Source.BootID)
	}
	if e.Source.Cursor != nil {
		field("cursor", *e.Source.Cursor)
	}
	field("unit", unitOrDash(e))
	if e.Process.PID != nil {
		field("pid", fmt.Sprintf("%d", *e.Process.PID))
	}
	if e.Process.Comm != nil {
		field("comm", *e.Process.Comm)
	}
	if e.Process.Executable != nil {
		field("executable", *e.Process.Executable)
	}
	field("message", e.Message)
	field("correlation", correlationLabel(e))

	fmt.Fprintln(out, "\nRaw journal fields")
	for _, key := range sortedKeys(e.Raw) {
		fmt.Fprintf(out, "  %-22s %s\n", key, e.Raw[key])
	}

	side := func(label string, evs []event.Event) {
		fmt.Fprintf(out, "\n%s (%d events within %s)\n", label, len(evs), window)
		for _, child := range evs {
			fmt.Fprintf(out, "  %s\n", output.EventLine(child, loc))
		}
	}
	side("Before", found.Before)
	side("After", found.After)

	if len(incs) > 0 {
		fmt.Fprintf(out, "\nCorrelations (%d)\n", len(incs))
		for _, inc := range incs {
			fmt.Fprintf(out, "  %s %s [%s]\n", inc.ID, inc.Rule, inc.Confidence)
			fmt.Fprintf(out, "    %s\n", inc.Reason)
		}
	}

	fmt.Fprintln(out, "\nSources")
	for _, s := range found.Sources {
		state := "ok"
		if !s.Available {
			state = "unavailable"
		}
		fmt.Fprintf(out, "  %s: %s (%s)\n", s.Name, state, s.Detail)
	}

	if len(found.Limitations) > 0 {
		fmt.Fprintln(out, "\nLimitations")
		for _, l := range found.Limitations {
			fmt.Fprintf(out, "  %s\n", l)
		}
	}
	return out.Flush()
}

func timeLabel(e event.Event) string {
	if e.TimestampUnixUS == 0 {
		return "unknown"
	}
	return e.Timestamp.UTC().Format(time.RFC3339Nano)
}

func unitOrDash(e event.Event) string {
	if e.Service.Unit == nil {
		return "-"
	}
	return *e.Service.Unit
}

// An empty list must read as "none" rather than as a missing value.
func correlationLabel(e event.Event) string {
	if len(e.CorrelationIDs) == 0 {
		return "none"
	}
	return strings.Join(e.CorrelationIDs, ", ")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
