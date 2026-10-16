package inspect

import (
	"context"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

type Context struct {
	Event       event.Event
	Before      []event.Event
	After       []event.Event
	Sources     []source.Status
	Limitations []string
}

func (c *Context) AddLimitation(name string) {
	for _, have := range c.Limitations {
		if have == name {
			return
		}
	}
	c.Limitations = append(c.Limitations, name)
}

func (c Context) CorrelationIDs() []string { return c.Event.CorrelationIDs }

// Two windowed queries around the event, not a causality analysis: the result
// says what the journal recorded before and after, never why (PRD 8.3).
func ForEvent(ctx context.Context, srcs []source.Source, e event.Event, window time.Duration) (Context, error) {
	out := Context{
		Event:       e,
		Before:      []event.Event{},
		After:       []event.Event{},
		Sources:     []source.Status{},
		Limitations: []string{},
	}
	if window <= 0 {
		return out, exit.Usage("--context must be a positive duration, got %s", window)
	}
	if e.TimestampUnixUS == 0 {
		out.AddLimitation("the event has no timestamp, so no surrounding window can be built")
		return out, nil
	}

	at := time.UnixMicro(e.TimestampUnixUS).UTC()
	bootID := ""
	if e.Source.BootID != nil {
		bootID = *e.Source.BootID
	}

	before, err := timeline.Run(ctx, srcs, timeline.Filter{Since: at.Add(-window), Until: at, BootID: bootID}, 0, "")
	if err != nil {
		return out, err
	}
	after, err := timeline.Run(ctx, srcs, timeline.Filter{Since: at, Until: at.Add(window), BootID: bootID}, 0, "")
	if err != nil {
		return out, err
	}

	out.Before = without(before.Events, e.ID)
	out.After = without(after.Events, e.ID)
	out.Sources = mergeSources(before.Sources, after.Sources)
	out.Limitations = append(out.Limitations, before.Limitations...)
	out.Limitations = append(out.Limitations, after.Limitations...)
	dedupeLimitations(&out)

	for _, side := range []struct {
		label string
		evs   []event.Event
		note  string
	}{
		{"before", out.Before, "no preceding events found within the window"},
		{"after", out.After, "no following events found within the window"},
	} {
		if len(side.evs) == 0 && !anyUnavailable(out.Sources) {
			out.AddLimitation(side.note)
		}
	}
	return out, nil
}

func without(evs []event.Event, id string) []event.Event {
	out := make([]event.Event, 0, len(evs))
	for _, e := range evs {
		if e.ID == id {
			continue
		}
		out = append(out, e)
	}
	return out
}

func mergeSources(a, b []source.Status) []source.Status {
	out := make([]source.Status, 0, len(a)+len(b))
	index := map[string]int{}
	for _, list := range [][]source.Status{a, b} {
		for _, s := range list {
			if i, ok := index[s.Name]; ok {
				if !s.Available {
					out[i] = s
				}
				continue
			}
			index[s.Name] = len(out)
			out = append(out, s)
		}
	}
	return out
}

func anyUnavailable(srcs []source.Status) bool {
	for _, s := range srcs {
		if !s.Available {
			return true
		}
	}
	return false
}

// A source level limitation already names its cause, and an unbounded window is
// the whole point of inspect, so both are dropped from the report.
func dedupeLimitations(c *Context) {
	kept := make([]string, 0, len(c.Limitations))
	for _, l := range c.Limitations {
		if l == "result_not_limited" || isSourceLimitation(l) {
			continue
		}
		kept = append(kept, l)
	}
	c.Limitations = kept
	for _, s := range c.Sources {
		if s.Available {
			continue
		}
		reason := s.Detail
		if reason == "" && s.Err != nil {
			reason = s.Err.Error()
		}
		c.AddLimitation(s.Name + " unavailable: " + reason)
	}
}

func isSourceLimitation(l string) bool {
	return len(l) > 7 && l[:7] == "source_"
}
