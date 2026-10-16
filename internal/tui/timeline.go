package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
)

const (
	severityWidth = 8
	categoryWidth = 11
)

func (m *Model) moveSelection(delta int) {
	if len(m.events) == 0 {
		m.selected = 0
		m.offset = 0
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected > len(m.events)-1 {
		m.selected = len(m.events) - 1
	}
	m.ensureVisible()
}

func (m *Model) ensureVisible() {
	rows := m.rows()
	if rows <= 0 {
		m.offset = 0
		return
	}
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+rows {
		m.offset = m.selected - rows + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) visibleRows() []string {
	return m.listRows(m.width, m.rows())
}

func (m Model) listRows(width, rows int) []string {
	if rows <= 0 || len(m.events) == 0 {
		return nil
	}
	end := m.offset + rows
	if end > len(m.events) {
		end = len(m.events)
	}
	rowsOut := make([]string, 0, end-m.offset)
	for i := m.offset; i < end; i++ {
		rowsOut = append(rowsOut, m.rowLine(m.events[i], i == m.selected, width))
	}
	return rowsOut
}

// rowLine adds the cursor column, cuts the row to the pane width and colours
// what is left. The cut happens on plain text on purpose: cutting a coloured
// string is what leaves half an escape sequence behind. The cursor is drawn as
// a character rather than only as an attribute, because a terminal with colours
// turned off still has to show which event is selected.
func (m Model) rowLine(ev event.Event, selected bool, width int) string {
	form := rowFormFor(width)
	plain := truncate(cursorLead(selected)+m.rowTextInForm(ev, form), width)
	if m.styles == nil {
		return plain
	}
	if selected {
		return m.styles.selected.Render(plain)
	}
	head, tail := plain, ""
	if start := messageStart(ev, form); start < len(plain) {
		head, tail = plain[:start], plain[start:]
	}
	head = m.styles.colourCell(head, severityStart(ev, form), ev.Severity)
	if m.search != "" {
		tail = m.styles.highlightMatches(tail, m.search)
	}
	return head + tail
}

// A narrow pane drops whole columns instead of cutting every one of them: forty
// columns of a timestamp, a severity and a cut off unit name would leave the
// message, the part a person reads, invisible.
type rowForm int

const (
	formFull rowForm = iota
	formNoCategory
	formCompact
)

func rowFormFor(width int) rowForm {
	switch {
	case width <= 0 || width >= 72:
		return formFull
	case width >= 44:
		return formNoCategory
	}
	return formCompact
}

func (m Model) rowTextInForm(ev event.Event, form rowForm) string {
	c := rowCellsOf(ev)
	switch form {
	case formNoCategory:
		return fmt.Sprintf("%s  %s  %s  %s", c.stamp, pad(c.severity, severityWidth), c.unit, c.message)
	case formCompact:
		return fmt.Sprintf("%s  %s  %s", c.stamp, pad(c.severity, severityWidth), c.message)
	}
	return m.rowText(ev)
}

func severityStart(ev event.Event, form rowForm) int {
	return len(cursorLead(false)) + len(rowStamp(ev)) + 2
}

func messageStart(ev event.Event, form rowForm) int {
	c := rowCellsOf(ev)
	start := severityStart(ev, form) + severityWidth + 2
	switch form {
	case formNoCategory:
		return start + len(c.unit) + 2
	case formCompact:
		return start
	}
	return start + categoryWidth + 2 + len(c.unit) + 2
}

func (m Model) timelineView() string {
	return strings.Join(m.listRows(m.width, m.rows()), "\n")
}

func (m Model) selectedEvent() (event.Event, bool) {
	if m.selected < 0 || m.selected >= len(m.events) {
		return event.Event{}, false
	}
	return m.events[m.selected], true
}

// rowText is the plain form of a row: tests and truncation work on it, so a
// cut never lands inside an escape sequence.
func (m Model) rowText(ev event.Event) string {
	c := rowCellsOf(ev)
	return fmt.Sprintf(rowFormat, c.stamp, pad(c.severity, severityWidth), pad(c.category, categoryWidth), c.unit, c.message)
}

type rowCells struct {
	stamp    string
	severity string
	category string
	unit     string
	message  string
}

const rowFormat = "%s  %s  %s  %s  %s"

func rowCellsOf(ev event.Event) rowCells {
	unit := "-"
	if ev.Service.Unit != nil && *ev.Service.Unit != "" {
		unit = *ev.Service.Unit
	}
	return rowCells{
		stamp:    rowStamp(ev),
		severity: dashIfEmpty(string(ev.Severity)),
		category: dashIfEmpty(string(ev.Category)),
		unit:     unit,
		message:  oneLine(ev.Message),
	}
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func rowStamp(ev event.Event) string {
	switch {
	case !ev.Timestamp.IsZero():
		return ev.Timestamp.Local().Format("15:04:05.000")
	case ev.TimestampUnixUS != 0:
		return time.UnixMicro(ev.TimestampUnixUS).Local().Format("15:04:05.000")
	}
	return "unknown"
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}
