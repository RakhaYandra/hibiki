package tui

import (
	"strings"
	"testing"
	"time"

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
