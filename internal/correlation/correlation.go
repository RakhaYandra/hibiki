package correlation

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

type Incident struct {
	ID          string        `json:"id"`
	Rule        string        `json:"rule"`
	Confidence  Confidence    `json:"confidence"`
	Reason      string        `json:"reason"`
	Primary     event.Event   `json:"primary"`
	Related     []event.Event `json:"related"`
	WindowStart time.Time     `json:"window_start"`
	WindowEnd   time.Time     `json:"window_end"`
}

type Options struct {
	Window time.Duration
}

func Run(events []event.Event, opts Options) []Incident {
	sorted := append([]event.Event(nil), events...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].TimestampUnixUS != sorted[j].TimestampUnixUS {
			return sorted[i].TimestampUnixUS < sorted[j].TimestampUnixUS
		}
		return sorted[i].ID < sorted[j].ID
	})
	timed := make([]event.Event, 0, len(sorted))
	for _, ev := range sorted {
		if ev.TimestampUnixUS != 0 {
			timed = append(timed, ev)
		}
	}

	incs := []Incident{}
	for _, r := range rules {
		window := r.window
		if opts.Window > 0 {
			window = opts.Window
		}
		for _, m := range r.match(timed, window) {
			m.Rule = r.id
			m.Confidence = r.confidence
			m.ID = incidentID(r.id, m.Primary, m.Related)
			incs = append(incs, m)
		}
	}
	sort.Slice(incs, func(i, j int) bool {
		if incs[i].Primary.TimestampUnixUS != incs[j].Primary.TimestampUnixUS {
			return incs[i].Primary.TimestampUnixUS < incs[j].Primary.TimestampUnixUS
		}
		if incs[i].Rule != incs[j].Rule {
			return incs[i].Rule < incs[j].Rule
		}
		return incs[i].Primary.ID < incs[j].Primary.ID
	})
	stamp(incs)
	return incs
}

func For(e event.Event, incs []Incident) []Incident {
	out := []Incident{}
	for _, inc := range incs {
		if inc.Primary.ID == e.ID {
			out = append(out, inc)
			continue
		}
		for _, rel := range inc.Related {
			if rel.ID == e.ID {
				out = append(out, inc)
				break
			}
		}
	}
	return out
}

func ApplyIDs(dst []event.Event, incs []Incident) {
	idx := indexIncidentIDs(incs)
	for i := range dst {
		if ids, ok := idx[dst[i].ID]; ok {
			dst[i].CorrelationIDs = append([]string(nil), ids...)
		}
	}
}

func stamp(incs []Incident) {
	idx := indexIncidentIDs(incs)
	for i := range incs {
		incs[i].Primary.CorrelationIDs = append([]string(nil), idx[incs[i].Primary.ID]...)
		for j := range incs[i].Related {
			incs[i].Related[j].CorrelationIDs = append([]string(nil), idx[incs[i].Related[j].ID]...)
		}
	}
}

func indexIncidentIDs(incs []Incident) map[string][]string {
	idx := make(map[string][]string)
	for _, inc := range incs {
		idx[inc.Primary.ID] = append(idx[inc.Primary.ID], inc.ID)
		for _, rel := range inc.Related {
			idx[rel.ID] = append(idx[rel.ID], inc.ID)
		}
	}
	for id := range idx {
		sort.Strings(idx[id])
	}
	return idx
}

func incidentID(rule string, primary event.Event, related []event.Event) string {
	ids := make([]string, 0, len(related))
	for _, rel := range related {
		ids = append(ids, rel.ID)
	}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(rule + ":" + primary.ID + ":" + strings.Join(ids, ",")))
	return "inc_" + hex.EncodeToString(sum[:])[:12]
}
