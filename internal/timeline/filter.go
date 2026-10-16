package timeline

import (
	"slices"
	"strings"

	"github.com/RakhaYandra/hibiki/internal/event"
)

// Every predicate is AND; the values inside one predicate are OR. An event with
// no timestamp never passes a bounded window: the window cannot place it
// (PRD 8.2). The keyword only ever reads the message, never raw fields.
// Match is the one predicate behind both surfaces: the CLI filter flags and the
// TUI search must agree on what a keyword means, so neither reimplements it.
func Match(f Filter, e event.Event) bool {
	if !f.Since.IsZero() && (e.TimestampUnixUS == 0 || e.TimestampUnixUS < f.Since.UnixMicro()) {
		return false
	}
	if !f.Until.IsZero() && (e.TimestampUnixUS == 0 || e.TimestampUnixUS > f.Until.UnixMicro()) {
		return false
	}
	if len(f.Categories) > 0 && !slices.Contains(f.Categories, e.Category) {
		return false
	}
	if len(f.Severities) > 0 && !slices.Contains(f.Severities, e.Severity) {
		return false
	}
	if len(f.Units) > 0 && (e.Service.Unit == nil || !slices.Contains(f.Units, *e.Service.Unit)) {
		return false
	}
	if len(f.PIDs) > 0 && (e.Process.PID == nil || !slices.Contains(f.PIDs, *e.Process.PID)) {
		return false
	}
	if f.BootID != "" && (e.Source.BootID == nil || *e.Source.BootID != f.BootID) {
		return false
	}
	if f.Keyword != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(f.Keyword)) {
		return false
	}
	return true
}
