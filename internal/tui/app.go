package tui

import (
	"context"
	"errors"
	"io"
	"os"
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
	opts Options

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

	width  int
	height int
}

func New(deps Deps, opts Options) Model {
	return Model{
		deps:   deps,
		opts:   opts,
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
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	return "HIBIKI / SYSTEM TIMELINE\nwindow since " + m.opts.Filter.Since.Format(time.RFC3339) + "\n"
}

func Run(ctx context.Context, deps Deps, opts Options, in io.Reader, out io.Writer) error {
	if err := diagnostics.CheckTerminal(os.Getenv("TERM"), isCharDevice(out)); err != nil {
		return err
	}
	model := New(deps, opts)
	model.ctx = ctx
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
