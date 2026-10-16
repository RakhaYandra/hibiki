package event

import "fmt"

const (
	LimitationIdentityFallback          = "identity_from_fallback"
	LimitationMessageNonUTF8            = "message_non_utf8"
	LimitationSourceRealtimeImplausible = "source_realtime_implausible"
	LimitationUnitUnresolved            = "unit_unresolved"
	LimitationMalformedRecords          = "malformed_records_skipped"
	LimitationMessageTruncated          = "message_truncated"
)

func (e *Event) AddLimitation(name string) {
	for _, have := range e.Limitations {
		if have == name {
			return
		}
	}
	e.Limitations = append(e.Limitations, name)
}

func (e *Event) Validate() error {
	if e.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version %d is not supported (want %d)", e.SchemaVersion, SchemaVersion)
	}
	if e.ID == "" {
		return fmt.Errorf("event id is empty")
	}
	if e.Source.Type == "" {
		return fmt.Errorf("event source type is empty")
	}
	if e.TimestampUnixUS <= 0 && e.TimeBasis != TimeBasisUnknown {
		return fmt.Errorf("timestamp_unix_us %d is not a usable realtime time", e.TimestampUnixUS)
	}
	if !e.Category.Valid() {
		return fmt.Errorf("category %q is not a known value", e.Category)
	}
	if !e.Severity.Valid() {
		return fmt.Errorf("severity %q is not a known value", e.Severity)
	}
	if e.TimeBasis != "" && !e.TimeBasis.Valid() {
		return fmt.Errorf("time_basis %q is not a known value", e.TimeBasis)
	}
	return nil
}
