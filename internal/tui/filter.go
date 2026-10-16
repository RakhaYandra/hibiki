package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/RakhaYandra/hibiki/internal/timeparse"
)

type filterMode int

const (
	modeNone filterMode = iota
	modeWindow
	modeFilter
	modeKeyword
)

const customSpec = "custom"

var windowPresets = []string{"5m", "30m", "1h", "6h", "24h", customSpec}

type filterState struct {
	mode    filterMode
	presets []string
	choice  int
	input   textinput.Model
	err     string
}

func customIndex() int { return len(windowPresets) - 1 }

func (m Model) now() time.Time {
	if m.deps.Now != nil {
		return m.deps.Now()
	}
	return time.Now()
}

func (m Model) openWindowPicker() Model {
	input := textinput.New()
	input.Placeholder = "2h or 2026-10-16T08:00:00Z"
	input.CharLimit = 64
	input.Prompt = ""

	st := &filterState{
		mode:    modeWindow,
		presets: append([]string(nil), windowPresets...),
		choice:  presetIndex(m.now(), m.filter.Since),
		input:   input,
	}
	st.syncFocus()
	m.input = st
	return m
}

func presetIndex(now, since time.Time) int {
	if since.IsZero() {
		return customIndex()
	}
	age := now.Sub(since)
	for i, spec := range windowPresets {
		if spec == customSpec {
			continue
		}
		span, err := time.ParseDuration(spec)
		if err != nil {
			continue
		}
		if age == span {
			return i
		}
	}
	return customIndex()
}

func (st *filterState) syncFocus() {
	if st.mode == modeWindow && st.choice == customIndex() {
		st.input.Focus()
		return
	}
	st.input.Blur()
}

func (m Model) handleWindowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	st := m.input
	switch msg.String() {
	case "esc":
		m.input = nil
		return m, nil
	case "up", "k":
		st.choice = clampChoice(st.choice-1, len(st.presets))
		st.err = ""
		st.syncFocus()
		return m, nil
	case "down", "j":
		st.choice = clampChoice(st.choice+1, len(st.presets))
		st.err = ""
		st.syncFocus()
		return m, nil
	case "enter":
		spec := st.presets[st.choice]
		if st.choice == customIndex() {
			spec = strings.TrimSpace(st.input.Value())
		}
		since, err := timeparse.ParseSince(spec, m.now())
		if err != nil {
			st.err = err.Error()
			return m, nil
		}
		m.filter.Since = since
		m.input = nil
		m.selected = 0
		m.offset = 0
		return m, m.startQuery(m.filter, m.limit, "")
	}
	if st.choice == customIndex() {
		var cmd tea.Cmd
		st.input, cmd = st.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func clampChoice(choice, n int) int {
	if choice < 0 {
		return 0
	}
	if choice > n-1 {
		return n - 1
	}
	return choice
}

func (m *Model) refresh() tea.Cmd {
	m.selected = 0
	m.offset = 0
	return m.startQuery(m.filter, m.limit, "")
}

func (m Model) pickerView() string {
	st := m.input
	if st == nil {
		return ""
	}
	var lines []string
	switch st.mode {
	case modeWindow:
		lines = append(lines, "window start")
		for i, spec := range st.presets {
			label := spec
			if spec == customSpec {
				label = "custom (span or RFC 3339): " + st.input.View()
			}
			lines = append(lines, cursorLead(i == st.choice)+label)
		}
	case modeFilter:
		lines = append(lines, "filters")
	}
	lines = append(lines, "keys: j/k choose  enter apply  esc cancel")
	return strings.Join(lines, "\n")
}

func cursorLead(selected bool) string {
	if selected {
		return "> "
	}
	return "  "
}

func (m Model) promptError() string {
	if m.input == nil {
		return ""
	}
	return m.input.err
}
