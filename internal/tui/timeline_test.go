package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
)

func TestSelectionClampsAtBothEnds(t *testing.T) {
	m := New(Deps{}, Options{})
	m.events = manyFakeEvents(3)

	m.moveSelection(-1)
	if m.selected != 0 {
		t.Fatalf("selected = %d, want 0: there is nothing above the first event", m.selected)
	}

	m.selected = 2
	m.moveSelection(1)
	if m.selected != 2 {
		t.Fatalf("selected = %d, want 2: there is nothing below the last event", m.selected)
	}
}

func TestOffsetFollowsTheSelection(t *testing.T) {
	m := New(Deps{}, Options{})
	m.height = 10
	m.events = manyFakeEvents(100)

	m.selected = 50
	m.ensureVisible()
	if m.offset > m.selected || m.selected >= m.offset+10 {
		t.Fatalf("offset = %d, selected = %d, want the selection inside the window", m.offset, m.selected)
	}

	m.selected = 0
	m.ensureVisible()
	if m.offset != 0 {
		t.Fatalf("offset = %d, want 0: the first event scrolls back to the top", m.offset)
	}
}

func TestRowTextCarriesTheDecisionFields(t *testing.T) {
	unit := "sshd.service"
	stamp := time.Date(2026, 10, 16, 8, 30, 0, 500000000, time.UTC)
	ev := event.Event{
		Timestamp:       stamp,
		TimestampUnixUS: stamp.UnixMicro(),
		Category:        event.CategoryService,
		Severity:        event.SeverityError,
		Message:         "Failed to start",
		Service:         event.Service{Unit: &unit},
	}

	m := New(Deps{}, Options{})
	row := m.rowText(ev)
	for _, want := range []string{stamp.Local().Format("15:04:05.000"), "error", "service", "sshd.service", "Failed to start"} {
		if !strings.Contains(row, want) {
			t.Errorf("row %q is missing %q", row, want)
		}
	}

	unknown := m.rowText(event.Event{})
	if !strings.HasPrefix(unknown, "unknown") {
		t.Errorf("row %q must say the time is unknown instead of printing the zero time", unknown)
	}
}

func TestVisibleRowsNeverExceedsTheHeight(t *testing.T) {
	m := New(Deps{}, Options{})
	m.height = 10
	m.events = manyFakeEvents(100)
	m.selected = 42
	m.ensureVisible()

	rows := m.visibleRows()
	if len(rows) > 10 {
		t.Fatalf("rows = %d, want at most the 10 rows the terminal has", len(rows))
	}
	for i, row := range rows {
		ev := m.events[m.offset+i]
		if !strings.Contains(row, m.rowText(ev)) {
			t.Fatalf("row %d does not belong to event %d", i, m.offset+i)
		}
	}
	if !strings.HasPrefix(rows[m.selected-m.offset], ">") {
		t.Error("the selected row must carry the cursor")
	}
}

func TestRowStartsWithTheLocalTime(t *testing.T) {
	ev := manyFakeEvents(1)[0]
	m := New(Deps{}, Options{})
	want := ev.Timestamp.Local().Format("15:04:05.000")
	if row := m.rowText(ev); !strings.HasPrefix(row, want) {
		t.Fatalf("row %q must start with %q so the left edge scans as time", row, want)
	}
}
