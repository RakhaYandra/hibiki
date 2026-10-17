package correlation

import (
	"fmt"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
)

const restartMessageID = "5eb03494b6584870a536b337290809b3"

type rule struct {
	id         string
	confidence Confidence
	window     time.Duration
	match      func(events []event.Event, window time.Duration) []Incident
}

var rules = []rule{
	{
		id:         "restart-loop",
		confidence: ConfidenceHigh,
		window:     5 * time.Minute,
		match:      matchRestartLoop,
	},
	{
		id:         "failed-then-error",
		confidence: ConfidenceMedium,
		window:     30 * time.Second,
		match:      matchFailedThenError,
	},
}

type groupKey struct {
	unit string
	boot string
}

func matchRestartLoop(events []event.Event, window time.Duration) []Incident {
	limit := window.Microseconds()
	incs := []Incident{}
	for key, group := range groupByUnit(events, restartCandidate) {
		for i := 0; i < len(group); {
			j := i
			for j+1 < len(group) && group[j+1].TimestampUnixUS-group[i].TimestampUnixUS <= limit {
				j++
			}
			if j-i+1 < 3 {
				i++
				continue
			}
			reason := fmt.Sprintf("%s: %d restart or failure events within %s", key.unit, j-i+1, formatWindow(window))
			incs = append(incs, incident(group[i], group[i+1:j+1], reason, window))
			i = j + 1
		}
	}
	return incs
}

func matchFailedThenError(events []event.Event, window time.Duration) []Incident {
	limit := window.Microseconds()
	incs := []Incident{}
	for key, group := range groupByUnit(events, errorCandidate) {
		var covered int64
		for i, ev := range group {
			if ev.Category != event.CategoryService || ev.TimestampUnixUS <= covered {
				continue
			}
			var related []event.Event
			for _, other := range group[i+1:] {
				delta := other.TimestampUnixUS - ev.TimestampUnixUS
				if delta <= 0 {
					continue
				}
				if delta > limit {
					break
				}
				related = append(related, other)
			}
			if len(related) == 0 {
				continue
			}
			covered = ev.TimestampUnixUS + limit
			reason := fmt.Sprintf("%s: failed, then %d error event(s) from the same unit within %s", key.unit, len(related), formatWindow(window))
			incs = append(incs, incident(ev, related, reason, window))
		}
	}
	return incs
}

func restartCandidate(ev event.Event) bool {
	return ev.Category == event.CategoryService &&
		(ev.Severity == event.SeverityError || ev.Raw["MESSAGE_ID"] == restartMessageID)
}

func errorCandidate(ev event.Event) bool {
	return ev.Severity == event.SeverityError
}

func groupByUnit(events []event.Event, keep func(event.Event) bool) map[groupKey][]event.Event {
	groups := make(map[groupKey][]event.Event)
	for _, ev := range events {
		if ev.Service.Unit == nil || !keep(ev) {
			continue
		}
		key := groupKey{unit: *ev.Service.Unit, boot: bootIDKey(ev.Source.BootID)}
		groups[key] = append(groups[key], ev)
	}
	return groups
}

// A nil boot id and any non-nil boot id must never share a group (ADR-007 boot guard), so the nil key gets a prefix no non-nil value can produce.
func bootIDKey(id *string) string {
	if id == nil {
		return "\x00"
	}
	return "\x01" + *id
}

func formatWindow(d time.Duration) string {
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int64(d/time.Minute))
	}
	return d.String()
}

func incident(primary event.Event, related []event.Event, reason string, window time.Duration) Incident {
	start := time.UnixMicro(primary.TimestampUnixUS).UTC()
	return Incident{
		Reason:      reason,
		Primary:     primary,
		Related:     related,
		WindowStart: start,
		WindowEnd:   start.Add(window),
	}
}
