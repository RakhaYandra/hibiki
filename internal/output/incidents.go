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
	return streamArray(out, jsonIncidents(incs))
}

// A hand-built incident can arrive with a nil Related, which encoding/json
// writes as null. The projection gives every incident the same array shape,
// and it lists the engine fields in the engine's own order because an embedded
// shadowed field would be marshalled last and change the published shape.
type jsonIncident struct {
	ID          string                 `json:"id"`
	Rule        string                 `json:"rule"`
	Confidence  correlation.Confidence `json:"confidence"`
	Reason      string                 `json:"reason"`
	Primary     event.Event            `json:"primary"`
	Related     []event.Event          `json:"related"`
	WindowStart time.Time              `json:"window_start"`
	WindowEnd   time.Time              `json:"window_end"`
}

func jsonIncidents(incs []correlation.Incident) []jsonIncident {
	out := make([]jsonIncident, 0, len(incs))
	for _, inc := range incs {
		out = append(out, jsonIncident{
			ID:          inc.ID,
			Rule:        inc.Rule,
			Confidence:  inc.Confidence,
			Reason:      inc.Reason,
			Primary:     inc.Primary,
			Related:     sliceOrEmpty(inc.Related),
			WindowStart: inc.WindowStart,
			WindowEnd:   inc.WindowEnd,
		})
	}
	return out
}

func RenderIncidentsText(w io.Writer, res timeline.Result, incs []correlation.Incident, opts TextOptions) error {
	loc := time.Local
	if !opts.Now.IsZero() {
		loc = opts.Now.Location()
	}
	out := bufio.NewWriter(w)

	fmt.Fprintf(out, "Incidents (%d)\n", len(incs))
	switch {
	case len(incs) > 0:
		for _, inc := range incs {
			renderIncident(out, inc, loc, opts.Color)
		}
	case len(res.Events) == 0:
		// No rule ran over zero events, so there is no correlation result to
		// report: the wording stays the one the timeline list uses for the
		// same fact instead of claiming a search came up empty.
		fmt.Fprintln(out, "\nno events matched in the requested window")
	default:
		fmt.Fprintln(out, "\nNo incidents found.")
	}
	fmt.Fprintf(out, "covered: %s\n", coverageLabel(res))

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
	fmt.Fprintf(out, "    window inspected %s .. %s\n", inc.WindowStart.Format(time.RFC3339Nano), inc.WindowEnd.Format(time.RFC3339Nano))
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
