package timeline

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
)

func strPtr(s string) *string { return &s }

func ev(us int64, id string) event.Event {
	pid := 42
	return event.Event{
		SchemaVersion:   event.SchemaVersion,
		ID:              id,
		Timestamp:       time.UnixMicro(us).UTC(),
		TimestampUnixUS: us,
		TimeBasis:       event.TimeBasisRealtime,
		Source:          event.Source{Type: "fake"},
		Category:        event.CategorySystem,
		Severity:        event.SeverityInfo,
		Message:         "connection refused",
		Service:         event.Service{Unit: strPtr("a.service")},
		Process:         event.Process{PID: &pid},
		CorrelationIDs:  []string{},
		Raw:             map[string]string{},
		Limitations:     []string{},
	}
}

func ids(evs []event.Event) []string {
	out := make([]string, 0, len(evs))
	for _, e := range evs {
		out = append(out, e.ID)
	}
	return out
}

type fakeSource struct {
	name   string
	evs    []event.Event
	err    error
	closes int
}

func newFakeSource(name string, evs ...event.Event) *fakeSource {
	return &fakeSource{name: name, evs: evs}
}

func newFailingSource(name string, err error) *fakeSource {
	return &fakeSource{name: name, err: err}
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Available(context.Context) source.Status {
	if f.err != nil {
		return source.Status{Name: f.name, Detail: f.err.Error(), Err: f.err}
	}
	return source.Status{Name: f.name, Available: true, Detail: "fake source"}
}

func (f *fakeSource) Query(context.Context, source.Query) (source.Iterator, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &fakeIterator{src: f, evs: f.evs}, nil
}

type fakeIterator struct {
	src *fakeSource
	evs []event.Event
	i   int
}

func (it *fakeIterator) Next(context.Context) (event.Event, error) {
	if it.i >= len(it.evs) {
		return event.Event{}, io.EOF
	}
	e := it.evs[it.i]
	it.i++
	return e, nil
}

func (it *fakeIterator) Close() error {
	it.src.closes++
	return nil
}

func assertClosed(t *testing.T, srcs ...*fakeSource) {
	t.Helper()
	for _, s := range srcs {
		if s.closes == 0 {
			t.Errorf("source %s was never closed", s.name)
		}
	}
}
