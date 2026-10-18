package output

import (
	"bufio"
	"fmt"
	"io"
	"time"

	"github.com/RakhaYandra/hibiki/internal/correlation"
	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

// RenderIncidentsJSON streams the same envelope the timeline renders with the
// incident list in place of the events, so a machine consumer reads both
// documents the same way and the evidence inside each incident already carries
// the incident ids that reference it.
func RenderIncidentsJSON(w io.Writer, res timeline.Result, incs []correlation.Incident) error {
	out, err := streamHead(w, NewEnvelope(res), "incidents")
	if err != nil {
		return err
	}
	return streamArray(out, sliceOrEmpty(incs))
}

func RenderIncidentsText(w io.Writer, res timeline.Result, incs []correlation.Incident, opts TextOptions) error {
	loc := time.Local
	if !opts.Now.IsZero() {
		loc = opts.Now.Location()
	}
	out := bufio.NewWriter(w)

	fmt.Fprintf(out, "Incidents (%d)\n", len(incs))
	if len(incs) == 0 {
		fmt.Fprintln(out, "\nNo incidents found.")
	} else {
		for _, inc := range incs {
			renderIncident(out, inc, loc, opts.Color)
		}
	}

	fmt.Fprintln(out, "\nSources")
	for _, s := range res.Sources {
		state := "ok"
		if !s.Available {
			state = "unavailable"
		}
		fmt.Fprintf(out, "  %s: %s (%s)\n", s.Name, state, s.Detail)
	}

	if len(res.Limitations) > 0 {
		fmt.Fprintln(out, "\nLimitations")
		for _, l := range res.Limitations {
			fmt.Fprintf(out, "  %s\n", l)
		}
	}
	// A truncated read may have dropped the events an incident needed, so the
	// answer must say it is partial instead of reading as a complete one.
	if res.Truncated && res.NextCursor != "" {
		fmt.Fprintf(out, "\nwarning: showing the first %d events; continue with --cursor %s\n", len(res.Events), res.NextCursor)
	}
	return out.Flush()
}

// The window and the evidence lines are the whole claim of an incident: a rule
// only asserts what it saw inside the window, and EventLine keeps one event
// reading the same way here and in the timeline list.
func renderIncident(out *bufio.Writer, inc correlation.Incident, loc *time.Location, color bool) {
	fmt.Fprintf(out, "\n  %s %s [%s]\n", inc.ID, inc.Rule, inc.Confidence)
	fmt.Fprintf(out, "    window %s .. %s\n", inc.WindowStart.Format(time.RFC3339Nano), inc.WindowEnd.Format(time.RFC3339Nano))
	fmt.Fprintf(out, "    reason %s\n", inc.Reason)
	fmt.Fprintf(out, "    primary   %s\n", incidentEventLine(inc.Primary, loc, color))
	fmt.Fprintf(out, "    related %d\n", len(inc.Related))
	for _, rel := range inc.Related {
		fmt.Fprintf(out, "      %s\n", incidentEventLine(rel, loc, color))
	}
}

func incidentEventLine(e event.Event, loc *time.Location, color bool) string {
	line := EventLine(e, loc)
	if !color {
		return line
	}
	return colorize(e.Severity, line)
}
