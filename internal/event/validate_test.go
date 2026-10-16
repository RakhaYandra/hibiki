package event

import (
	"testing"
	"time"
)

func validEvent() Event {
	return Event{
		SchemaVersion:   SchemaVersion,
		ID:              "evt_abc",
		Timestamp:       time.UnixMicro(1792138530123456).UTC(),
		TimestampUnixUS: 1792138530123456,
		TimeBasis:       TimeBasisRealtime,
		Source:          Source{Type: "systemd-journal"},
		Category:        CategorySystem,
		Severity:        SeverityInfo,
		Limitations:     []string{},
	}
}

func TestValidateRejectsMissingRequired(t *testing.T) {
	e := Event{SchemaVersion: SchemaVersion, TimestampUnixUS: 1792138530123456, Category: CategorySystem, Severity: SeverityInfo}
	if err := e.Validate(); err == nil {
		t.Fatal("missing id and source type must be rejected")
	}
	e.ID = "evt_x"
	e.Source = Source{Type: "systemd-journal"}
	if err := e.Validate(); err != nil {
		t.Fatalf("event should be valid: %v", err)
	}
}

func TestValidateRejectsWrongSchemaVersion(t *testing.T) {
	e := validEvent()
	e.SchemaVersion = 2
	if err := e.Validate(); err == nil {
		t.Fatal("a different schema_version must be rejected")
	}
}

func TestValidateRejectsUnknownEnumsAndNonPositiveTime(t *testing.T) {
	cases := map[string]func(*Event){
		"category": func(e *Event) { e.Category = "banana" },
		"severity": func(e *Event) { e.Severity = "banana" },
		"basis":    func(e *Event) { e.TimeBasis = "banana" },
		"time":     func(e *Event) { e.TimestampUnixUS = 0 },
	}
	for name, mutate := range cases {
		e := validEvent()
		mutate(&e)
		if err := e.Validate(); err == nil {
			t.Errorf("%s: invalid event must be rejected", name)
		}
	}
}

func TestValidateAllowsEmptyMessage(t *testing.T) {
	e := validEvent()
	e.Message = ""
	if err := e.Validate(); err != nil {
		t.Fatalf("empty message must stay legal: %v", err)
	}
}

func TestAddLimitationIsIdempotent(t *testing.T) {
	e := validEvent()
	e.AddLimitation(LimitationUnitUnresolved)
	e.AddLimitation(LimitationUnitUnresolved)
	if len(e.Limitations) != 1 {
		t.Fatalf("limitations = %v", e.Limitations)
	}
}

func TestLimitationConstantsAreDistinct(t *testing.T) {
	all := []string{
		LimitationIdentityFallback,
		LimitationMessageNonUTF8,
		LimitationSourceRealtimeImplausible,
		LimitationUnitUnresolved,
		LimitationMalformedRecords,
		LimitationMessageTruncated,
	}
	seen := map[string]bool{}
	for _, l := range all {
		if l == "" {
			t.Fatal("a limitation constant is empty")
		}
		if seen[l] {
			t.Fatalf("duplicate limitation constant %q", l)
		}
		seen[l] = true
	}
}
