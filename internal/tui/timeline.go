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

func (m Model) rows() int {
	if m.height <= 0 {
		return 0
	}
	return m.height
}

func (m Model) visibleRows() []string {
	rows := m.rows()
	if rows <= 0 || len(m.events) == 0 {
		return nil
	}
	end := m.offset + rows
	if end > len(m.events) {
		end = len(m.events)
	}
	rowsOut := make([]string, 0, end-m.offset)
	for i := m.offset; i < end; i++ {
		rowsOut = append(rowsOut, m.rowLine(m.events[i], i == m.selected))
	}
	return rowsOut
}

// rowLine adds the cursor column in front of the row. The cursor is drawn as a
// character rather than only as an attribute, because a terminal with colours
// turned off still has to show which event is selected.
func (m Model) rowLine(ev event.Event, selected bool) string {
	return cursorLead(selected) + m.rowStyled(ev, selected)
}

func (m Model) timelineView() string {
	return strings.Join(m.visibleRows(), "\n")
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
	unit := "-"
	if ev.Service.Unit != nil && *ev.Service.Unit != "" {
		unit = *ev.Service.Unit
	}
	return fmt.Sprintf("%s  %-*s  %-*s  %s  %s",
		rowStamp(ev),
		severityWidth, dashIfEmpty(string(ev.Severity)),
		categoryWidth, dashIfEmpty(string(ev.Category)),
		unit,
		oneLine(ev.Message),
	)
}

func (m Model) rowStyled(ev event.Event, selected bool) string {
	plain := m.rowText(ev)
	if m.styles == nil {
		return plain
	}
	if selected {
		return m.styles.selected.Render(plain)
	}
	st, ok := m.styles.severityStyle(ev.Severity)
	if !ok {
		return plain
	}
	return st.Render(plain)
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
