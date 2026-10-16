package output

import (
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

func fixedNow() time.Time { return time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC) }

func sampleEvent() event.Event {
	unit := "example.service"
	pid := 2401
	ts := time.Date(2026, 10, 16, 8, 15, 30, 123456000, time.UTC)
	return event.Event{
		SchemaVersion:   event.SchemaVersion,
		ID:              "evt_7d91c2",
		Timestamp:       ts,
		TimestampUnixUS: ts.UnixMicro(),
		TimeBasis:       event.TimeBasisRealtime,
		Source:          event.Source{Type: "systemd-journal"},
		Category:        event.CategoryService,
		Severity:        event.SeverityError,
		Message:         "Failed to start example.service",
		Service:         event.Service{Unit: &unit},
		Process:         event.Process{PID: &pid},
		CorrelationIDs:  []string{},
		Raw:             map[string]string{"PRIORITY": "3"},
		Limitations:     []string{},
	}
}

func sampleResult() timeline.Result {
	e := sampleEvent()
	return timeline.Result{
		Events:       []event.Event{e},
		CoveredSince: e.Timestamp,
		CoveredUntil: e.Timestamp,
		Sources:      []source.Status{{Name: "systemd-journal", Available: true, Detail: "journal is readable"}},
		Limitations:  []string{},
	}
}
