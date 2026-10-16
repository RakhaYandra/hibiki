package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RakhaYandra/hibiki/internal/diagnostics"
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
	deps   Deps
	opts   Options
	width  int
	height int
}

func New(deps Deps, opts Options) Model { return Model{deps: deps, opts: opts} }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
	program := tea.NewProgram(New(deps, opts), tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx))
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
