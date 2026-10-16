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
			res.Sources[len(res.Sources)-1] = source.Status{Name: status.Name, Detail: err.Error(), Err: err}
			res.AddLimitation("source_unavailable:" + status.Name)
			continue
		}
		heap.Push(&open, &mergeCursor{ev: first, name: status.Name, it: it})
	}
	defer closeIterators()

	for open.Len() > 0 {
		head := heap.Pop(&open).(*mergeCursor)
		res.Events = append(res.Events, head.ev)

		next, err := head.it.Next(ctx)
		switch {
		case err == nil:
			head.ev = next
			heap.Push(&open, head)
		case errors.Is(err, io.EOF):
		default:
			res.AddLimitation("source_failed_midstream:" + head.name)
		}
	}
	return res, nil
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
