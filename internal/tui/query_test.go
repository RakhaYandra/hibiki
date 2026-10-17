package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

func readableResult(evs []event.Event) timeline.Result {
	return timeline.Result{
		Events:  evs,
		Sources: []source.Status{{Name: "journal", Available: true, Detail: "readable"}},
	}
}

func TestQueryDoneFromASupersededTokenIsIgnored(t *testing.T) {
	m := New(Deps{Sources: []source.Source{newFakeSource()}}, Options{})
	m.startQuery(timeline.Filter{}, 200, "")
	stale := m.queryToken
	m.applyQueryDone(queryDoneMsg{token: stale, res: readableResult(manyFakeEvents(2))})

	m.startQuery(timeline.Filter{}, 200, "")
	m.applyQueryDone(queryDoneMsg{token: stale, res: readableResult(manyFakeEvents(10))})

	if len(m.events) != 2 {
		t.Fatalf("events = %d, want the 2 events the newer query already delivered", len(m.events))
	}
	if !m.loading {
		t.Error("loading = false, want the newer query to still be in flight")
	}
}

func TestSourceErrorBecomesAnErrorState(t *testing.T) {
	m := New(Deps{Sources: []source.Source{}}, Options{})
	m.startQuery(timeline.Filter{}, 200, "")
	m.applyQueryDone(queryDoneMsg{token: m.queryToken, res: readableResult(manyFakeEvents(3))})

	want := exit.SourceUnavailable("journal is not readable")
	m.startQuery(timeline.Filter{}, 200, "")
	m.applyQueryDone(queryDoneMsg{token: m.queryToken, err: want})

	if !errors.Is(m.err, want) {
		t.Fatalf("err = %v, want %v", m.err, want)
	}
	if len(m.events) != 0 {
		t.Errorf("events = %d, want none: a failed query must not keep the old list", len(m.events))
	}
	if m.loading {
		t.Error("loading = true after the query answered")
	}
}

func TestEmptyReadableWindowIsNotAnError(t *testing.T) {
	m := New(Deps{Sources: []source.Source{newFakeSource()}}, Options{})
	m.startQuery(timeline.Filter{}, 200, "")
	m.applyQueryDone(queryDoneMsg{token: m.queryToken, res: readableResult(nil)})

	if m.err != nil {
		t.Fatalf("err = %v, want nil: an empty readable window is a success", m.err)
	}
	if m.loading {
		t.Error("loading = true after the query answered")
	}
	if !m.isEmpty() {
		t.Error("isEmpty = false, want the empty state")
	}
}

func TestQuitCancelsTheRunningQuery(t *testing.T) {
	fake := newFakeSource()
	fake.block = true
	m := New(Deps{Sources: []source.Source{fake}}, Options{})

	cmd := m.startQuery(timeline.Filter{}, 200, "")
	go cmd()
	m.cancel()

	select {
	case <-fake.Cancelled:
	case <-time.After(time.Second):
		t.Fatal("the query did not observe the cancellation within a second")
	}
}

func TestSuccessfulQueryWritesTheSnapshotOnce(t *testing.T) {
	var saves int
	var saved timeline.Result
	m := New(Deps{
		Sources: []source.Source{newFakeSource()},
		Save: func(res timeline.Result) (string, error) {
			saves++
			saved = res
			return "/tmp/snapshot", nil
		},
	}, Options{})

	m.startQuery(timeline.Filter{}, 200, "")
	m.applyQueryDone(queryDoneMsg{token: m.queryToken, res: readableResult(manyFakeEvents(3))})

	if saves != 1 {
		t.Fatalf("save calls = %d, want exactly one", saves)
	}
	if len(saved.Events) != 3 {
		t.Errorf("saved events = %d, want the delivered result", len(saved.Events))
	}
}

func TestAFailedSaveIsAWarningNotAQueryFailure(t *testing.T) {
	m := New(Deps{
		Sources: []source.Source{newFakeSource()},
		Save:    func(timeline.Result) (string, error) { return "", errors.New("cache is not writable") },
	}, Options{})

	m.startQuery(timeline.Filter{}, 200, "")
	m.applyQueryDone(queryDoneMsg{token: m.queryToken, res: readableResult(manyFakeEvents(2))})

	if m.err != nil {
		t.Fatalf("err = %v, want nil: the answer is already in hand", m.err)
	}
	if len(m.events) != 2 {
		t.Errorf("events = %d, want the delivered result", len(m.events))
	}
	if m.saveWarning == "" {
		t.Error("saveWarning is empty, want the reason on the footer")
	}
}

func pagingModel(fake *fakeSource, limit, pageSize int) Model {
	return New(Deps{
		Sources:  []source.Source{fake},
		Now:      func() time.Time { return time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC) },
		PageSize: pageSize,
	}, Options{
		Filter: timeline.Filter{Since: time.Date(2026, 10, 16, 7, 0, 0, 0, time.UTC)},
		Limit:  limit,
	})
}

func loadFirstPage(t *testing.T, m Model) Model {
	t.Helper()
	cmd := m.startQuery(m.filter, m.limit, "")
	if cmd == nil {
		t.Fatal("cmd = nil, want the first page")
	}
	model, _ := m.Update(cmd())
	return model.(Model)
}

func idsOf(evs []event.Event) map[string]bool {
	seen := make(map[string]bool, len(evs))
	for _, ev := range evs {
		seen[ev.ID] = true
	}
	return seen
}

func TestMovingDownAtTheBottomLoadsTheNextPage(t *testing.T) {
	fake := newFakeSource(manyFakeEvents(250)...)
	m := loadFirstPage(t, pagingModel(fake, 200, 50))
	if len(m.events) != 200 || !m.truncated || m.cursorNext == "" {
		t.Fatalf("first page = %d events, truncated %v, cursor %q, want 200 and a cursor", len(m.events), m.truncated, m.cursorNext)
	}

	m.selected = len(m.events) - 1
	m.ensureVisible()
	asked, cmd := press(t, m, "down")
	if cmd == nil {
		t.Fatal("cmd = nil, want the next page at the bottom")
	}
	if len(asked.events) != 200 {
		t.Fatalf("events = %d, want the list untouched until the page arrives", len(asked.events))
	}
	if !asked.loadingMore {
		t.Error("loadingMore = false, want it set while the page is in flight")
	}

	arrived, _ := asked.Update(cmd())
	got := arrived.(Model)
	if len(got.events) != 250 {
		t.Fatalf("events = %d, want the 250 the source holds", len(got.events))
	}
	if got.loadingMore {
		t.Error("loadingMore = true, want it cleared once the page arrived")
	}
	if seen := idsOf(got.events); len(seen) != len(got.events) {
		t.Fatalf("ids = %d for %d rows, want every id once", len(seen), len(got.events))
	}
	if got.truncated || got.cursorNext != "" {
		t.Fatalf("truncated = %v cursor = %q, want the list complete", got.truncated, got.cursorNext)
	}

	moved, _ := press(t, got, "down")
	if moved.selected != 200 {
		t.Fatalf("selected = %d, want the first event of the new page", moved.selected)
	}
}

func TestMovingDownAtTheBottomOfACompleteListChangesNothing(t *testing.T) {
	fake := newFakeSource(manyFakeEvents(30)...)
	m := loadFirstPage(t, pagingModel(fake, 200, 50))
	if m.truncated {
		t.Fatal("truncated = true, want a complete list")
	}
	m.selected = len(m.events) - 1

	pressed, cmd := press(t, m, "down")
	if cmd != nil {
		t.Fatal("cmd != nil, want nothing to fetch once the list is complete")
	}
	if pressed.selected != len(m.events)-1 {
		t.Fatalf("selected = %d, want it left at the last event", pressed.selected)
	}
	if len(fake.calls()) != 1 {
		t.Fatalf("queries = %d, want no second query", len(fake.calls()))
	}
}

func TestAFailedNextPageKeepsTheLoadedEvents(t *testing.T) {
	fake := newFakeSource(manyFakeEvents(250)...)
	m := loadFirstPage(t, pagingModel(fake, 200, 50))
	m.selected = len(m.events) - 1

	asked, _ := press(t, m, "down")
	if !asked.loadingMore {
		t.Fatal("loadingMore = false, want the page in flight")
	}

	model, _ := asked.Update(queryDoneMsg{
		token: asked.queryToken,
		page:  true,
		err:   exit.SourceUnavailable("journal is not readable"),
	})
	failed := model.(Model)

	if len(failed.events) != 200 {
		t.Fatalf("events = %d, want the loaded page kept", len(failed.events))
	}
	if failed.loadingMore {
		t.Error("loadingMore = true, want it cleared after the failure")
	}
	if failed.err != nil {
		t.Error("err set, want the failure not to replace the list with an error state")
	}
	if !strings.Contains(failed.footerView(), "next page failed") {
		t.Errorf("footer = %q, want the failed page named", failed.footerView())
	}
}

func TestTheNextPageUsesTheSameFilterAndTheCursor(t *testing.T) {
	fake := newFakeSource(manyFakeEvents(250)...)
	m := loadFirstPage(t, pagingModel(fake, 200, 50))
	if m.cursorNext == "" {
		t.Fatal("cursorNext is empty, want a cursor from the first page")
	}
	m.selected = len(m.events) - 1

	asked, cmd := press(t, m, "down")
	if cmd == nil {
		t.Fatal("cmd = nil, want the next page")
	}
	arrived, _ := asked.Update(cmd())
	got := arrived.(Model)

	calls := fake.calls()
	if len(calls) != 2 {
		t.Fatalf("queries = %d, want two", len(calls))
	}
	if !calls[0].Since.Equal(calls[1].Since) || !calls[0].Until.Equal(calls[1].Until) || calls[0].BootID != calls[1].BootID {
		t.Fatalf("second query = %+v, want the same window as %+v", calls[1], calls[0])
	}
	if len(got.events) != 250 || got.events[200].ID != "evt-200" {
		t.Fatalf("events = %d, want the page to resume at evt-200", len(got.events))
	}

	got.cursorNext = "%%%not-a-cursor"
	got.truncated = true
	got.selected = len(got.events) - 1
	refused, refusedCmd := press(t, got, "down")
	if refusedCmd == nil {
		t.Fatal("cmd = nil, want the corrupt cursor to reach the engine")
	}
	model, _ := refused.Update(refusedCmd())
	checked := model.(Model)
	if !strings.Contains(checked.footerView(), "next page failed") {
		t.Fatalf("footer = %q, want the engine to refuse the cursor the model handed over", checked.footerView())
	}
	if len(checked.events) != 250 {
		t.Fatalf("events = %d, want the list kept", len(checked.events))
	}
}

func TestPagingNeverRepeatsAnEventEvenIfTheSourceDoes(t *testing.T) {
	base := manyFakeEvents(250)
	dup := fakeEvent(base[200].TimestampUnixUS, "evt-150")
	list := append(append(append([]event.Event{}, base[:200]...), dup), base[200:]...)
	fake := newFakeSource(list...)

	m := loadFirstPage(t, pagingModel(fake, 200, 51))
	m.selected = len(m.events) - 1
	asked, cmd := press(t, m, "down")
	arrived, _ := asked.Update(cmd())
	got := arrived.(Model)

	if len(got.events) != 250 {
		t.Fatalf("events = %d, want the 250 unique rows", len(got.events))
	}
	if got.duplicates != 1 {
		t.Fatalf("duplicates = %d, want the repeated event counted", got.duplicates)
	}
	if seen := idsOf(got.events); len(seen) != len(got.events) {
		t.Fatalf("ids = %d for %d rows, want no id twice", len(seen), len(got.events))
	}
	if !strings.Contains(got.footerView(), "duplicate") {
		t.Errorf("footer = %q, want the skipped row reported", got.footerView())
	}
}
