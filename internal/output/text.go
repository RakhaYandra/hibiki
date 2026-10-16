package output

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

const timeLayout = "15:04:05.000"

type TextOptions struct {
	Color bool
	Now   time.Time
}

func RenderText(w io.Writer, res timeline.Result, opts TextOptions) error {
	loc := time.Local
	if !opts.Now.IsZero() {
		loc = opts.Now.Location()
	}
	out := bufio.NewWriter(w)

	for _, e := range res.Events {
		line := EventLine(e, loc)
		if opts.Color {
			line = colorize(e.Severity, line)
		}
		fmt.Fprintln(out, line)
	}

	if len(res.Events) == 0 {
		fmt.Fprintln(out, "no events matched in the requested window")
	}
	fmt.Fprintf(out, "covered: %s\n", coverageLabel(res))

	for _, s := range res.Sources {
		if !s.Available {
			fmt.Fprintf(out, "warning: source %s is unavailable: %s\n", s.Name, s.Detail)
		}
	}
	for _, limitation := range res.Limitations {
		fmt.Fprintf(out, "warning: %s\n", limitation)
	}
	if res.Truncated && res.NextCursor != "" {
		fmt.Fprintf(out, "warning: showing the first %d events; continue with --cursor %s\n", len(res.Events), res.NextCursor)
	}
	return out.Flush()
}

// EventLine is the one line summary shared by the timeline list and the
// inspect surroundings, so both views describe an event the same way.
func EventLine(e event.Event, loc *time.Location) string {
	return fmt.Sprintf("%s | %-8s | %-11s | %s | %s",
		eventTime(e, loc),
		strings.ToUpper(string(e.Severity)),
		e.Category,
		unitLabel(e),
		firstLine(e.Message))
}

func eventTime(e event.Event, loc *time.Location) string {
	if e.TimestampUnixUS == 0 {
		return "-"
	}
	return e.Timestamp.In(loc).Format(timeLayout)
}

func unitLabel(e event.Event) string {
	if e.Service.Unit == nil {
		return "-"
	}
	return *e.Service.Unit
}

// A journal message can be multiline; the list stays one line per event and
// inspect is where the full text belongs.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func coverageLabel(res timeline.Result) string {
	if res.CoveredSince.IsZero() && res.CoveredUntil.IsZero() {
		return "nothing observed"
	}
	return res.CoveredSince.Format(time.RFC3339Nano) + " .. " + res.CoveredUntil.Format(time.RFC3339Nano)
}

func colorize(severity event.Severity, line string) string {
	var code string
	switch severity {
	case event.SeverityCritical, event.SeverityError:
		code = "31"
	case event.SeverityWarning:
		code = "33"
	case event.SeverityDebug:
		code = "90"
	default:
		return line
	}
	return "\x1b[" + code + "m" + line + "\x1b[0m"
}
