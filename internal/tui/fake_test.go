package tui

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
)

type fakeSource struct {
	Events []event.Event
	Err    error

	Cancelled chan struct{}
	Queries   []source.Query

	block bool
	once  sync.Once
	mu    sync.Mutex
}

func newFakeSource(events ...event.Event) *fakeSource {
	return &fakeSource{Events: events, Cancelled: make(chan struct{})}
}

func (f *fakeSource) Name() string { return "fake" }

func (f *fakeSource) Available(context.Context) source.Status {
	if f.Err != nil {
		return source.Status{Name: "fake", Detail: f.Err.Error(), Err: f.Err}
	}
	return source.Status{Name: "fake", Available: true, Detail: "fake source"}
}

func (f *fakeSource) Query(_ context.Context, q source.Query) (source.Iterator, error) {
	f.mu.Lock()
	f.Queries = append(f.Queries, q)
	f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	return &fakeIterator{src: f, evs: f.Events}, nil
}

func (f *fakeSource) calls() []source.Query {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]source.Query(nil), f.Queries...)
}

type fakeIterator struct {
	src *fakeSource
	evs []event.Event
	i   int
}

func (it *fakeIterator) Next(ctx context.Context) (event.Event, error) {
	if it.src.block {
		<-ctx.Done()
		it.src.once.Do(func() { close(it.src.Cancelled) })
		return event.Event{}, ctx.Err()
	}
	if it.i >= len(it.evs) {
		return event.Event{}, io.EOF
	}
	ev := it.evs[it.i]
	it.i++
	return ev, nil
}

func (it *fakeIterator) Close() error { return nil }

func fakeEvent(us int64, id string) event.Event {
	unit := "sshd.service"
	return event.Event{
		SchemaVersion:   event.SchemaVersion,
		ID:              id,
		Timestamp:       time.UnixMicro(us).UTC(),
		TimestampUnixUS: us,
		TimeBasis:       event.TimeBasisRealtime,
		Source:          event.Source{Type: "journal"},
		Category:        event.CategoryService,
		Severity:        event.SeverityError,
		Message:         "message " + id,
		Service:         event.Service{Unit: &unit},
		Raw:             map[string]string{"MESSAGE": "message " + id},
		CorrelationIDs:  []string{},
		Limitations:     []string{},
	}
}

func manyFakeEvents(n int) []event.Event {
	base := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	evs := make([]event.Event, 0, n)
	for i := 0; i < n; i++ {
		evs = append(evs, fakeEvent(base.Add(time.Duration(i)*time.Second).UnixMicro(), fmt.Sprintf("evt-%03d", i)))
	}
	return evs
}
