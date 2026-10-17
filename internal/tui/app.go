package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/RakhaYandra/hibiki/internal/diagnostics"
	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

var ErrInterrupted = errors.New("interrupted")

type Deps struct {
	Sources      []source.Source
	Now          func() time.Time
	Save         func(timeline.Result) (string, error)
	PageSize     int
	DetailWindow time.Duration
}

type Options struct {
	Filter timeline.Filter
	Limit  int
}

type layoutMode int

const (
	layoutSplit layoutMode = iota
	layoutSingle
)

const splitMinWidth = 100

func (m Model) layout() layoutMode {
	if m.width >= splitMinWidth {
		return layoutSplit
	}
	return layoutSingle
}

type Model struct {
	deps Deps

	ctx         context.Context
	run         *queryRun
	queryToken  int
	loading     bool
	loadingMore bool
	interrupted bool
	err         error
	saveWarning string
	pageWarning string
	duplicates  int

	filter       timeline.Filter
	limit        int
	events       []event.Event
	truncated    bool
	cursorNext   string
	coveredSince time.Time
	coveredUntil time.Time
	sources      []source.Status
	limitations  []string

	selected int
	offset   int

	input        *filterState
	detail       *detailState
	detailToken  int
	detailCancel context.CancelFunc
	detailDone   chan struct{}

	search string

	styles *styles

	width  int
	height int
}

// queryRun is shared by pointer between the model and every command built from
// a copy of it, so a query started by Init can still be stopped and waited for.
type queryRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func New(deps Deps, opts Options) Model {
	return Model{
		deps:       deps,
		ctx:        context.Background(),
		run:        &queryRun{},
		filter:     opts.Filter,
		limit:      opts.Limit,
		queryToken: 1,
		loading:    true,
	}
}

func (m *Model) isEmpty() bool {
	return len(m.events) == 0 && m.err == nil && !m.loading
}

// cancelInFlight stops every query the model is waiting on, so leaving the TUI
// never leaves a goroutine reading the journal.
func (m *Model) cancelInFlight() {
	if m.run != nil && m.run.cancel != nil {
		m.run.cancel()
		m.run.cancel = nil
	}
	if m.detailCancel != nil {
		m.detailCancel()
		m.detailCancel = nil
	}
}

func (m Model) Init() tea.Cmd {
	// Init gets a copy of the model, so the query it starts records its cancel
	// function and completion in the shared queryRun, not on the copy.
	filter, limit, token := m.filter, m.limit, m.queryToken
	ctx, cancel := context.WithCancel(m.ctx)
	finished := make(chan struct{})
	m.run.cancel = cancel
	m.run.done = finished
	return func() tea.Msg {
		defer close(finished)
		res, err := timeline.Run(ctx, m.deps.Sources, filter, limit, "")
		return queryDoneMsg{token: token, res: res, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case queryDoneMsg:
		return m, m.applyQueryDone(msg)
	case detailDoneMsg:
		m.applyDetailDone(msg)
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.detail != nil {
		return m.handleDetailKey(msg)
	}
	if m.input != nil {
		switch m.input.mode {
		case modeWindow:
			return m.handleWindowKey(msg)
		case modeFilter:
			return m.handleFilterKey(msg)
		case modeKeyword:
			return m.handleKeywordKey(msg)
		}
	}
	switch msg.String() {
	case "q":
		m.cancelInFlight()
		return m, tea.Quit
	case "ctrl+c":
		m.cancelInFlight()
		m.interrupted = true
		return m, tea.Quit
	case "j", "down":
		if m.atBottom() {
			return m, m.maybeLoadMore()
		}
		m.moveSelection(1)
	case "k", "up":
		m.moveSelection(-1)
	case "g":
		m.moveSelection(-len(m.events))
	case "G":
		m.moveSelection(len(m.events))
	case "n":
		m.jumpMatch(1)
	case "N":
		m.jumpMatch(-1)
	case "/":
		return m.openKeywordInput(), nil
	case "f":
		return m.openFilterMenu(), nil
	case "t":
		return m.openWindowPicker(), nil
	case "r":
		return m, m.refresh()
	case "enter":
		return m, m.openDetail()
	}
	return m, nil
}

func (m Model) View() string {
	header := strings.Split(m.headerView(), "\n")
	footer := strings.Split(m.footerView(), "\n")
	body := strings.Split(m.bodyView(), "\n")

	rows := m.rows()
	if rows > 0 {
		if len(body) > rows {
			body = body[:rows]
		}
		for len(body) < rows {
			body = append(body, "")
		}
	}

	frame := make([]string, 0, len(header)+len(body)+len(footer))
	frame = append(frame, header...)
	frame = append(frame, body...)
	frame = append(frame, footer...)
	for i, line := range frame {
		frame[i] = truncate(line, m.width)
	}
	return strings.Join(frame, "\n")
}

func (m Model) atBottom() bool {
	return len(m.events) > 0 && m.selected >= len(m.events)-1
}

func (m Model) rows() int {
	if m.height <= 0 {
		return 0
	}
	spare := m.height - lineCount(m.headerView()) - lineCount(m.footerView())
	if spare < 1 {
		return 1
	}
	return spare
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// truncate cuts plain text to a visible width and marks the cut. It is ANSI
// aware, so a row that already carries colour loses cells, never half an escape
// sequence.
func truncate(s string, width int) string {
	if width <= 0 {
		return s
	}
	return ansi.Truncate(s, width, "...")
}

func padVisible(s string, width int) string {
	got := lipgloss.Width(s)
	if got >= width {
		return s
	}
	return s + strings.Repeat(" ", width-got)
}

func (m Model) bodyView() string {
	if m.detail != nil {
		if m.layout() == layoutSplit {
			return m.splitBody()
		}
		return m.detailView()
	}
	if m.input != nil && (m.input.mode == modeWindow || m.input.mode == modeFilter) {
		return m.pickerView()
	}
	switch {
	case m.err != nil:
		return m.errorView()
	case len(m.events) > 0:
		return m.timelineView()
	case m.loading:
		return loadingNote
	}
	return m.emptyState()
}

// splitBody puts the list beside the detail. The list takes the larger share so
// a timestamp, a unit and part of the message still fit; below that the detail
// keeps the 40 columns its own lines need.
func (m Model) splitBody() string {
	rows := m.rows()
	if rows <= 0 {
		return m.detailView()
	}
	leftWidth := m.width * 66 / 100
	if right := m.width - leftWidth - 3; right < 40 {
		leftWidth = m.width - 43
	}
	rightWidth := m.width - leftWidth - 3

	left := m.listRows(leftWidth, rows)
	right := m.detailRows(rightWidth, rows)

	out := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out = append(out, padVisible(l, leftWidth)+" | "+padVisible(r, rightWidth))
	}
	return strings.Join(out, "\n")
}

// shutdownWait bounds how long a quitting session waits for the query in
// flight to stop before reporting it as stuck.
const shutdownWait = 2 * time.Second

func Run(ctx context.Context, deps Deps, opts Options, in io.Reader, out io.Writer) error {
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	if err := diagnostics.CheckTerminal(os.Getenv("TERM"), isCharDevice(out)); err != nil {
		return err
	}
	model := New(deps, opts)
	model.ctx = ctx
	model.styles = newStyles(out)
	program := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx), tea.WithAltScreen())
	final, err := program.Run()
	if m, ok := final.(Model); ok {
		m.cancelInFlight()
		if m.run != nil && m.run.done != nil {
			select {
			case <-m.run.done:
			case <-time.After(shutdownWait):
				return errors.New("the query in flight did not stop in time")
			}
		}
		if m.detailDone != nil {
			select {
			case <-m.detailDone:
			case <-time.After(shutdownWait):
				return errors.New("the inspect in flight did not stop in time")
			}
		}
		if m.interrupted {
			return ErrInterrupted
		}
	}
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	if errors.Is(err, tea.ErrInterrupted) {
		return ErrInterrupted
	}
	return err
}

func isCharDevice(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
