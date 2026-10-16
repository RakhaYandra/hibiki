package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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

type Model struct {
	deps Deps

	ctx         context.Context
	cancel      context.CancelFunc
	queryToken  int
	loading     bool
	err         error
	saveWarning string

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

	input *filterState

	search string

	styles *styles

	width  int
	height int
}

func New(deps Deps, opts Options) Model {
	return Model{
		deps:   deps,
		ctx:    context.Background(),
		filter: opts.Filter,
		limit:  opts.Limit,
	}
}

func (m *Model) isEmpty() bool {
	return len(m.events) == 0 && m.err == nil && !m.loading
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case queryDoneMsg:
		return m, m.applyQueryDone(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
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
	}
	return m, nil
}

func (m Model) View() string {
	return strings.Join([]string{m.headerView(), m.bodyView(), m.footerView()}, "\n")
}

func (m Model) bodyView() string {
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

func Run(ctx context.Context, deps Deps, opts Options, in io.Reader, out io.Writer) error {
	if err := diagnostics.CheckTerminal(os.Getenv("TERM"), isCharDevice(out)); err != nil {
		return err
	}
	model := New(deps, opts)
	model.ctx = ctx
	model.styles = newStyles(out)
	program := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx))
	_, err := program.Run()
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
