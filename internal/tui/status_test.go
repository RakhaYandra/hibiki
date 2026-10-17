package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

func TestSourceChipsShowAvailabilityAndReason(t *testing.T) {
	got := sourceChips([]source.Status{
		{Name: "journal", Available: true, Detail: "readable"},
		{Name: "file", Detail: "Permission denied"},
	})
	if !strings.Contains(got, "journal ok") {
		t.Errorf("chips = %q, want a readable source to say ok", got)
	}
	if !strings.Contains(got, "file unavailable: Permission denied") {
		t.Errorf("chips = %q, want the reason next to the unavailable source", got)
	}
}

func TestALimitationIsVisibleNextToAGreenChip(t *testing.T) {
	m := New(Deps{}, Options{})
	m.sources = []source.Status{{Name: "journal", Available: true, Detail: "readable"}}
	m.limitations = []string{"malformed_records:3"}

	header := m.headerView()
	if !strings.Contains(header, "journal ok") {
		t.Fatalf("header = %q, want the readable chip", header)
	}
	if !strings.Contains(header, "malformed_records:3") {
		t.Errorf("header = %q, want the limitation next to the ok chip: an incomplete list must not read as complete", header)
	}
}

func TestTruncationHintAppearsOnlyWhenTruncated(t *testing.T) {
	m := New(Deps{}, Options{})
	m.coveredSince = time.Date(2026, 10, 16, 7, 0, 0, 0, time.UTC)
	m.coveredUntil = time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	m.truncated = true
	m.cursorNext = "cursor-1"
	if !strings.Contains(m.footerView(), moreEventsNote) {
		t.Errorf("footer = %q, want the more events hint while a cursor is left", m.footerView())
	}

	m.truncated = false
	if strings.Contains(m.footerView(), moreEventsNote) {
		t.Errorf("footer = %q, want no hint once the list is complete", m.footerView())
	}
}

func TestCoverageLineRendersBothEndsAndTheEmptyCase(t *testing.T) {
	m := New(Deps{}, Options{})
	if got := m.coverageLine(); got != noCoverageNote {
		t.Fatalf("coverage = %q, want %q while nothing was observed", got, noCoverageNote)
	}

	m.coveredSince = time.Date(2026, 10, 16, 7, 0, 0, 0, time.UTC)
	m.coveredUntil = time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	want := "covered: 2026-10-16T07:00:00Z .. 2026-10-16T08:00:00Z"
	if got := m.coverageLine(); got != want {
		t.Fatalf("coverage = %q, want %q", got, want)
	}
}

func TestLoadingAndErrorReplaceTheFooter(t *testing.T) {
	loading := New(Deps{}, Options{})
	loading.loading = true
	if n := strings.Count(loading.View(), loadingNote); n != 1 {
		t.Errorf("loading note appears %d times, want exactly one", n)
	}

	refreshing := New(Deps{}, Options{})
	refreshing.loading = true
	refreshing.events = manyFakeEvents(3)
	if !strings.Contains(refreshing.footerView(), loadingNote) {
		t.Errorf("footer = %q, want the loading note while a list is already on screen", refreshing.footerView())
	}

	failed := New(Deps{}, Options{})
	failed.events = manyFakeEvents(3)
	failed.err = exit.SourceUnavailable("journal is not readable")
	view := failed.View()
	if !strings.Contains(view, "journal is not readable") {
		t.Errorf("view = %q, want the reason", view)
	}
	if !strings.Contains(view, "exit code 3 (source unavailable)") {
		t.Errorf("view = %q, want the code and what it means", view)
	}
	if strings.Contains(view, failed.rowText(failed.events[0])) {
		t.Errorf("view = %q, want no stale rows once the query failed", view)
	}

	empty := New(Deps{}, Options{})
	empty.loading = false
	empty.sources = []source.Status{{Name: "journal", Available: true, Detail: "readable"}}
	if !strings.Contains(empty.View(), emptyNote) {
		t.Errorf("view = %q, want the empty window note", empty.View())
	}
}

func TestWindowAndFilterChipsReflectTheActiveFilter(t *testing.T) {
	since := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	m := New(Deps{}, Options{Filter: timeline.Filter{
		Since:      since,
		Severities: []event.Severity{event.SeverityError, event.SeverityCritical},
	}})

	header := m.headerView()
	if !strings.Contains(header, "2026-10-16T00:00:00Z") {
		t.Errorf("header = %q, want the window start", header)
	}
	if !strings.Contains(header, "error,critical") {
		t.Errorf("header = %q, want both severities as the CLI writes them", header)
	}
}
