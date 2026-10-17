package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

func windowModel(now time.Time, since time.Time) Model {
	return New(Deps{
		Sources: []source.Source{},
		Now:     func() time.Time { return now },
	}, Options{Filter: timeline.Filter{Since: since}, Limit: 200})
}

func TestWindowPickerOffersThePresets(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)
	m := windowModel(now, now.Add(-30*time.Minute))

	m = m.openWindowPicker()
	if m.input == nil || m.input.mode != modeWindow {
		t.Fatalf("input = %+v, want the window picker open", m.input)
	}
	got := strings.Join(m.input.presets, ",")
	want := "5m,30m,1h,6h,24h,custom"
	if got != want {
		t.Fatalf("presets = %q, want %q", got, want)
	}
	if m.input.choice != 1 {
		t.Errorf("choice = %d, want the preset the window came from (30m)", m.input.choice)
	}
}

func TestChoosingAPresetRecomputesSinceFromOneNow(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)
	m := windowModel(now, now.Add(-5*time.Minute))

	pressed, _ := press(t, m, "t", "down")
	if pressed.input == nil || pressed.input.choice != 1 {
		t.Fatalf("choice = %+v, want the second preset", pressed.input)
	}
	applied, cmd := press(t, pressed, "enter")
	if applied.input != nil {
		t.Errorf("input = %+v, want the picker closed once a preset is chosen", applied.input)
	}
	if want := now.Add(-30 * time.Minute).UTC(); !applied.filter.Since.Equal(want) {
		t.Fatalf("since = %s, want %s", applied.filter.Since, want)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want a query to be started")
	}
}

func TestRefreshKeepsTheFilterAndRestartsFromTheTop(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)
	fake := newFakeSource(manyFakeEvents(3)...)
	m := New(Deps{Sources: []source.Source{fake}, Now: func() time.Time { return now }},
		Options{Filter: timeline.Filter{Since: now.Add(-30 * time.Minute)}, Limit: 2})
	since := m.filter.Since

	m.events = manyFakeEvents(1)
	m.selected = 0
	m.offset = 0
	m.truncated = true
	m.cursorNext = "cursor-past-the-first-page"

	pressed, cmd := press(t, m, "r")
	if cmd == nil {
		t.Fatal("cmd = nil, want refresh to start a query")
	}
	if !pressed.filter.Since.Equal(since) {
		t.Fatalf("since = %s, want the filter kept", pressed.filter.Since)
	}
	if pressed.selected != 0 || pressed.offset != 0 {
		t.Fatalf("selected = %d offset = %d, want the first event", pressed.selected, pressed.offset)
	}

	model, _ := pressed.Update(cmd())
	got := model.(Model)
	if len(got.events) == 0 || got.events[0].ID != "evt-000" {
		t.Fatalf("events = %+v, want the list to start at the first event again", got.events)
	}
	if got.cursorNext == "cursor-past-the-first-page" {
		t.Error("the stale cursor survived the refresh")
	}
}

func TestEscLeavesTheWindowUnchanged(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)
	since := now.Add(-6 * time.Hour)
	m := windowModel(now, since)

	pressed, cmd := press(t, m, "t", "down", "esc")
	if pressed.input != nil {
		t.Errorf("input = %+v, want the picker closed", pressed.input)
	}
	if !pressed.filter.Since.Equal(since) {
		t.Fatalf("since = %s, want %s untouched", pressed.filter.Since, since)
	}
	if cmd != nil {
		t.Error("cmd != nil, want no query after cancelling")
	}
}

func TestACustomWindowAcceptsRFC3339AndRejectsGarbage(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)
	m := windowModel(now, now.Add(-30*time.Minute))

	pressed, _ := press(t, m, "t")
	for pressed.input.choice != customIndex() {
		pressed, _ = press(t, pressed, "down")
	}

	typed, _ := press(t, pressed, "2", "0", "2", "6", "-", "1", "0", "-", "1", "6", "T", "0", "8", ":", "0", "0", ":", "0", "0", "Z")
	applied, _ := press(t, typed, "enter")
	want := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	if !applied.filter.Since.Equal(want) {
		t.Fatalf("since = %s, want %s", applied.filter.Since, want)
	}
	if applied.input != nil {
		t.Errorf("input = %+v, want the picker closed after a valid stamp", applied.input)
	}

	garbage, _ := press(t, m, "t")
	for garbage.input.choice != customIndex() {
		garbage, _ = press(t, garbage, "down")
	}
	garbage, _ = press(t, garbage, "y", "e", "s", "t", "e", "r", "d", "a", "y", "i", "s", "h")
	garbage, _ = press(t, garbage, "enter")
	if garbage.input == nil {
		t.Fatal("input = nil, want the picker to stay open on a parse error")
	}
	if !strings.Contains(garbage.footerView(), "invalid time") {
		t.Errorf("footer = %q, want the parse error", garbage.footerView())
	}
	if !garbage.filter.Since.Equal(now.Add(-30 * time.Minute)) {
		t.Errorf("since = %s, want the window unchanged after a bad entry", garbage.filter.Since)
	}
}

func searchEvents() []event.Event {
	first := manyFakeEvents(1)[0]
	first.ID = "evt-000"
	first.Message = "error: unit failed"

	second := manyFakeEvents(1)[0]
	second.ID = "evt-001"
	second.Message = "started cleanly"

	third := manyFakeEvents(1)[0]
	third.ID = "evt-002"
	third.Message = "another Error upstream"

	third.Timestamp = third.Timestamp.Add(2 * time.Second)
	third.TimestampUnixUS = third.Timestamp.UnixMicro()
	second.Timestamp = second.Timestamp.Add(time.Second)
	second.TimestampUnixUS = second.Timestamp.UnixMicro()
	return []event.Event{first, second, third}
}

func searchModel() Model {
	m := New(Deps{Sources: []source.Source{newFakeSource()}}, Options{Limit: 200})
	m.loading = false
	return m
}

func TestKeywordSearchMarksMatchesWithoutFilteringTheList(t *testing.T) {
	m := searchModel()
	m.events = searchEvents()

	typed, _ := press(t, m, "/", "e", "r", "r", "enter")
	if typed.search != "err" {
		t.Fatalf("search = %q, want the typed term", typed.search)
	}
	if len(typed.events) != 3 {
		t.Fatalf("events = %d, want the list untouched: search marks, it does not filter", len(typed.events))
	}
	for i, want := range []bool{true, false, true} {
		if got := typed.matches(typed.events[i]); got != want {
			t.Errorf("matches(event %d) = %v, want %v", i, got, want)
		}
	}
}

func TestJumpMatchWrapsInBothDirections(t *testing.T) {
	m := searchModel()
	m.events = searchEvents()
	m.search = "err"

	m.selected = 0
	m.jumpMatch(1)
	if m.selected != 2 {
		t.Fatalf("selected = %d, want the next match at 2", m.selected)
	}
	m.jumpMatch(1)
	if m.selected != 0 {
		t.Fatalf("selected = %d, want the search to wrap to 0", m.selected)
	}
	m.jumpMatch(-1)
	if m.selected != 2 {
		t.Fatalf("selected = %d, want the search to wrap backwards to 2", m.selected)
	}
}

func TestAnEmptySearchClearsAndDisablesJumping(t *testing.T) {
	m := searchModel()
	m.events = searchEvents()
	m.search = "err"
	m.selected = 1

	cleared, _ := press(t, m, "/", "enter")
	if cleared.search != "" {
		t.Fatalf("search = %q, want it cleared", cleared.search)
	}
	if cleared.matches(cleared.events[0]) {
		t.Error("matches = true, want an empty search to mark nothing")
	}

	unchanged, _ := press(t, cleared, "n", "N")
	if unchanged.selected != 1 {
		t.Fatalf("selected = %d, want jumping disabled while nothing is searched", unchanged.selected)
	}
}

func TestASearchWithNoMatchesSaysSo(t *testing.T) {
	m := searchModel()
	m.events = searchEvents()

	typed, _ := press(t, m, "/", "z", "z", "z", "enter")
	if len(typed.events) != 3 {
		t.Fatalf("events = %d, want the list kept", len(typed.events))
	}
	if !strings.Contains(typed.footerView(), noMatchNote) {
		t.Fatalf("footer = %q, want %q", typed.footerView(), noMatchNote)
	}
}

func TestTheFilterMenuChangesTheQuery(t *testing.T) {
	events := searchEvents()
	events[1].Severity = event.SeverityInfo
	fake := newFakeSource(events...)
	m := New(Deps{Sources: []source.Source{fake}}, Options{Limit: 200})

	opened, _ := press(t, m, "f")
	if opened.input == nil || opened.input.mode != modeFilter {
		t.Fatalf("input = %+v, want the filter menu open on severity", opened.input)
	}
	if opened.input.fields[opened.input.choice].label != "severity" {
		t.Fatalf("label = %q, want severity first", opened.input.fields[opened.input.choice].label)
	}

	applied, cmd := press(t, opened, "e", "r", "r", "o", "r", ",", "c", "r", "i", "t", "i", "c", "a", "l", "enter")
	if applied.input != nil {
		t.Errorf("input = %+v, want the menu closed", applied.input)
	}
	want := []event.Severity{event.SeverityError, event.SeverityCritical}
	if len(applied.filter.Severities) != len(want) {
		t.Fatalf("severities = %v, want %v", applied.filter.Severities, want)
	}
	for i := range want {
		if applied.filter.Severities[i] != want[i] {
			t.Fatalf("severities = %v, want %v", applied.filter.Severities, want)
		}
	}
	if !strings.Contains(applied.headerView(), "error,critical") {
		t.Errorf("header = %q, want the filter the query will use", applied.headerView())
	}

	if cmd == nil {
		t.Fatal("cmd = nil, want a query")
	}
	model, _ := applied.Update(cmd())
	got := model.(Model)
	if len(got.events) != 2 || got.events[1].ID == "evt-001" {
		t.Fatalf("events = %+v, want the info event left out", got.events)
	}
}

func TestTheFilterMenuRejectsAnUnknownSeverity(t *testing.T) {
	m := New(Deps{Sources: []source.Source{newFakeSource()}}, Options{Limit: 200})

	opened, _ := press(t, m, "f")
	applied, cmd := press(t, opened, "l", "o", "u", "d", "enter")
	if applied.input == nil {
		t.Fatal("input = nil, want the menu to stay open on a bad value")
	}
	if !strings.Contains(applied.footerView(), "unknown severity") {
		t.Errorf("footer = %q, want the parse error", applied.footerView())
	}
	if cmd != nil {
		t.Error("cmd != nil, want no query while the value is invalid")
	}
	if len(applied.filter.Severities) != 0 {
		t.Errorf("severities = %v, want the filter unchanged", applied.filter.Severities)
	}
}

func TestClearingTheFilterMenuRestoresTheWholeWindow(t *testing.T) {
	fake := newFakeSource(searchEvents()...)
	window := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	base := timeline.Filter{
		Since:      window,
		Severities: []event.Severity{event.SeverityCritical},
		Categories: []event.Category{event.CategorySecurity},
		Units:      []string{"sshd.service"},
		Keyword:    "zzz",
	}
	m := New(Deps{Sources: []source.Source{fake}}, Options{Filter: base, Limit: 200})

	opened, _ := press(t, m, "f")
	cleared := opened
	for _, label := range []string{"severity", "category", "unit", "keyword"} {
		for cleared.input.fields[cleared.input.choice].label != label {
			cleared, _ = press(t, cleared, "down")
		}
		typed := len(cleared.input.input.Value())
		for i := 0; i < typed; i++ {
			cleared, _ = press(t, cleared, "backspace")
		}
	}
	applied, cmd := press(t, cleared, "enter")

	if len(applied.filter.Severities) != 0 || len(applied.filter.Categories) != 0 ||
		len(applied.filter.Units) != 0 || applied.filter.Keyword != "" {
		t.Fatalf("filter = %+v, want every detail field cleared", applied.filter)
	}
	if !applied.filter.Since.Equal(window) {
		t.Errorf("since = %s, want %s kept: the menu edits details, not the window", applied.filter.Since, window)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want a query")
	}
	model, _ := applied.Update(cmd())
	if got := model.(Model); len(got.events) != 3 {
		t.Fatalf("events = %d, want the whole window back", len(got.events))
	}
}
