package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/timeline"
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
	fields  []filterField
}

type filterField struct {
	label string
	value string
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
	if st.mode == modeFilter || (st.mode == modeWindow && st.choice == customIndex()) {
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

func (m Model) openKeywordInput() Model {
	input := textinput.New()
	input.Placeholder = "text in the message"
	input.CharLimit = 128
	input.Prompt = ""
	input.Focus()
	m.input = &filterState{mode: modeKeyword, input: input}
	return m
}

func (m Model) handleKeywordKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.input = nil
		return m, nil
	case "enter":
		m.applyKeyword(m.input.input.Value())
		return m, nil
	}
	var cmd tea.Cmd
	m.input.input, cmd = m.input.input.Update(msg)
	return m, cmd
}

func (m *Model) applyKeyword(term string) {
	m.search = strings.TrimSpace(term)
	m.input = nil
	if m.search == "" {
		return
	}
	m.jumpMatch(1)
}

func (m Model) matches(ev event.Event) bool {
	if m.search == "" {
		return false
	}
	return timeline.Match(timeline.Filter{Keyword: m.search}, ev)
}

func (m *Model) jumpMatch(direction int) {
	if m.search == "" || len(m.events) == 0 {
		return
	}
	n := len(m.events)
	for step := 1; step <= n; step++ {
		i := ((m.selected+direction*step)%n + n) % n
		if m.matches(m.events[i]) {
			m.selected = i
			m.ensureVisible()
			return
		}
	}
}

func (m Model) anyMatch() bool {
	for _, ev := range m.events {
		if m.matches(ev) {
			return true
		}
	}
	return false
}

func (m Model) openFilterMenu() Model {
	input := textinput.New()
	input.CharLimit = 128
	input.Prompt = ""
	st := &filterState{
		mode: modeFilter,
		fields: []filterField{
			{label: "severity", value: joinSeverities(m.filter.Severities)},
			{label: "category", value: joinCategories(m.filter.Categories)},
			{label: "unit", value: strings.Join(m.filter.Units, ",")},
			{label: "keyword", value: m.filter.Keyword},
		},
		input: input,
	}
	st.loadCurrent()
	st.syncFocus()
	m.input = st
	return m
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	st := m.input
	switch msg.String() {
	case "esc":
		m.input = nil
		return m, nil
	case "up", "k":
		st.commitCurrent()
		st.choice = clampChoice(st.choice-1, len(st.fields))
		st.err = ""
		st.loadCurrent()
		return m, nil
	case "down", "j":
		st.commitCurrent()
		st.choice = clampChoice(st.choice+1, len(st.fields))
		st.err = ""
		st.loadCurrent()
		return m, nil
	case "enter":
		st.commitCurrent()
		next, err := st.toFilter(m.filter)
		if err != nil {
			st.err = err.Error()
			return m, nil
		}
		m.filter = next
		m.input = nil
		m.selected = 0
		m.offset = 0
		return m, m.startQuery(m.filter, m.limit, "")
	}
	var cmd tea.Cmd
	st.input, cmd = st.input.Update(msg)
	return m, cmd
}

func (st *filterState) commitCurrent() {
	if st.mode != modeFilter || st.choice >= len(st.fields) {
		return
	}
	st.fields[st.choice].value = st.input.Value()
}

func (st *filterState) loadCurrent() {
	if st.mode != modeFilter || st.choice >= len(st.fields) {
		return
	}
	st.input.SetValue(st.fields[st.choice].value)
	st.input.CursorEnd()
}

func (st *filterState) fieldValue(label string) string {
	for _, f := range st.fields {
		if f.label == label {
			return f.value
		}
	}
	return ""
}

func (st *filterState) toFilter(base timeline.Filter) (timeline.Filter, error) {
	f := base
	severities, err := parseSeverityList(st.fieldValue("severity"))
	if err != nil {
		return base, err
	}
	f.Severities = severities

	categories, err := parseCategoryList(st.fieldValue("category"))
	if err != nil {
		return base, err
	}
	f.Categories = categories
	f.Units = splitCSV(st.fieldValue("unit"))
	f.Keyword = strings.TrimSpace(st.fieldValue("keyword"))
	return f, nil
}

func parseSeverityList(value string) ([]event.Severity, error) {
	parts := splitCSV(value)
	out := make([]event.Severity, 0, len(parts))
	for _, p := range parts {
		s := event.Severity(p)
		if !s.Valid() {
			return nil, fmt.Errorf("unknown severity %q: use debug, info, warning, error, critical or unknown", p)
		}
		out = append(out, s)
	}
	return out, nil
}

func parseCategoryList(value string) ([]event.Category, error) {
	parts := splitCSV(value)
	out := make([]event.Category, 0, len(parts))
	for _, p := range parts {
		c := event.Category(p)
		if !c.Valid() {
			return nil, fmt.Errorf("unknown category %q: use system, service, application, process, security or unknown", p)
		}
		out = append(out, c)
	}
	return out, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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
		for i, field := range st.fields {
			value := field.value
			if i == st.choice {
				value = st.input.View()
			}
			lines = append(lines, cursorLead(i == st.choice)+field.label+": "+value)
		}
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
