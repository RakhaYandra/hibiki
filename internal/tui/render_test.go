package tui

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/inspect"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// Goldens are compared as plain frames, so the render tests run without a style
// set and every frame they read stays free of escape sequences.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update to write it)", err)
	}
	if got != string(want) {
		t.Errorf("frame differs from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func pinLocalUTC(t *testing.T) {
	t.Helper()
	previous := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = previous })
}

func renderEvents(n int) []event.Event {
	unit := "sshd.service"
	evs := manyFakeEvents(n)
	for i := range evs {
		evs[i].Service.Unit = &unit
		evs[i].Message = "Failed to start the unit"
		evs[i].Severity = event.SeverityError
	}
	return evs
}

func renderModel(width, height int, evs []event.Event) Model {
	now := time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)
	m := New(Deps{
		Sources:      []source.Source{newFakeSource(evs...)},
		Now:          func() time.Time { return now },
		DetailWindow: 2 * time.Minute,
	}, Options{Filter: timeline.Filter{Since: now.Add(-30 * time.Minute)}, Limit: 200})
	m.width, m.height = width, height
	m.loading = false
	m.events = evs
	m.sources = []source.Status{{Name: "journal", Available: true, Detail: "readable"}}
	if len(evs) > 0 {
		m.coveredSince = evs[0].Timestamp
		m.coveredUntil = evs[len(evs)-1].Timestamp
	}
	return m
}

func TestWideLayoutSplitsListAndDetail(t *testing.T) {
	pinLocalUTC(t)
	evs := renderEvents(6)
	m := renderModel(120, 40, evs)
	m.selected = 1

	opened, _ := press(t, m, "enter")
	if opened.layout() != layoutSplit {
		t.Fatalf("layout = %v, want the split layout at 120 columns", opened.layout())
	}

	correlated, incs := inspect.Correlate(inspect.Context{
		Event:  evs[1],
		Before: evs[:1],
		After:  evs[2:3],
	})
	withContext, _ := opened.Update(detailDoneMsg{
		token: opened.detail.token,
		ctx:   correlated,
		incs:  incs,
	})
	frame := withContext.(Model).View()

	if !strings.Contains(frame, "event "+evs[1].ID) {
		t.Errorf("frame = %q, want the detail pane", frame)
	}
	if !strings.Contains(frame, evs[0].Message) {
		t.Errorf("frame = %q, want the list pane beside it", frame)
	}
	if !strings.Contains(frame, " | ") {
		t.Errorf("frame = %q, want the two panes separated", frame)
	}
	golden(t, "wide-split", frame)
}

func TestSingleColumnLayoutUsesTheFullHeight(t *testing.T) {
	pinLocalUTC(t)
	m := renderModel(80, 24, renderEvents(50))

	if m.layout() != layoutSingle {
		t.Fatalf("layout = %v, want the single column layout at 80 columns", m.layout())
	}
	if got := len(m.visibleRows()); got != m.rows() {
		t.Fatalf("rows = %d, want all %d the frame has", got, m.rows())
	}

	frame := m.View()
	lines := strings.Split(frame, "\n")
	if len(lines) != 24 {
		t.Fatalf("frame lines = %d, want it to fill the 24 row terminal", len(lines))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > 80 {
			t.Fatalf("line %d is %d wide, want at most 80", i, w)
		}
	}
	golden(t, "single-80x24", frame)
}

func TestFortyColumnsTruncatesWithAnEllipsis(t *testing.T) {
	pinLocalUTC(t)
	long := strings.Repeat("a-very-long-unit-name-", 6) + "end"
	evs := renderEvents(3)
	for i := range evs {
		evs[i].Service.Unit = &long
	}

	m := renderModel(40, 20, evs)
	rows := m.listRows(m.width, m.rows())
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want the 3 events", len(rows))
	}
	for i, row := range rows {
		if w := lipgloss.Width(row); w > 40 {
			t.Fatalf("row %d is %d wide, want at most 40: %q", i, w, row)
		}
		if !strings.HasSuffix(row, "...") {
			t.Fatalf("row %d = %q, want the cut marked", i, row)
		}
	}
	golden(t, "narrow-40x20", m.View())
}

func TestTruncateNeverSplitsAnEscapeSequence(t *testing.T) {
	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetColorProfile(termenv.TrueColor)
	styled := renderer.NewStyle().Bold(true).Render(strings.Repeat("x", 50))
	if !strings.Contains(styled, "\x1b[") {
		t.Fatal("the fixture carries no escape sequence, so the test would prove nothing")
	}

	got := truncate(styled, 10)
	if w := lipgloss.Width(got); w != 10 {
		t.Fatalf("width = %d, want exactly 10", w)
	}
	if n := strings.Count(got, "\x1b["); n == 0 || n%2 != 0 {
		t.Fatalf("escapes = %d, want a balanced pair left: %q", n, got)
	}
	if !strings.HasSuffix(stripANSI(got), "...") {
		t.Fatalf("got = %q, want the cut marked", got)
	}
}

func TestEmptyErrorAndLoadingGoldens(t *testing.T) {
	pinLocalUTC(t)

	empty := renderModel(80, 24, nil)
	golden(t, "empty-80x24", empty.View())

	failed := renderModel(80, 24, nil)
	failed.err = exit.SourceUnavailable("journal is not readable")
	golden(t, "error-80x24", failed.View())

	loading := renderModel(80, 24, nil)
	loading.loading = true
	golden(t, "loading-80x24", loading.View())
}

func TestResizeDuringADetailQueryReflows(t *testing.T) {
	pinLocalUTC(t)
	evs := renderEvents(6)
	m := renderModel(80, 24, evs)
	m.selected = 1

	opened, cmd := press(t, m, "enter")
	if opened.detail == nil || !opened.detail.loading {
		t.Fatal("want a detail query in flight")
	}

	wide, _ := opened.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if wide.(Model).layout() != layoutSplit {
		t.Fatal("want the split layout at 120 columns")
	}
	if strings.TrimSpace(wide.(Model).View()) == "" {
		t.Fatal("the wide frame is empty")
	}

	narrow, _ := wide.(Model).Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	if narrow.(Model).layout() != layoutSingle {
		t.Fatal("want the single column layout at 60 columns")
	}

	if cmd == nil {
		t.Fatal("cmd = nil, want the context query")
	}
	done, _ := narrow.(Model).Update(cmd())
	final := done.(Model)

	frame := final.View()
	if !strings.Contains(frame, "event "+evs[1].ID) {
		t.Fatalf("frame = %q, want the detail of the event the pane was opened on", frame)
	}
	if strings.Contains(frame, " | ") {
		t.Errorf("frame = %q, want no side by side pane at 60 columns", frame)
	}
	golden(t, "narrow-detail", frame)
}

func TestALongFieldDetailGolden(t *testing.T) {
	pinLocalUTC(t)
	ev := rawEvent(161)
	m := renderModel(80, 24, []event.Event{ev})

	opened, _ := press(t, m, "enter")
	for i := 0; i < 4; i++ {
		opened, _ = press(t, opened, "down")
	}
	if opened.detail.offset != 4 {
		t.Fatalf("offset = %d, want the 4 lines scrolled", opened.detail.offset)
	}
	golden(t, "long-detail", opened.View())
}
