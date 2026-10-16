package tui

import (
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/RakhaYandra/hibiki/internal/event"
)

type styles struct {
	renderer  *lipgloss.Renderer
	selected  lipgloss.Style
	muted     lipgloss.Style
	highlight lipgloss.Style
	severity  map[event.Severity]lipgloss.Style
}

func newStyles(w io.Writer) *styles {
	r := lipgloss.NewRenderer(w)
	s := &styles{
		renderer:  r,
		selected:  r.NewStyle().Reverse(true),
		muted:     r.NewStyle().Foreground(lipgloss.Color("244")),
		highlight: r.NewStyle().Bold(true).Foreground(lipgloss.Color("214")),
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

// colourCell colours a fixed-width column inside an already cut plain line.
// When the cut did not leave the whole cell, the line is returned unchanged.
func (s *styles) colourCell(line string, start int, sev event.Severity) string {
	end := start + severityWidth
	if s == nil || start < 0 || len(line) < end {
		return line
	}
	st, ok := s.severityStyle(sev)
	if !ok {
		return line
	}
	return line[:start] + st.Render(line[start:end]) + line[end:]
}

// highlightMatches wraps every occurrence of term, case-insensitively. A slice
// whose fold does not round-trip (a length-changing Unicode case) is skipped one
// byte at a time rather than cut mid-rune.
func (s *styles) highlightMatches(text, term string) string {
	if s == nil || term == "" {
		return text
	}
	lower := strings.ToLower(text)
	needle := strings.ToLower(term)
	var b strings.Builder
	i := 0
	for i < len(text) {
		at := strings.Index(lower[i:], needle)
		if at < 0 {
			b.WriteString(text[i:])
			break
		}
		start := i + at
		end := start + len(term)
		if end > len(text) || !strings.EqualFold(text[start:end], term) {
			b.WriteString(text[i : start+1])
			i = start + 1
			continue
		}
		b.WriteString(text[i:start])
		b.WriteString(s.highlight.Render(text[start:end]))
		i = end
	}
	return b.String()
}
