package output

import (
	"encoding/json"
	"io"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

var nowFunc = time.Now

// A raw source.Status would marshal its error as {}, so consumers get an
// explicit projection instead.
type SourceStatus struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
}

// The envelope carries the limitations and the covered range next to the
// events, so a machine consumer can tell a full answer from a partial one
// (PRD 8.4).
type Envelope struct {
	SchemaVersion int            `json:"schema_version"`
	GeneratedAt   time.Time      `json:"generated_at"`
	CoveredSince  *time.Time     `json:"covered_since"`
	CoveredUntil  *time.Time     `json:"covered_until"`
	Truncated     bool           `json:"truncated"`
	NextCursor    *string        `json:"next_cursor"`
	Sources       []SourceStatus `json:"sources"`
	Limitations   []string       `json:"limitations"`
	Events        []event.Event  `json:"events"`
}

func RenderJSON(w io.Writer, res timeline.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(NewEnvelope(res))
}

func RenderEventJSON(w io.Writer, e event.Event) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}

func NewEnvelope(res timeline.Result) Envelope {
	env := Envelope{
		SchemaVersion: event.SchemaVersion,
		GeneratedAt:   nowFunc().UTC(),
		Truncated:     res.Truncated,
		Sources:       make([]SourceStatus, 0, len(res.Sources)),
		Limitations:   sliceOrEmpty(res.Limitations),
		Events:        sliceOrEmpty(res.Events),
	}
	if !res.CoveredSince.IsZero() {
		since := res.CoveredSince.UTC()
		env.CoveredSince = &since
	}
	if !res.CoveredUntil.IsZero() {
		until := res.CoveredUntil.UTC()
		env.CoveredUntil = &until
	}
	if res.NextCursor != "" {
		cursor := res.NextCursor
		env.NextCursor = &cursor
	}
	env.Sources = SourceStatuses(res.Sources)
	return env
}

func SourceStatuses(in []source.Status) []SourceStatus {
	out := make([]SourceStatus, 0, len(in))
	for _, s := range in {
		status := SourceStatus{Name: s.Name, Available: s.Available, Detail: s.Detail}
		if s.Err != nil {
			status.Error = s.Err.Error()
		}
		out = append(out, status)
	}
	return out
}

func sliceOrEmpty[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}
