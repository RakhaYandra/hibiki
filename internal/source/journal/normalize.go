package journal

import (
	"strconv"
	"strings"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
)

const sourceType = "systemd-journal"

const (
	minPlausibleRealtimeUS = int64(1e15)
	maxPlausibleRealtimeUS = int64(1e17)
)

// Normalize applies one fixed rule table to a journal record. Nothing here
// reads the message text, and nothing reads a live service state: a journal
// record is evidence of the past only (PRD 7.3, 6.1).
func Normalize(fields map[string]string) (event.Event, error) {
	e := event.Event{
		SchemaVersion:  event.SchemaVersion,
		Category:       event.CategoryFromJournal(fields),
		Severity:       event.SeverityFromPriority(fields["PRIORITY"]),
		Message:        fields["MESSAGE"],
		CorrelationIDs: []string{},
		Limitations:    []string{},
		Raw:            rawCopy(fields),
	}

	src := event.Source{
		Type:   sourceType,
		Host:   fields["_HOSTNAME"],
		BootID: optionalBootID(fields["_BOOT_ID"]),
		Cursor: optional(fields["__CURSOR"]),
	}
	e.Source = src
	e.ID = event.IDFor(src)
	if src.Cursor == nil {
		e.AddLimitation(event.LimitationIdentityFallback)
	}

	if err := setTime(&e, fields); err != nil {
		return event.Event{}, err
	}

	if raw := fields["__MONOTONIC_TIMESTAMP"]; raw != "" {
		mono, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return event.Event{}, exit.BadData("__MONOTONIC_TIMESTAMP %q is not an integer", raw)
		}
		e.TimestampMonotonicUS = &mono
	}

	unit := resolveUnit(fields)
	if unit == nil {
		e.AddLimitation(event.LimitationUnitUnresolved)
	}
	e.Service = event.Service{Unit: unit}
	e.Process = event.Process{
		PID:        optionalInt(fields["_PID"]),
		Comm:       optional(fields["_COMM"]),
		Executable: optional(fields["_EXE"]),
	}

	if limitation := fields[sentinelLimitationsKey]; limitation != "" {
		e.AddLimitation(limitation)
	}
	return e, nil
}

func setTime(e *event.Event, fields map[string]string) error {
	var exactUS int64
	hasExact := false
	if raw := fields["__REALTIME_TIMESTAMP"]; raw != "" {
		us, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || us <= 0 {
			return exit.BadData("__REALTIME_TIMESTAMP %q is not a usable realtime timestamp", raw)
		}
		exactUS, hasExact = us, true
	}

	sourceUS, hasSource := int64(0), false
	if raw := fields["_SOURCE_REALTIME_TIMESTAMP"]; raw != "" {
		if us, err := strconv.ParseInt(raw, 10, 64); err == nil && plausibleRealtime(us) {
			sourceUS, hasSource = us, true
		} else {
			e.AddLimitation(event.LimitationSourceRealtimeImplausible)
		}
	}

	switch {
	case hasExact:
		e.TimestampUnixUS, e.TimeBasis = exactUS, event.TimeBasisRealtime
	case hasSource:
		e.TimestampUnixUS, e.TimeBasis = sourceUS, event.TimeBasisRealtimeApprox
	default:
		e.TimeBasis = event.TimeBasisUnknown
	}
	if e.TimestampUnixUS > 0 {
		e.Timestamp = time.UnixMicro(e.TimestampUnixUS).UTC()
	}
	return nil
}

// A user unit is the same unit the journal logs twice: as user@UID.service in
// _SYSTEMD_UNIT and by its real name in _SYSTEMD_USER_UNIT (spike 5).
func resolveUnit(fields map[string]string) *string {
	if u := fields["_SYSTEMD_USER_UNIT"]; u != "" && u != "init.scope" {
		return &u
	}
	if u := fields["_SYSTEMD_UNIT"]; u != "" && !strings.HasPrefix(u, "user@") {
		return &u
	}
	return nil
}

func rawCopy(fields map[string]string) map[string]string {
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		if k == sentinelLimitationsKey {
			continue
		}
		out[k] = v
	}
	return out
}

func optionalBootID(raw string) *string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if id, err := event.NormalizeBootID(raw); err == nil {
		return optional(id)
	}
	// A source value that is not a uuid is kept as it came, so nothing read from
	// the journal is lost.
	return optional(raw)
}

func plausibleRealtime(us int64) bool {
	return us >= minPlausibleRealtimeUS && us <= maxPlausibleRealtimeUS
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func optionalInt(s string) *int {
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}
