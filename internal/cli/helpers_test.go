package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

// testNow is the clock every command test runs on, so windows and events are
// deterministic.
var testNow = time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)

func at(offset time.Duration) int64 { return testNow.Add(offset).UnixMicro() }

type recordingSource struct {
	name    string
	evs     []event.Event
	err     error
	queries []source.Query
}

func (r *recordingSource) Name() string { return r.name }

func (r *recordingSource) Available(context.Context) source.Status {
	if r.err != nil {
		return source.Status{Name: r.name, Detail: r.err.Error(), Err: r.err}
	}
	return source.Status{Name: r.name, Available: true, Detail: "recording source"}
}

func (r *recordingSource) Query(_ context.Context, q source.Query) (source.Iterator, error) {
	r.queries = append(r.queries, q)
	if r.err != nil {
		return nil, r.err
	}
	return &sliceIterator{evs: r.evs}, nil
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

func testEvent(us int64, id string, category event.Category, severity event.Severity) event.Event {
	unit := "a.service"
	return event.Event{
		SchemaVersion:   event.SchemaVersion,
		ID:              id,
		Timestamp:       time.UnixMicro(us).UTC(),
		TimestampUnixUS: us,
		TimeBasis:       event.TimeBasisRealtime,
		Source:          event.Source{Type: "test"},
		Category:        category,
		Severity:        severity,
		Message:         "message " + id,
		Service:         event.Service{Unit: &unit},
		CorrelationIDs:  []string{},
		Raw:             map[string]string{},
		Limitations:     []string{},
	}
}

func manyEvents(n int) []event.Event {
	evs := make([]event.Event, 0, n)
	base := testNow.Add(-10 * time.Minute)
	for i := 0; i < n; i++ {
		evs = append(evs, testEvent(base.Add(time.Duration(i)*time.Second).UnixMicro(), fmt.Sprintf("evt-%03d", i), event.CategorySystem, event.SeverityInfo))
	}
	return evs
}

type cliDeps struct {
	sources   []source.Source
	unitState func(ctx context.Context, unit string, userScope bool) (string, bool, error)
	now       time.Time
	save      func(res timeline.Result) (string, error)
}

func installDeps(t *testing.T, d cliDeps) {
	t.Helper()
	prevSources, prevUnit, prevNow := newSources, lookupUnit, nowFunc
	if d.sources != nil {
		newSources = func() []source.Source { return d.sources }
	}
	if d.unitState != nil {
		lookupUnit = d.unitState
	}
	if !d.now.IsZero() {
		nowFunc = func() time.Time { return d.now }
	}
	t.Cleanup(func() { newSources, lookupUnit, nowFunc = prevSources, prevUnit, prevNow })
}

type cliResult struct {
	stdout string
	stderr string
	code   int
}

// runCLI goes through the same path as Execute(), so exit codes and the error
// line on stderr are the real ones.
func runCLI(t *testing.T, d cliDeps, args ...string) cliResult {
	t.Helper()
	installDeps(t, d)
	root := NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	code := run(root)
	return cliResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func strPtr(s string) *string { return &s }

func unitStateNotFound(context.Context, string, bool) (string, bool, error) {
	return "not-found", false, nil
}
