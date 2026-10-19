package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RakhaYandra/hibiki/internal/correlation"
	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/inspect"
)

type detailState struct {
	ev      event.Event
	ctx     *inspect.Context
	incs    []correlation.Incident
	offset  int
	loading bool
	err     error
	token   int
}

type detailDoneMsg struct {
	token int
	ctx   inspect.Context
	incs  []correlation.Incident
	err   error
}

func (m *Model) openDetail() tea.Cmd {
	ev, ok := m.selectedEvent()
	if !ok {
		return nil
	}
	m.detailToken++
	token := m.detailToken
	if m.detailCancel != nil {
		m.detailCancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.detailCancel = cancel
	finished := make(chan struct{})
	m.detailDone = finished
	m.detail = &detailState{ev: ev, loading: true, token: token}

	srcs := m.deps.Sources
	window := m.deps.DetailWindow
	return func() tea.Msg {
		defer close(finished)
		out, err := inspect.ForEvent(ctx, srcs, ev, window)
		// The correlation runs in the command goroutine, off the frame path;
		// Update only stores the incidents the command already computed.
		out, incs := inspect.Correlate(out)
		return detailDoneMsg{token: token, ctx: out, incs: incs, err: err}
	}
}

// The correlation ran off the frame path in the command above; the frame path
// only reads the incidents it already has.
func (m *Model) applyDetailDone(msg detailDoneMsg) {
	if m.detail == nil || msg.token != m.detail.token {
		return
	}
	m.detail.loading = false
	m.detail.err = msg.err
	m.detail.ctx = &msg.ctx
	m.detail.incs = msg.incs
}

func (m *Model) closeDetail() {
	if m.detailCancel != nil {
		m.detailCancel()
		m.detailCancel = nil
	}
	m.detail = nil
}

func (m *Model) scrollDetail(delta int) {
	if m.detail == nil {
		return
	}
	m.detail.offset += delta
	m.clampDetail()
}

func (m *Model) clampDetail() {
	if m.detail == nil {
		return
	}
	max := len(m.detailLines()) - m.rows()
	if max < 0 {
		max = 0
	}
	if m.detail.offset > max {
		m.detail.offset = max
	}
	if m.detail.offset < 0 {
		m.detail.offset = 0
	}
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeDetail()
	case "j", "down":
		m.scrollDetail(1)
	case "k", "up":
		m.scrollDetail(-1)
	case "G":
		m.detail.offset = len(m.detailLines())
		m.clampDetail()
	case "g":
		m.detail.offset = 0
		m.clampDetail()
	case "q":
		m.cancelInFlight()
		return m, tea.Quit
	case "ctrl+c":
		m.cancelInFlight()
		m.interrupted = true
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) detailLines() []string {
	d := m.detail
	if d == nil {
		return nil
	}
	ev := d.ev
	lines := []string{
		"event " + ev.ID,
		"time  " + stamp(ev.Timestamp) + "   basis " + dashIfEmpty(string(ev.TimeBasis)),
		"category " + dashIfEmpty(string(ev.Category)) + "   severity " + dashIfEmpty(string(ev.Severity)),
		"message " + oneLine(ev.Message),
	}
	if d.loading {
		lines = append(lines, loadingNote)
	}
	if d.err != nil {
		lines = append(lines, "error: "+d.err.Error())
	}

	if ev.Service.Unit != nil {
		lines = append(lines, "unit  "+*ev.Service.Unit)
	}
	if ev.Process.PID != nil {
		lines = append(lines, "pid   "+strconv.Itoa(*ev.Process.PID))
	}
	source := "source " + dashIfEmpty(ev.Source.Type)
	if ev.Source.Host != "" {
		source += " on " + ev.Source.Host
	}
	if ev.Source.BootID != nil {
		source += "   boot " + *ev.Source.BootID
	}
	lines = append(lines, source)

	lines = append(lines, fmt.Sprintf("fields (%d)", len(ev.Raw)))
	keys := make([]string, 0, len(ev.Raw))
	for key := range ev.Raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, "  "+key+"="+oneLine(ev.Raw[key]))
	}

	if d.ctx == nil {
		return lines
	}
	lines = append(lines, fmt.Sprintf("before (%d)", len(d.ctx.Before)))
	for _, prev := range d.ctx.Before {
		lines = append(lines, "  "+m.rowText(prev))
	}
	lines = append(lines, fmt.Sprintf("after (%d)", len(d.ctx.After)))
	for _, next := range d.ctx.After {
		lines = append(lines, "  "+m.rowText(next))
	}
	for _, inc := range d.incs {
		lines = append(lines, fmt.Sprintf("correlation %s %s [%s]", inc.ID, inc.Rule, inc.Confidence))
		lines = append(lines, fmt.Sprintf("    window inspected %s .. %s", inc.WindowStart.UTC().Format(time.RFC3339Nano), inc.WindowEnd.UTC().Format(time.RFC3339Nano)))
		lines = append(lines, "  "+oneLine(inc.Reason))
	}
	if len(d.ctx.Limitations) > 0 {
		lines = append(lines, "limitations: "+strings.Join(d.ctx.Limitations, " "))
	}
	return lines
}

func (m Model) detailView() string {
	return strings.Join(m.detailRows(m.width, m.rows()), "\n")
}

func (m Model) detailRows(width, rows int) []string {
	lines := m.detailLines()
	start := 0
	if rows > 0 {
		if m.detail.offset < len(lines) {
			start = m.detail.offset
		} else {
			start = len(lines)
		}
		end := start + rows
		if end > len(lines) {
			end = len(lines)
		}
		lines = lines[start:end]
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, truncate(line, width))
	}
	return out
}
