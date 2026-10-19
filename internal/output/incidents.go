package output

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/RakhaYandra/hibiki/internal/correlation"
	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

// RenderIncidentsJSON streams one incidents document from an envelope the
// caller built, so stdout and a --save file render the same generated_at and
// neither holds a second copy of the result in memory (PRD 14).
func RenderIncidentsJSON(w io.Writer, env Envelope, incs []correlation.Incident) error {
	out, err := streamHead(w, env, "incidents")
	if err != nil {
		return err
	}
	return streamArray(out, jsonIncidents(incs))
}

// SaveIncidentsJSON writes the same document RenderIncidentsJSON prints to the
// file, from the same envelope so generated_at matches stdout byte for byte.
func SaveIncidentsJSON(path string, env Envelope, incs []correlation.Incident) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	if err := RenderIncidentsJSON(f, env, incs); err != nil {
		f.Close()
		return fmt.Errorf("cannot encode the result: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return nil
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
