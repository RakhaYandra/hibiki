package event

import "time"

const SchemaVersion = 1

type Category string

const (
	CategorySystem      Category = "system"
	CategoryService     Category = "service"
	CategoryApplication Category = "application"
	CategoryProcess     Category = "process"
	CategorySecurity    Category = "security"
	CategoryUnknown     Category = "unknown"
)

type Severity string

const (
	SeverityDebug    Severity = "debug"
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
	SeverityUnknown  Severity = "unknown"
)

type TimeBasis string

const (
	TimeBasisRealtime       TimeBasis = "realtime"
	TimeBasisRealtimeApprox TimeBasis = "realtime_approx"
	TimeBasisMonotonic      TimeBasis = "monotonic"
	TimeBasisUnknown        TimeBasis = "unknown"
)

type Source struct {
	Type   string  `json:"type"`
	Host   string  `json:"host,omitempty"`
	BootID *string `json:"boot_id"`
	Cursor *string `json:"cursor"`
}

type Service struct {
	Unit        *string `json:"unit"`
	ActiveState *string `json:"active_state"`
	SubState    *string `json:"sub_state"`
	Result      *string `json:"result"`
}

type Process struct {
	PID        *int    `json:"pid"`
	Comm       *string `json:"comm"`
	Executable *string `json:"executable"`
}

type Event struct {
	SchemaVersion        int               `json:"schema_version"`
	ID                   string            `json:"id"`
	Timestamp            time.Time         `json:"timestamp"`
	TimestampUnixUS      int64             `json:"timestamp_unix_us"`
	TimestampMonotonicUS *int64            `json:"timestamp_monotonic_us"`
	TimeBasis            TimeBasis         `json:"time_basis"`
	Source               Source            `json:"source"`
	Category             Category          `json:"category"`
	Severity             Severity          `json:"severity"`
	Message              string            `json:"message"`
	Service              Service           `json:"service"`
	Process              Process           `json:"process"`
	CorrelationIDs       []string          `json:"correlation_ids"`
	Raw                  map[string]string `json:"raw"`
	Limitations          []string          `json:"limitations"`
}

func (c Category) Valid() bool {
	switch c {
	case CategorySystem, CategoryService, CategoryApplication, CategoryProcess, CategorySecurity, CategoryUnknown:
		return true
	}
	return false
}

func (s Severity) Valid() bool {
	switch s {
	case SeverityDebug, SeverityInfo, SeverityWarning, SeverityError, SeverityCritical, SeverityUnknown:
		return true
	}
	return false
}

func (t TimeBasis) Valid() bool {
	switch t {
	case TimeBasisRealtime, TimeBasisRealtimeApprox, TimeBasisMonotonic, TimeBasisUnknown:
		return true
	}
	return false
}
