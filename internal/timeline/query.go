package timeline

import (
	"container/heap"
	"context"
	"errors"
	"io"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
)

type Filter struct {
	Since, Until time.Time
	Categories   []event.Category
	Severities   []event.Severity
	Units        []string
	PIDs         []int
	Keyword      string
	BootID       string
}

type Result struct {
	Events       []event.Event
	Truncated    bool
	NextCursor   string
	CoveredSince time.Time
	CoveredUntil time.Time
	Sources      []source.Status
	Limitations  []string
}

func (r *Result) AddLimitation(name string) {
	for _, have := range r.Limitations {
		if have == name {
			return
		}
	}
	r.Limitations = append(r.Limitations, name)
}

func Run(ctx context.Context, srcs []source.Source, f Filter, limit int, after string) (Result, error) {
	res := Result{
		Events:      []event.Event{},
		Sources:     []source.Status{},
		Limitations: []string{},
	}

	var iterators []source.Iterator
	closeIterators := func() {
		for _, it := range iterators {
			_ = it.Close()
		}
	}

	open := make(mergeHeap, 0, len(srcs))
	for _, s := range srcs {
		status := s.Available(ctx)
		res.Sources = append(res.Sources, status)
		if !status.Available {
			res.AddLimitation("source_unavailable:" + status.Name)
			continue
		}
		it, err := s.Query(ctx, source.Query{Since: f.Since, Until: f.Until, BootID: f.BootID})
		if err != nil {
			res.Sources[len(res.Sources)-1] = source.Status{Name: status.Name, Detail: err.Error(), Err: err}
			res.AddLimitation("source_unavailable:" + status.Name)
			continue
		}
		iterators = append(iterators, it)
		first, err := it.Next(ctx)
		if errors.Is(err, io.EOF) {
			continue
		}
		if err != nil {
			addStreamError(&res, status.Name, err, "source_unavailable:")
			if !isLimitation(err) {
				res.Sources[len(res.Sources)-1] = source.Status{Name: status.Name, Detail: err.Error(), Err: err}
			}
			continue
		}
		heap.Push(&open, &mergeCursor{ev: first, name: status.Name, it: it})
	}
	defer closeIterators()

	afterEvent, err := decodeAfter(after)
	if err != nil {
		return Result{}, err
	}
	if limit <= 0 {
		res.AddLimitation("result_not_limited")
	}

	var last *event.Event
	for open.Len() > 0 {
		head := heap.Pop(&open).(*mergeCursor)
		ev := head.ev

		next, err := head.it.Next(ctx)
		switch {
		case err == nil:
			head.ev = next
			heap.Push(&open, head)
		case errors.Is(err, io.EOF):
		default:
			addStreamError(&res, head.name, err, "source_failed_midstream:")
		}

		if !match(f, ev) {
			continue
		}
		if afterEvent != nil && !orderLess(*afterEvent, ev) {
			continue
		}
		if limit > 0 && len(res.Events) >= limit {
			res.Truncated = true
			break
		}
		res.Events = append(res.Events, ev)
		updateCoverage(&res, ev)
		last = &ev
	}
	// A cursor is only useful when something is left to fetch.
	if res.Truncated && last != nil {
		res.NextCursor = EncodeCursor(*last)
	}
	return res, nil
}

// Coverage describes the range the returned events actually prove. It is never
// copied from the filter: that would claim coverage the query did not observe.
func updateCoverage(res *Result, e event.Event) {
	if e.TimestampUnixUS == 0 {
		return
	}
	t := time.UnixMicro(e.TimestampUnixUS).UTC()
	if res.CoveredSince.IsZero() || t.Before(res.CoveredSince) {
		res.CoveredSince = t
	}
	if t.After(res.CoveredUntil) {
		res.CoveredUntil = t
	}
}

// A source reports something it had to leave out through Next, because an
// iterator has no other channel for it. That is news for the user, not a
// failure of the query.
func addStreamError(res *Result, name string, err error, token string) {
	var lim *source.Limitation
	if errors.As(err, &lim) {
		res.AddLimitation(lim.Message)
		return
	}
	res.AddLimitation(token + name)
}

func isLimitation(err error) bool {
	var lim *source.Limitation
	return errors.As(err, &lim)
}

type mergeCursor struct {
	ev   event.Event
	name string
	it   source.Iterator
}

type mergeHeap []*mergeCursor

func (h mergeHeap) Len() int           { return len(h) }
func (h mergeHeap) Less(i, j int) bool { return orderLess(h[i].ev, h[j].ev) }
func (h mergeHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *mergeHeap) Push(x any)        { *h = append(*h, x.(*mergeCursor)) }
func (h *mergeHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}
