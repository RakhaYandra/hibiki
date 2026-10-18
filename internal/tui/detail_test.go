package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/inspect"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/source/journal"
)

func detailModel(t *testing.T, fake *fakeSource, window time.Duration, evs []event.Event) Model {
	t.Helper()
	m := New(Deps{
		Sources:      []source.Source{fake},
		Now:          func() time.Time { return time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC) },
		DetailWindow: window,
	}, Options{Limit: 200})
	m.events = evs
	m.height = 20
	return m
}

func rawEvent(n int) event.Event {
	ev := manyFakeEvents(1)[0]
	ev.ID = "evt-raw"
	raw := map[string]string{
		"MESSAGE":  "the message",
		"_PID":     "4242",
		"_BOOT_ID": "30107f27d01e4e42b24846e4da81db1a",
	}
	for i := 0; i < n; i++ {
		raw[fmt.Sprintf("F%03d", i)] = fmt.Sprintf("value %d", i)
	}
	ev.Raw = raw
	return ev
}

// The crash-loop fixture is read through the journal adapter so the detail pane
// correlates the same records a journal would carry.
func crashLoopEvents(t *testing.T) []event.Event {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "test", "fixtures", "regression", "crash-loop.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	evs := []event.Event{}
	if err := journal.Decode(f, func(fields map[string]string) error {
		ev, err := journal.Normalize(fields)
		if err != nil {
			return err
		}
		evs = append(evs, ev)
		return nil
	}, func(line int, err error) {
		t.Errorf("fixture line %d: %v", line, err)
	}); err != nil {
		t.Fatal(err)
	}
	return evs
}

func openLoadedDetail(t *testing.T, m Model) Model {
	t.Helper()
	opened, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Fatal("cmd = nil, want the context query to run")
	}
	model, _ := opened.Update(cmd())
	loaded, ok := model.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", model)
	}
	return loaded
}

func crashLoopDetail(t *testing.T, cursor string) Model {
	t.Helper()
	evs := crashLoopEvents(t)
	selected := -1
	for i, ev := range evs {
		if ev.Source.Cursor != nil && *ev.Source.Cursor == cursor {
			selected = i
		}
	}
	if selected < 0 {
		t.Fatalf("the fixture has no event at cursor %s", cursor)
	}
	m := detailModel(t, newFakeSource(evs...), 2*time.Minute, evs)
	m.selected = selected
	return openLoadedDetail(t, m)
}

func TestDetailListsTheIncidentOfTheEvent(t *testing.T) {
	loaded := crashLoopDetail(t, "s=cl;i=3")

	var correlations []string
	for _, line := range loaded.detailLines() {
		if strings.HasPrefix(line, "correlation ") {
			correlations = append(correlations, line)
		}
	}
	if len(correlations) != 3 {
		t.Fatalf("correlation lines = %d, want one per incident of the event:\n%s", len(correlations), strings.Join(loaded.detailLines(), "\n"))
	}
	for _, want := range []string{"restart-loop [high]", "failed-then-error [medium]", "pid-errors [low]"} {
		if !strings.Contains(strings.Join(correlations, "\n"), want) {
			t.Errorf("correlation lines lost %q:\n%s", want, strings.Join(correlations, "\n"))
		}
	}
	for i, line := range correlations {
		if !strings.HasPrefix(line, "correlation inc_") {
			t.Errorf("line %d = %q, want the incident id first", i, line)
		}
	}
}

func TestDetailExplainsEachIncidentOnItsOwnLine(t *testing.T) {
	lines := crashLoopDetail(t, "s=cl;i=3").detailLines()
	explained := 0
	for i, line := range lines {
		if !strings.HasPrefix(line, "correlation ") {
			continue
		}
		if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "  ") || !strings.Contains(lines[i+1], "within") {
			t.Fatalf("line %d = %q has no indented reason after it:\n%s", i, line, strings.Join(lines, "\n"))
		}
		explained++
	}
	if explained != 3 {
		t.Fatalf("explained = %d, want a reason for every incident:\n%s", explained, strings.Join(lines, "\n"))
	}
}

func TestDetailReportsOnlyTheIncidentsOfTheEvent(t *testing.T) {
	lines := crashLoopDetail(t, "s=cl;i=1").detailLines()
	var correlations []string
	for _, line := range lines {
		if strings.HasPrefix(line, "correlation ") {
			correlations = append(correlations, line)
		}
	}
	if len(correlations) != 1 || !strings.Contains(correlations[0], "restart-loop [high]") {
		t.Fatalf("correlation lines = %v, want only the incident the first record takes part in", correlations)
	}
}

func TestDetailShowsTheEvidenceBeforeTheIncidents(t *testing.T) {
	lines := crashLoopDetail(t, "s=cl;i=3").detailLines()
	after, correlation := -1, -1
	for i, line := range lines {
		if after < 0 && strings.HasPrefix(line, "after (") {
			after = i
		}
		if correlation < 0 && strings.HasPrefix(line, "correlation ") {
			correlation = i
		}
	}
	if after < 0 || correlation < 0 || correlation < after {
		t.Fatalf("after = %d, correlation = %d, want the evidence rows first:\n%s", after, correlation, strings.Join(lines, "\n"))
	}
}

func TestDetailShowsNoCorrelationLineWhenNoRuleMatches(t *testing.T) {
	single := manyFakeEvents(1)
	loaded := openLoadedDetail(t, detailModel(t, newFakeSource(single...), 2*time.Minute, single))

	for _, line := range loaded.detailLines() {
		if strings.HasPrefix(line, "correlation ") {
			t.Errorf("line = %q, want no correlation line at all:\n%s", line, strings.Join(loaded.detailLines(), "\n"))
		}
	}
}

func TestOpenDetailUsesTheConfiguredContextWindow(t *testing.T) {
	fake := newFakeSource()
	ev := manyFakeEvents(1)[0]
	m := detailModel(t, fake, 2*time.Minute, []event.Event{ev})

	opened, cmd := press(t, m, "enter")
	if opened.detail == nil {
		t.Fatal("detail = nil, want the pane open on the selected event")
	}
	if !opened.detail.loading {
		t.Error("loading = false, want the pane to say it is fetching")
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want the context query to run")
	}

	model, _ := opened.Update(cmd())
	done := model.(Model)
	if done.detail.ctx == nil {
		t.Fatal("ctx = nil, want the surrounding window")
	}
	if len(done.detail.ctx.Limitations) == 0 && len(fake.calls()) != 2 {
		t.Fatalf("queries = %d, want one before and one after the event", len(fake.calls()))
	}
	for i, q := range fake.calls() {
		if span := q.Until.Sub(q.Since); span != 2*time.Minute {
			t.Errorf("query %d spans %s, want the configured 2m window", i, span)
		}
	}
}

func TestDetailListsRawFieldsInSortedOrder(t *testing.T) {
	fake := newFakeSource()
	ev := rawEvent(5)
	m := detailModel(t, fake, 2*time.Minute, []event.Event{ev})

	opened, _ := press(t, m, "enter")
	lines := opened.detailLines()

	var fieldLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "  ") && strings.Contains(line, "=") {
			fieldLines = append(fieldLines, line)
		}
	}
	if len(fieldLines) != len(ev.Raw) {
		t.Fatalf("field lines = %d, want every one of the %d raw fields", len(fieldLines), len(ev.Raw))
	}
	keys := make([]string, 0, len(ev.Raw))
	for k := range ev.Raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, key := range keys {
		if !strings.HasPrefix(fieldLines[i], "  "+key+"=") {
			t.Fatalf("line %d = %q, want %q in sorted order", i, fieldLines[i], key)
		}
	}
	view := strings.Join(lines, "\n")
	if !strings.Contains(view, "the message") {
		t.Errorf("view = %q, want the raw value verbatim", view)
	}
}

func TestEscClosesDetailAndKeepsTheSelection(t *testing.T) {
	fake := newFakeSource()
	m := detailModel(t, fake, 2*time.Minute, manyFakeEvents(5))
	m.selected = 3

	opened, _ := press(t, m, "enter")
	closed, _ := press(t, opened, "esc")
	if closed.detail != nil {
		t.Fatalf("detail = %+v, want it closed", closed.detail)
	}
	if closed.selected != 3 {
		t.Fatalf("selected = %d, want the list where it was", closed.selected)
	}
}

func TestALongFieldListScrollsAndClamps(t *testing.T) {
	fake := newFakeSource()
	ev := rawEvent(161)
	m := detailModel(t, fake, 2*time.Minute, []event.Event{ev})

	opened, _ := press(t, m, "enter")
	if total := len(opened.detailLines()); total <= opened.rows() {
		t.Fatalf("lines = %d, want more than the %d rows so scrolling matters", total, opened.rows())
	}

	down := opened
	for i := 0; i < 400; i++ {
		down, _ = press(t, down, "down")
	}
	max := len(down.detailLines()) - down.rows()
	if down.detail.offset != max {
		t.Fatalf("offset = %d, want the maximum %d", down.detail.offset, max)
	}

	up := down
	for i := 0; i < 400; i++ {
		up, _ = press(t, up, "up")
	}
	if up.detail.offset != 0 {
		t.Fatalf("offset = %d, want 0 at the top", up.detail.offset)
	}
}

func TestAFailedDetailQueryKeepsTheKnownEvent(t *testing.T) {
	fake := newFakeSource()
	ev := rawEvent(5)
	m := detailModel(t, fake, 2*time.Minute, []event.Event{ev})

	opened, _ := press(t, m, "enter")
	model, _ := opened.Update(detailDoneMsg{
		token: opened.detail.token,
		err:   errors.New("journal is not readable"),
	})
	failed := model.(Model)

	view := strings.Join(failed.detailLines(), "\n")
	if !strings.Contains(view, "journal is not readable") {
		t.Errorf("view = %q, want the failure", view)
	}
	if !strings.Contains(view, ev.ID) {
		t.Errorf("view = %q, want the event that was already in hand", view)
	}
	if !strings.Contains(view, "_BOOT_ID=30107f27d01e4e42b24846e4da81db1a") {
		t.Errorf("view = %q, want the raw fields that are still known", view)
	}
}

func TestADetailFromASupersededEventIsIgnored(t *testing.T) {
	fake := newFakeSource()
	first := manyFakeEvents(1)[0]
	first.ID = "evt-A"
	second := manyFakeEvents(1)[0]
	second.ID = "evt-B"
	second.Timestamp = second.Timestamp.Add(time.Second)
	second.TimestampUnixUS = second.Timestamp.UnixMicro()

	m := detailModel(t, fake, 2*time.Minute, []event.Event{first, second})
	m.selected = 0
	onA, _ := press(t, m, "enter")
	stale := onA.detail.token

	onB, _ := press(t, onA, "esc")
	onB.selected = 1
	onB, _ = press(t, onB, "enter")

	model, _ := onB.Update(detailDoneMsg{token: stale, ctx: inspect.Context{Event: first}})
	got := model.(Model)
	if got.detail.ev.ID != "evt-B" {
		t.Fatalf("event = %q, want the pane to still describe evt-B", got.detail.ev.ID)
	}
	if !strings.Contains(strings.Join(got.detailLines(), "\n"), "evt-B") {
		t.Error("the frame shows the superseded event")
	}
}
