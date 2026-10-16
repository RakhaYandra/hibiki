package event

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }

func TestEventJSONRoundTrip(t *testing.T) {
	const us = int64(1792142671274265)
	mono := int64(4698621544)
	in := Event{
		SchemaVersion:        SchemaVersion,
		ID:                   "evt_7d91c2",
		Timestamp:            time.UnixMicro(us).UTC(),
		TimestampUnixUS:      us,
		TimestampMonotonicUS: &mono,
		TimeBasis:            TimeBasisRealtime,
		Source: Source{
			Type:   "systemd-journal",
			Host:   "host",
			BootID: strptr("30107f27-d01e-4e42-b248-46e4da81db1a"),
			Cursor: strptr("s=abc;i=1"),
		},
		Category:       CategoryService,
		Severity:       SeverityError,
		Message:        "Failed to start example.service",
		Service:        Service{Unit: strptr("example.service")},
		Process:        Process{PID: intptr(2401), Comm: strptr("python3")},
		CorrelationIDs: []string{},
		Raw:            map[string]string{"PRIORITY": "3"},
		Limitations:    []string{},
	}

	first, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Event
	if err := json.Unmarshal(first, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	second, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("round trip changed the event:\n%s\n%s", first, second)
	}
	if out.TimestampUnixUS != us || !out.Timestamp.Equal(in.Timestamp) {
		t.Errorf("timestamp mismatch: %s / %d", out.Timestamp, out.TimestampUnixUS)
	}
}

func TestEventJSONKeepsNullsForUnknownOptional(t *testing.T) {
	b, err := json.Marshal(Event{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"boot_id":null`,
		`"cursor":null`,
		`"timestamp_monotonic_us":null`,
		`"unit":null`,
		`"active_state":null`,
		`"sub_state":null`,
		`"result":null`,
		`"pid":null`,
		`"comm":null`,
		`"executable":null`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
}
