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
	cancel      context.CancelFunc
	queryToken  int
	loading     bool
	loadingMore bool
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
	case "q", "ctrl+c":
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
