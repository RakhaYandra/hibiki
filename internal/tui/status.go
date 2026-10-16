package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

const (
	titleLine      = "HIBIKI / SYSTEM TIMELINE"
	emptyNote      = "no events matched in the requested window"
	loadingNote    = "loading: querying the journal"
	moreEventsNote = "more events below"
	noCoverageNote = "covered: nothing observed"
)

func (m Model) headerView() string {
	lines := []string{titleLine, filterLine(m.filter)}
	chips := sourceChips(m.sources)
	if len(m.limitations) > 0 {
		chips = strings.TrimSpace(chips + "   limitations: " + strings.Join(m.limitations, " "))
	}
	if chips != "" {
		lines = append(lines, chips)
	}
	return strings.Join(lines, "\n")
}

func filterLine(f timeline.Filter) string {
	parts := []string{"window " + windowLabel(f.Since, f.Until)}
	parts = append(parts, filterChips(f)...)
	return strings.Join(parts, "   ")
}

func windowLabel(since, until time.Time) string {
	from := "beginning"
	if !since.IsZero() {
		from = stamp(since)
	}
	to := "now"
	if !until.IsZero() {
		to = stamp(until)
	}
	return from + " .. " + to
}

func filterChips(f timeline.Filter) []string {
	var chips []string
	if len(f.Severities) > 0 {
		chips = append(chips, "severity: "+joinSeverities(f.Severities))
	}
	if len(f.Categories) > 0 {
		chips = append(chips, "category: "+joinCategories(f.Categories))
	}
	if len(f.Units) > 0 {
		chips = append(chips, "unit: "+strings.Join(f.Units, ","))
	}
	if len(f.PIDs) > 0 {
		pids := make([]string, 0, len(f.PIDs))
		for _, pid := range f.PIDs {
			pids = append(pids, fmt.Sprintf("%d", pid))
		}
		chips = append(chips, "pid: "+strings.Join(pids, ","))
	}
	if f.Keyword != "" {
		chips = append(chips, "keyword: "+f.Keyword)
	}
	if f.BootID != "" {
		chips = append(chips, "boot: "+f.BootID)
	}
	return chips
}

func joinSeverities(sevs []event.Severity) string {
	out := make([]string, 0, len(sevs))
	for _, sev := range sevs {
		out = append(out, string(sev))
	}
	return strings.Join(out, ",")
}

func joinCategories(cats []event.Category) string {
	out := make([]string, 0, len(cats))
	for _, cat := range cats {
		out = append(out, string(cat))
	}
	return strings.Join(out, ",")
}

func sourceChips(srcs []source.Status) string {
	chips := make([]string, 0, len(srcs))
	for _, src := range srcs {
		if src.Available {
			chips = append(chips, src.Name+" ok")
			continue
		}
		reason := strings.TrimSpace(src.Detail)
		if reason == "" {
			reason = "no reason reported"
		}
		chips = append(chips, src.Name+" unavailable: "+reason)
	}
	return strings.Join(chips, "   ")
}

func (m Model) coverageLine() string {
	if m.coveredSince.IsZero() && m.coveredUntil.IsZero() {
		return noCoverageNote
	}
	return "covered: " + stamp(m.coveredSince) + " .. " + stamp(m.coveredUntil)
}

func (m Model) footerView() string {
	var lines []string
	if promptErr := m.promptError(); promptErr != "" {
		lines = append(lines, promptErr)
	}
	switch {
	case m.loading:
		lines = append(lines, loadingNote)
	case m.err != nil:
	default:
		parts := []string{m.coverageLine()}
		if m.truncated && m.cursorNext != "" {
			parts = append(parts, moreEventsNote)
		}
		if m.saveWarning != "" {
			parts = append(parts, "snapshot warning: "+m.saveWarning)
		}
		lines = append(lines, strings.Join(parts, "   "))
	}
	lines = append(lines, m.keyHints())
	return strings.Join(lines, "\n")
}

func (m Model) keyHints() string {
	hints := []string{"j/k move", "g/G top/bottom", "t window", "r refresh", "q quit"}
	return "keys: " + strings.Join(hints, "  ")
}

func (m Model) emptyState() string {
	line := emptyNote
	if len(m.sources) > 0 && countReadable(m.sources) == 0 {
		line = "no source could be read"
	}
	lines := []string{line}
	if len(m.limitations) > 0 {
		lines = append(lines, "limitations: "+strings.Join(m.limitations, " "))
	}
	return strings.Join(lines, "\n")
}

func countReadable(srcs []source.Status) int {
	n := 0
	for _, src := range srcs {
		if src.Available {
			n++
		}
	}
	return n
}

func (m Model) errorView() string {
	if m.err == nil {
		return ""
	}
	code := exit.FromError(m.err)
	return fmt.Sprintf("error: %s\nexit code %d (%s)", m.err, code, exitMeaning(code))
}

func exitMeaning(code int) string {
	switch code {
	case exit.CodeUsage:
		return "usage or configuration"
	case exit.CodeSourceUnavailable:
		return "source unavailable"
	case exit.CodeBadData:
		return "bad data"
	case exit.CodeInternal:
		return "internal error"
	}
	return "ok"
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.UTC().Format(time.RFC3339)
}
