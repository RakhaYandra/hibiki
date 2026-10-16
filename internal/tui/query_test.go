package tui

import (
	"errors"
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
