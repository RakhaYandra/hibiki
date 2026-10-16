package inspect

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

const base = int64(1_792_000_000_000_000)

func inspEvent(us int64, id string) event.Event {
	return event.Event{
		SchemaVersion:   event.SchemaVersion,
		ID:              id,
		Timestamp:       time.UnixMicro(us).UTC(),
		TimestampUnixUS: us,
		TimeBasis:       event.TimeBasisRealtime,
		Source:          event.Source{Type: "fake"},
		Category:        event.CategoryService,
		Severity:        event.SeverityWarning,
		Message:         "message " + id,
		CorrelationIDs:  []string{},
		Raw:             map[string]string{},
		Limitations:     []string{},
	}
}

func neighbourhood() []event.Event {
	return []event.Event{
		inspEvent(base-600*1e6, "evt_far"),
		inspEvent(base-60*1e6, "evt_before"),
		inspEvent(base, "evt_main"),
		inspEvent(base+60*1e6, "evt_after"),
	}
}

type fakeSource struct {
	name string
	evs  []event.Event
	err  error
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Available(context.Context) source.Status {
	if f.err != nil {
		return source.Status{Name: f.name, Detail: f.err.Error(), Err: f.err}
	}
	return source.Status{Name: f.name, Available: true, Detail: "fake"}
}

func (f *fakeSource) Query(context.Context, source.Query) (source.Iterator, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &sliceIterator{evs: f.evs}, nil
}

type sliceIterator struct {
	evs []event.Event
	i   int
}

func (it *sliceIterator) Next(context.Context) (event.Event, error) {
	if it.i >= len(it.evs) {
		return event.Event{}, io.EOF
	}
	e := it.evs[it.i]
	it.i++
	return e, nil
}

func (it *sliceIterator) Close() error { return nil }

func hasLimitation(c Context, want string) bool {
	for _, l := range c.Limitations {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}

func TestInspectFindsBeforeAndAfterWithinWindow(t *testing.T) {
	mainEvent := neighbourhood()[2]
	got, err := ForEvent(context.Background(), []source.Source{&fakeSource{name: "fake", evs: neighbourhood()}}, mainEvent, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Before) != 1 || got.Before[0].ID != "evt_before" {
		t.Fatalf("before = %+v", got.Before)
	}
	if len(got.After) != 1 || got.After[0].ID != "evt_after" {
		t.Fatalf("after = %+v", got.After)
	}
	for _, e := range append(append([]event.Event{}, got.Before...), got.After...) {
		if e.ID == "evt_far" {
			t.Error("an event outside the window must not appear")
		}
	}
}

func TestInspectExcludesTheEventItself(t *testing.T) {
	mainEvent := neighbourhood()[2]
	got, err := ForEvent(context.Background(), []source.Source{&fakeSource{name: "fake", evs: neighbourhood()}}, mainEvent, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range append(append([]event.Event{}, got.Before...), got.After...) {
		if e.ID == mainEvent.ID {
			t.Fatalf("the inspected event must not repeat in its own surroundings: %+v", got)
		}
	}
}

func TestInspectStatesMissingPrecedingEvents(t *testing.T) {
	only := inspEvent(base, "evt_main")
	following := inspEvent(base+30*1e6, "evt_after")
	got, err := ForEvent(context.Background(), []source.Source{&fakeSource{name: "fake", evs: []event.Event{only, following}}}, only, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Before) != 0 {
		t.Fatalf("before = %+v", got.Before)
	}
	if !hasLimitation(got, "no preceding events found") {
		t.Errorf("an empty side must be explained, not left blank: %v", got.Limitations)
	}
	for _, l := range got.Limitations {
		if strings.Contains(l, "unavailable") {
			t.Errorf("a readable source must not be reported as unavailable: %v", got.Limitations)
		}
	}
}

func TestInspectReportsUnavailableSource(t *testing.T) {
	broken := &fakeSource{name: "broken-journal", err: exit.SourceUnavailable("permission denied")}
	got, err := ForEvent(context.Background(), []source.Source{broken}, inspEvent(base, "evt_main"), 2*time.Minute)
	if err != nil {
		t.Fatalf("an unreadable source must not fail inspect: %v", err)
	}
	if !hasLimitation(got, "broken-journal") || !hasLimitation(got, "permission denied") {
		t.Errorf("the limitation must name the source and the reason: %v", got.Limitations)
	}
}

func TestInspectRefusesToPlaceATimelessEvent(t *testing.T) {
	timeless := inspEvent(0, "evt_timeless")
	timeless.TimeBasis = event.TimeBasisUnknown
	src := &fakeSource{name: "fake", evs: []event.Event{timeless}}
	got, err := ForEvent(context.Background(), []source.Source{src}, timeless, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Before) != 0 || len(got.After) != 0 {
		t.Fatalf("no window can be built without a timestamp: %+v", got)
	}
	if !hasLimitation(got, "no timestamp") {
		t.Errorf("limitations = %v", got.Limitations)
	}
}

func TestInspectRejectsNonPositiveWindow(t *testing.T) {
	_, err := ForEvent(context.Background(), []source.Source{&fakeSource{name: "fake"}}, inspEvent(base, "evt_main"), 0)
	if err == nil {
		t.Fatal("a zero window must be a usage error")
	}
	var e *exit.Error
	if !errors.As(err, &e) || e.Code != exit.CodeUsage {
		t.Errorf("err = %v", err)
	}
}

func TestInspectKeepsSourceStatusForReporting(t *testing.T) {
	mainEvent := neighbourhood()[2]
	got, err := ForEvent(context.Background(), []source.Source{&fakeSource{name: "fake", evs: neighbourhood()}}, mainEvent, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) == 0 || got.Sources[0].Name != "fake" {
		t.Fatalf("sources = %+v", got.Sources)
	}
}

func TestInspectDoesNotClaimCausality(t *testing.T) {
	mainEvent := neighbourhood()[2]
	got, err := ForEvent(context.Background(), []source.Source{&fakeSource{name: "fake", evs: neighbourhood()}}, mainEvent, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range got.Limitations {
		if strings.Contains(strings.ToLower(l), "because") || strings.Contains(strings.ToLower(l), "caused") {
			t.Errorf("inspect must never state a cause: %v", got.Limitations)
		}
	}
	if got.CorrelationIDs() != nil && len(got.CorrelationIDs()) != 0 {
		t.Errorf("v0.1.0 has no correlation data: %v", got.CorrelationIDs())
	}
}

var _ = timeline.Filter{}
