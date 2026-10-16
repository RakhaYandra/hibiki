package tui

import (
	"io"

	"github.com/charmbracelet/lipgloss"

	"github.com/RakhaYandra/hibiki/internal/event"
)

type styles struct {
	renderer *lipgloss.Renderer
	selected lipgloss.Style
	muted    lipgloss.Style
	severity map[event.Severity]lipgloss.Style
}

func newStyles(w io.Writer) *styles {
	r := lipgloss.NewRenderer(w)
	s := &styles{
		renderer: r,
		selected: r.NewStyle().Reverse(true),
		muted:    r.NewStyle().Foreground(lipgloss.Color("244")),
		severity: map[event.Severity]lipgloss.Style{
			event.SeverityCritical: r.NewStyle().Foreground(lipgloss.Color("196")),
			event.SeverityError:    r.NewStyle().Foreground(lipgloss.Color("203")),
			event.SeverityWarning:  r.NewStyle().Foreground(lipgloss.Color("214")),
			event.SeverityInfo:     r.NewStyle().Foreground(lipgloss.Color("39")),
			event.SeverityDebug:    r.NewStyle().Foreground(lipgloss.Color("244")),
		},
	}
	return s
}

func (s *styles) severityStyle(sev event.Severity) (lipgloss.Style, bool) {
	if s == nil {
		return lipgloss.Style{}, false
	}
	st, ok := s.severity[sev]
	return st, ok
}
